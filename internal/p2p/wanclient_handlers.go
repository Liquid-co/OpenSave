package p2p

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/opensave/opensave/internal/delta"
	"github.com/opensave/opensave/internal/e2ee"
	"github.com/opensave/opensave/internal/p2p/pairing"
	"github.com/opensave/opensave/internal/p2p/syncengine"
	"github.com/opensave/opensave/internal/store"
	"github.com/opensave/opensave/internal/version"
)

// handleMessage dispatches one relay message, mirroring
// wan-client.js#handleRelayMessage.
func (w *WanClient) handleMessage(ctx context.Context, msg RelayMessage) {
	localID := w.localPeerID()
	if msg.To != "" && msg.To != localID {
		return
	}

	// Presence tracking for any message carrying a sender.
	if msg.From != "" && msg.From != localID {
		w.trackPresence(msg)
	}

	switch msg.Type {
	case "hello":
		if msg.From == localID {
			return
		}
		w.recordDiscovered(msg)

		// They claim we're paired but we aren't: notify unpair (unless a
		// handshake is in flight).
		_, pairedErr := w.engine.Store.GetPeer(msg.From)
		if pairedErr != nil && contains(msg.PairedPeers, localID) && !w.engine.Pairing.HasIncoming(msg.From) {
			w.engine.Log("warn", fmt.Sprintf("WAN peer %s thinks we're paired but we unpaired — notifying", msg.From))
			w.send(RelayMessage{Type: "unpair-notify", To: msg.From, From: localID})
		}
		if pairedErr != nil {
			w.warnIfStalePairing(msg)
		}

		// Reply so they discover us immediately.
		settings, err := w.engine.Store.GetSettings()
		if err == nil {
			w.send(RelayMessage{
				Type: "hello-reply", To: msg.From, From: localID,
				DeviceName: settings.DeviceName, DeviceType: settings.DeviceType, Port: settings.Port,
				Games:      w.gamesStateJSON(),
				AppVersion: version.Version, BuildTimeMs: version.BuildTimeMs(),
			})
		}

	case "hello-reply", "ping":
		if msg.From != localID {
			w.recordDiscovered(msg)
		}

	case "unpair-notify":
		if msg.From == localID {
			return
		}
		if _, err := w.engine.Store.GetPeer(msg.From); err == nil {
			w.engine.Log("warn", fmt.Sprintf("received unpair-notify from WAN peer %s — unpairing", msg.From))
			_ = w.engine.Store.UnpairPeer(msg.From)
			w.engine.notifyPeerUpdate()
		}

	case "untrack-notify":
		if msg.From == localID || msg.GameID == "" {
			return
		}
		if _, err := w.engine.Store.GetPeer(msg.From); err == nil {
			w.engine.applyPeerUntrack(msg.GameID)
		}

	case "retrack-notify":
		if msg.From == localID || msg.GameID == "" {
			return
		}
		if _, err := w.engine.Store.GetPeer(msg.From); err == nil {
			w.engine.applyPeerRetrack(msg.GameID)
		}

	case "sync-event":
		// Unseal before anything reads the payload. Every branch below uses
		// msg.Data, so decrypting in place here keeps them all unchanged
		// rather than leaving one of them reading ciphertext.
		if len(msg.SealedData) > 0 {
			plain, err := w.engine.openFromPeer(msg.From, msg.SealedData)
			if err != nil {
				w.engine.Log("warn", fmt.Sprintf(
					"could not decrypt a sync update from %s: %v", msg.From, err))
				return
			}
			msg.Data = plain
		}
		var ev syncengine.ProgressEvent
		_ = json.Unmarshal(msg.Data, &ev)
		switch msg.EventType {
		case "sync-start":
			if w.engine.Sync.Progress.OnSyncStart != nil {
				w.engine.Sync.Progress.OnSyncStart(msg.GameID, ev)
			}
		case "sync-progress":
			if w.engine.Sync.Progress.OnSyncProgress != nil {
				w.engine.Sync.Progress.OnSyncProgress(msg.GameID, ev)
			}
		case "sync-complete":
			if w.engine.Sync.Progress.OnSyncComplete != nil {
				w.engine.Sync.Progress.OnSyncComplete(msg.GameID, ev)
			}
			// Peer finished pulling from us over the relay: refresh the
			// shared lineage so pushed files start counting as synced.
			if peer, err := w.engine.Store.GetPeer(msg.From); err == nil {
				sp := syncengine.Peer{ID: peer.ID, Name: peer.Name, Address: "relay", Port: peer.Port, IsWan: true}
				// Same as the LAN path: the peer named the files it wrote, so
				// record them now instead of waiting on a manifest round trip
				// — over the relay that trip is slower still, so the window
				// this closes is wider here than on a LAN.
				var raw map[string]any
				_ = json.Unmarshal(msg.Data, &raw)
				w.engine.Sync.AddConfirmedLineage(msg.GameID, sp.ID, stringsFromEventData(raw, "pulledFiles"))
				go func() {
					refreshCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
					defer cancel()
					w.engine.Sync.RefreshLineage(refreshCtx, msg.GameID, sp)
				}()
			}
		case "in-sync":
			// Peer verified both sides match: confirm on our side (hash
			// re-check) before recording lineage + last-synced.
			var payload struct {
				ManifestHash string `json:"manifestHash"`
			}
			_ = json.Unmarshal(msg.Data, &payload)
			claimedHash := payload.ManifestHash
			if peer, err := w.engine.Store.GetPeer(msg.From); err == nil {
				sp := syncengine.Peer{ID: peer.ID, Name: peer.Name, Address: "relay", Port: peer.Port, IsWan: true}
				go func() {
					refreshCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
					defer cancel()
					w.engine.Sync.ConfirmInSync(refreshCtx, msg.GameID, sp, claimedHash)
				}()
			}
		case "sync-error":
			if w.engine.Sync.Progress.OnSyncError != nil {
				w.engine.Sync.Progress.OnSyncError(msg.GameID, ev)
			}
		}

	case "request":
		go w.serveRequest(ctx, msg)

	case "response":
		w.mu.Lock()
		pr, ok := w.pending[msg.MsgID]
		w.mu.Unlock()
		if !ok {
			return
		}
		// Only the peer this request was sent to may answer it, and it must
		// prove that the same way a request does. Checked here rather than
		// where the answer is consumed so a forgery never reaches the
		// one-slot channel and displaces the real reply.
		if msg.From != pr.peerID {
			w.engine.Log("warn", fmt.Sprintf(
				"discarded a reply to %s from %s — it was sent to %s", msg.MsgID, msg.From, pr.peerID))
			return
		}
		if !w.engine.verifyResponseAuth(pr.peerID, msg) {
			return
		}
		// Decrypt only after the reply has been shown to come from the peer
		// this request went to, and to be unaltered.
		if len(msg.SealedData) > 0 {
			plain, err := w.engine.openFromPeer(pr.peerID, msg.SealedData)
			if err != nil {
				w.engine.Log("warn", fmt.Sprintf(
					"could not decrypt a reply from %s: %v", pr.peerID, err))
				return
			}
			msg.Data = plain
		}
		select {
		case pr.ch <- msg:
		default:
		}
	}
}

// trackPresence flips paired peers online (routing them via relay) and
// refreshes discovered timestamps.
func (w *WanClient) trackPresence(msg RelayMessage) {
	if msg.AppVersion != "" {
		w.engine.recordPeerBuild(msg.From, msg.AppVersion, msg.BuildTimeMs)
	}
	peer, err := w.engine.Store.GetPeer(msg.From)
	if err == nil {
		wasOffline := peer.Status != "online"
		changed := wasOffline || peer.Address != "relay"
		peer.Status = "online"
		peer.Address = "relay"
		peer.LastSeenMs = time.Now().UnixMilli()
		if msg.Port > 0 {
			peer.Port = msg.Port
		}
		_ = w.engine.Store.UpsertPeer(peer)
		if wasOffline {
			w.engine.Log("info", fmt.Sprintf("peer %q came online via WAN; auto-syncing", peer.Name))
			w.engine.GoSync(func(ctx context.Context) { w.engine.SyncAllGames(ctx) })
		}
		if changed {
			w.engine.notifyPeerUpdate()
		}
	}

	w.mu.Lock()
	if p, ok := w.discovered[msg.From]; ok {
		p.LastSeen = time.Now().UnixMilli()
		w.discovered[msg.From] = p
	}
	w.mu.Unlock()
}

// warnIfStalePairing explains the one failure this protocol cannot repair on
// its own. A device that is wiped and reinstalled generates a fresh node ID,
// so the pairing row we still hold — keyed on the old one — can never match
// its messages again: trackPresence looks the sender up by ID, misses, and
// the old row sits at "offline" forever while the device is plainly online
// and sitting in the same room. Nothing about that is visible to the user,
// who sees a live device reported as dead.
//
// Recognising it by device name and re-pointing the pairing at the new ID
// automatically would be the friendly fix, and it is the wrong one: pairing
// is this app's trust boundary, and any device that claims the right name
// would inherit it. So say what happened and let the user decide.
func (w *WanClient) warnIfStalePairing(msg RelayMessage) {
	if msg.DeviceName == "" {
		return
	}
	peers, err := w.engine.Store.ListPeers()
	if err != nil {
		return
	}
	for _, p := range peers {
		if p.ID == msg.From || !strings.EqualFold(p.Name, msg.DeviceName) {
			continue
		}
		w.mu.Lock()
		already := w.staleWarned[msg.From]
		w.staleWarned[msg.From] = true
		w.mu.Unlock()
		if !already {
			w.engine.Log("warn", fmt.Sprintf(
				"%q is online but paired under an old identity, so it shows as offline — "+
					"this happens when a device is reinstalled or its data is cleared. "+
					"Unpair it and pair again to reconnect.", p.Name))
		}
		return
	}
}

func (w *WanClient) recordDiscovered(msg RelayMessage) {
	if msg.DeviceName == "" {
		return
	}
	w.mu.Lock()
	_, existed := w.discovered[msg.From]
	w.discovered[msg.From] = WanPeer{
		ID: msg.From, DeviceName: msg.DeviceName,
		DeviceType: orDefault(msg.DeviceType, "desktop"),
		Address:    "relay", Port: msg.Port, IsWan: true,
		LastSeen: time.Now().UnixMilli(),
	}
	w.mu.Unlock()
	if !existed {
		w.engine.notifyPeerUpdate()
	}
}

// serveRequest answers an HTTP-shaped RPC from a WAN peer — the relay-side
// equivalent of the /api/p2p/* routes, with the same pairing guard.
func (w *WanClient) serveRequest(ctx context.Context, msg RelayMessage) {
	// Decrypt before routing. Done after the caller has already verified the
	// message's MAC, so this only ever opens ciphertext that came from the
	// paired peer.
	if len(msg.SealedBody) > 0 {
		plain, err := w.engine.openFromPeer(msg.From, msg.SealedBody)
		if err != nil {
			w.engine.Log("warn", fmt.Sprintf("could not decrypt a request from %s: %v", msg.From, err))
			w.send(RelayMessage{
				Type: "response", To: msg.From, From: w.localPeerID(),
				MsgID: msg.MsgID, Status: 400,
				Data: []byte(`{"error":"payload could not be decrypted"}`),
			})
			return
		}
		msg.Body = plain
	}

	status, data := w.routeRequest(ctx, msg)
	raw, err := json.Marshal(data)
	if err != nil {
		status = 500
		raw = []byte(`{"error":"response serialization failed"}`)
	}
	resp := RelayMessage{
		Type: "response", To: msg.From, From: w.localPeerID(),
		MsgID: msg.MsgID, Status: status,
	}
	// Replies carry the save data itself — manifests, blocks — so this is the
	// direction that matters most for confidentiality.
	if sealed, ok := w.engine.sealForPeer(msg.From, raw); ok {
		resp.SealedData = sealed
	} else {
		resp.Data = raw
	}
	// Authenticate the answer as well as the question. The relay broadcasts
	// to the room, so every member sees the request id and could race a reply
	// to it — and an accepted forged reply means attacker-chosen bytes
	// written into a save folder, which is worse than being read.
	if key, keyErr := w.engine.requestAuthKey(msg.From); keyErr == nil {
		if nonce, nonceErr := e2ee.NewNonce(); nonceErr == nil {
			resp.Nonce = nonce
			resp.AuthMs = time.Now().UnixMilli()
			resp.Auth = e2ee.ResponseMAC(key, resp.From, resp.To, resp.MsgID, resp.Status,
				wireBody(resp.SealedData, resp.Data), nonce, resp.AuthMs)
		}
	}
	w.send(resp)
}

func (w *WanClient) routeRequest(ctx context.Context, msg RelayMessage) (int, any) {
	route := msg.Route
	from := msg.From
	w.engine.Log("info", fmt.Sprintf("WAN API request: %s %s from %s", msg.Method, route, from))

	requiresPairing := strings.HasPrefix(route, "/manifest/") ||
		strings.HasPrefix(route, "/blocks/") ||
		strings.HasPrefix(route, "/snapshot/") ||
		strings.HasPrefix(route, "/sync/trigger/") ||
		strings.HasPrefix(route, "/delete-file/") ||
		route == "/games" ||
		route == "/unpair"

	peer, pairedErr := w.engine.Store.GetPeer(from)
	isPaired := pairedErr == nil

	if requiresPairing && !isPaired {
		w.engine.Log("warn", fmt.Sprintf("blocked %s from unpaired WAN peer %s", route, from))
		return 401, map[string]string{"error": "Unauthorized: Requesting peer is not paired."}
	}

	// Being paired is a claim, not proof: "from" is written by the sender and
	// the relay does not check it, while the room publishes every device's
	// paired peer IDs. Requests that read or destroy save data have to prove
	// the sender holds the key pinned at pairing. See requestauth.go.
	if requiresPairing && isPaired {
		outcome, authErr := w.engine.verifyRequestAuth(peer, msg)
		switch outcome {
		case authRefused:
			w.engine.Log("warn", fmt.Sprintf("blocked %s from %s: %v", route, from, authErr))
			return 401, map[string]string{"error": "Unauthorized: Request failed authentication."}
		case authNotPossible:
			// Paired before end-to-end encryption existed, or the other side
			// has not upgraded yet. Allowed, as it was before, and said out
			// loud once it matters rather than failing silently.
			w.engine.Log("warn", fmt.Sprintf(
				"%s from %q is not authenticated — re-pair the two devices to protect it",
				route, peer.Name))
		}
	}

	switch {
	case route == "/ping":
		settings, _ := w.engine.Store.GetSettings()
		return 200, map[string]any{"status": "ok", "deviceName": settings.DeviceName, "deviceType": settings.DeviceType}

	case route == "/handshake":
		// publicKey is decoded here for the same reason the LAN handler
		// decodes it, and its absence here was the whole of the bug: the
		// sender has always put a key in the handshake, and this struct
		// dropped it on the floor. A field the JSON decoder does not know
		// about is not an error, so nothing failed and nothing warned — the
		// pairing completed, looked identical, and had no shared secret.
		//
		// The consequence was exact and inverted: pairing on a LAN pinned a
		// key and encrypted traffic that never leaves the house, while
		// pairing over the internet pinned nothing and sent saves through
		// the relay in the clear, which is the one place sealing exists for.
		var body struct {
			PeerID     string `json:"peerId"`
			DeviceName string `json:"deviceName"`
			DeviceType string `json:"deviceType"`
			Port       int    `json:"port"`
			PublicKey  string `json:"publicKey"`
		}
		if err := json.Unmarshal(msg.Body, &body); err != nil || body.PeerID == "" {
			return 400, map[string]string{"error": "peerId is required"}
		}
		w.engine.Pairing.RecordIncoming(pairing.IncomingRequest{
			PeerID: body.PeerID, DeviceName: body.DeviceName,
			DeviceType: orDefault(body.DeviceType, "desktop"),
			Address:    "relay", Port: body.Port, IsWan: true,
			PublicKey: body.PublicKey,
		})
		w.engine.notifyPeerUpdate()
		return 200, map[string]any{"status": "pending", "message": "Pairing request received via WAN. Waiting for approval."}

	case route == "/approve-confirm":
		var body struct {
			PeerID     string `json:"peerId"`
			DeviceName string `json:"deviceName"`
			DeviceType string `json:"deviceType"`
			Port       int    `json:"port"`
			PublicKey  string `json:"publicKey"`
		}
		if err := json.Unmarshal(msg.Body, &body); err != nil || body.PeerID == "" {
			return 400, map[string]string{"error": "peerId is required"}
		}
		if !w.engine.Pairing.ValidateConfirm(body.PeerID, "relay", body.Port, isPaired) {
			w.engine.Log("warn", "blocked unsolicited WAN approve-confirm from "+body.PeerID)
			return 400, map[string]string{"error": "Pairing confirmation rejected: no matching handshake initiated."}
		}
		w.engine.Pairing.TakeIncoming(body.PeerID)
		if err := w.engine.Store.UpsertPeer(store.Peer{
			ID: body.PeerID, Name: body.DeviceName, DeviceType: orDefault(body.DeviceType, "desktop"),
			Address: "relay", Port: body.Port, Status: "online", LastSeenMs: time.Now().UnixMilli(),
		}); err != nil {
			return 500, map[string]string{"error": err.Error()}
		}
		// Pinned after the row exists and separately from it, matching the
		// LAN handler: a key written through UpsertPeer would be blanked by
		// the next ordinary status update.
		if body.PublicKey != "" {
			if err := w.engine.Store.SetPeerPublicKey(body.PeerID, body.PublicKey); err != nil {
				w.engine.Log("warn", fmt.Sprintf(
					"could not pin %q's encryption key, so syncs with it stay unencrypted: %v",
					body.DeviceName, err))
			} else {
				// This message arrived before there was a key to check it
				// with, so it was let through unauthenticated — correctly,
				// since the key it carries is the one needed to check it.
				// Now that the key is pinned, check it retrospectively.
				//
				// Worth the second pass for one reason: encryption only turns
				// on once a peer has proved it can decrypt, and without this
				// the proof waits for the peer's next message. That leaves the
				// first request after pairing — a manifest, which names every
				// file in the save — travelling in the clear through a room
				// that may have anyone in it. Verifying here means a pairing
				// is encrypted from its first byte instead of its second
				// exchange.
				w.engine.latchIfAuthentic(body.PeerID, msg)
			}
		}
		w.engine.notifyPeerUpdate()
		return 200, map[string]any{"success": true, "message": "Pairing confirmed."}

	case route == "/unpair":
		var body struct {
			PeerID string `json:"peerId"`
		}
		_ = json.Unmarshal(msg.Body, &body)
		_ = w.engine.Store.UnpairPeer(body.PeerID)
		w.engine.notifyPeerUpdate()
		return 200, map[string]any{"success": true, "message": "Unpaired successfully."}

	case route == "/games":
		return 200, w.engine.PeerGameList()

	case strings.HasPrefix(route, "/manifest/"):
		return w.serveManifest(route, msg.From)

	case strings.HasPrefix(route, "/blocks/"):
		return w.serveBlocks(route, msg.Body)

	case strings.HasPrefix(route, "/delete-file/"):
		return w.serveDeleteFile(route, msg.Body, msg.From)

	case strings.HasPrefix(route, "/snapshot/"):
		return w.serveSnapshotDownload(route)

	case strings.HasPrefix(route, "/app-binary"):
		if !isPaired {
			return 401, map[string]string{"error": "Unauthorized: Requesting peer is not paired."}
		}
		return serveAppBinaryChunk(route)

	case strings.HasPrefix(route, "/sync/trigger/"):
		gameID := route[strings.LastIndex(route, "/")+1:]
		w.engine.GoSync(func(ctx context.Context) {
			syncCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
			defer cancel()
			if _, err := w.engine.SyncGame(syncCtx, gameID); err != nil {
				w.engine.Log("warn", fmt.Sprintf("WAN-triggered sync %s: %v", gameID, err))
			}
		})
		return 200, map[string]any{"success": true, "message": "Sync triggered."}

	default:
		return 404, map[string]string{"error": "Endpoint not supported over WAN."}
	}
}

// peerID is the relay sender, needed only to record who is waiting when this
// device is set to ask before tracking an unknown game.
func (w *WanClient) serveManifest(route string, peerID string) (int, any) {
	u, err := url.Parse(route)
	if err != nil {
		return 400, map[string]string{"error": "bad route"}
	}
	gameID := u.Path[strings.LastIndex(u.Path, "/")+1:]

	// Same auto-track + cover-backfill behavior as the LAN route — relay
	// peers were previously auto-tracked without cover art, which is why
	// covers didn't propagate between WAN-paired devices.
	game, err := w.engine.ensureManifestGame(gameID, manifestQueryFromURL(u.Query()), peerID)
	if err != nil {
		return 404, map[string]string{"error": err.Error()}
	}

	manifest, err := delta.BuildManifest(game.SavePath)
	if err != nil {
		return 500, map[string]string{"error": err.Error()}
	}
	resp := map[string]any{
		"gameId": gameID, "activeBranch": game.ActiveBranch, "manifest": manifest,
	}
	if latest, err := w.engine.Snapshots.LatestSnapshot(gameID, ""); err == nil {
		resp["latestSnapshot"] = syncengine.SnapshotInfo{ID: latest.ID, Timestamp: latest.Timestamp, Comment: latest.Comment}
	} else {
		resp["latestSnapshot"] = nil
	}
	return 200, resp
}

func (w *WanClient) serveBlocks(route string, rawBody json.RawMessage) (int, any) {
	gameID := route[strings.LastIndex(route, "/")+1:]
	var body struct {
		RelPath      string   `json:"relPath"`
		BlockIndices []int    `json:"blockIndices"`
		BlockSize    int      `json:"blockSize"`
		Encodings    []string `json:"encodings"`
	}
	if err := json.Unmarshal(rawBody, &body); err != nil || body.RelPath == "" {
		return 400, map[string]string{"error": "relPath is required"}
	}

	game, err := w.engine.trackedGameForPeer(gameID)
	if err != nil {
		return 404, map[string]string{"error": "Game not found."}
	}
	if !delta.IsSafePath(game.SavePath, body.RelPath) {
		return 403, map[string]string{"error": "Access denied: path traversal attempt detected."}
	}

	// Resolved, not joined: this device's manifest advertises the agreed
	// (composed) spelling, while the file on this disk may be stored
	// decomposed — a macOS save. Joining the key verbatim would fail to
	// find the very file this device just offered.
	fullPath := delta.LocalNameFor(game.SavePath, body.RelPath)
	if isFile, _ := delta.ResolveLocalSaveFilePath(game.SavePath); isFile {
		fullPath = game.SavePath
	}
	blocks, err := delta.ReadBlocks(fullPath, body.BlockIndices, body.BlockSize)
	if err != nil {
		return 500, map[string]string{"error": err.Error()}
	}
	out := encodeBlocks(blocks, wantsGzip(body.Encodings))
	return 200, map[string]any{"relPath": body.RelPath, "blocks": out}
}

// serveDeleteFile applies a deletion a peer asked for over the relay.
//
// fromPeerID is who asked. It is needed because applying the deletion changes
// this device without any sync having run, so the merge-base for that peer is
// left describing a state that still holds the deleted file — and a base
// behind both sides turns the next ordinary one-sided edit into a conflict on
// a save nobody else touched. The relay path matters more than the LAN one
// here: devices syncing over the internet are the ones far enough apart for
// the stale window to be noticed.
func (w *WanClient) serveDeleteFile(route string, rawBody json.RawMessage, fromPeerID string) (int, any) {
	gameID := route[strings.LastIndex(route, "/")+1:]
	var body struct {
		RelPath string `json:"relPath"`
	}
	if err := json.Unmarshal(rawBody, &body); err != nil || body.RelPath == "" {
		return 400, map[string]string{"error": "relPath is required."}
	}
	game, err := w.engine.trackedGameForPeer(gameID)
	if err != nil {
		return 404, map[string]string{"error": "Game not found."}
	}
	if !delta.IsSafePath(game.SavePath, body.RelPath) {
		return 403, map[string]string{"error": "invalid path"}
	}
	// Resolved, not joined: this device's manifest advertises the agreed
	// (composed) spelling, while the file on this disk may be stored
	// decomposed — a macOS save. Joining the key verbatim would fail to
	// find the very file this device just offered.
	full := delta.LocalNameFor(game.SavePath, body.RelPath)
	_ = os.Chmod(full, 0o666)
	_ = os.Remove(full)

	if peer, pErr := w.engine.Store.GetPeer(fromPeerID); pErr == nil {
		w.engine.refreshLineageAfterDeletion(gameID, syncengine.Peer{
			ID: peer.ID, Name: peer.Name, Address: peer.Address, Port: peer.Port,
		})
	}
	return 200, map[string]any{"success": true}
}

func (w *WanClient) serveSnapshotDownload(route string) (int, any) {
	parts := strings.Split(strings.Trim(route, "/"), "/")
	if len(parts) < 3 {
		return 400, map[string]string{"error": "bad route"}
	}
	snapshotID := parts[len(parts)-1]

	snap, err := w.engine.Store.GetSnapshot(snapshotID)
	if err != nil {
		return 404, map[string]string{"error": "Snapshot ZIP file not found."}
	}
	raw, err := os.ReadFile(snap.ZipPath)
	if err != nil {
		return 404, map[string]string{"error": "Snapshot ZIP file not found."}
	}
	return 200, map[string]any{
		"snapshotId": snapshotID,
		"base64Data": base64.StdEncoding.EncodeToString(raw),
		"fileName":   snapshotID + ".zip",
	}
}

func contains(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}
