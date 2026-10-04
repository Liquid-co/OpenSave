package p2p

import (
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/opensave/opensave/internal/delta"
	"github.com/opensave/opensave/internal/logging"
	"github.com/opensave/opensave/internal/owntouch"
	"github.com/opensave/opensave/internal/p2p/pairing"
	"github.com/opensave/opensave/internal/p2p/syncengine"
	"github.com/opensave/opensave/internal/store"
	"github.com/opensave/opensave/internal/switchtitle"
	"github.com/opensave/opensave/internal/version"
)

// RegisterRoutes mounts the peer-to-peer protocol under /api/p2p on the
// daemon's router. Unlike the dashboard routes (localhost-only), these are
// reachable from the LAN but guarded by requirePairedPeer.
func (e *Engine) RegisterRoutes(r chi.Router) {
	r.Get("/api/p2p/ping", e.handlePing)
	r.Post("/api/p2p/handshake", e.handleHandshake)
	r.Post("/api/p2p/approve-confirm", e.handleApproveConfirm)

	r.Group(func(r chi.Router) {
		r.Use(e.requirePairedPeer)
		r.Post("/api/p2p/unpair", e.handleUnpair)
		r.Post("/api/p2p/untrack", e.handlePeerUntrack)
		r.Post("/api/p2p/retrack", e.handlePeerRetrack)
		r.Get("/api/p2p/games", e.handlePeerGameList)
		r.Post("/api/p2p/sync-event/{gameId}", e.handleSyncEvent)
		r.Get("/api/p2p/app-binary", e.handleAppBinary)
		r.Get("/api/p2p/speedtest", e.handleSpeedTest)

		// What moves save data, or starts a sync: refused while paused.
		r.Group(func(r chi.Router) {
			r.Use(e.refuseWhilePaused)
			r.Get("/api/p2p/manifest/{gameId}", e.handleManifest)
			r.Post("/api/p2p/blocks/{gameId}", e.handleBlocks)
			r.Post("/api/p2p/files/{gameId}", e.handleFileBatch)
			r.Post("/api/p2p/delete-file/{gameId}", e.handleDeleteFile)
			r.Get("/api/sync/trigger/{gameId}", e.handleSyncTrigger)
		})
	})
}

// speedTestMax bounds one speed test, and speedTestData is what it sends:
// random, so nothing along the way can compress it into a better figure.
const speedTestMax = 8 << 20

var speedTestData = func() []byte {
	b := make([]byte, 1<<20)
	_, _ = rand.Read(b)
	return b
}()

// handleSpeedTest sends n bytes (at most speedTestMax) for a peer to time:
// how it learns how close this device is (syncengine/linkspeed.go).
func (e *Engine) handleSpeedTest(w http.ResponseWriter, r *http.Request) {
	n, _ := strconv.Atoi(r.URL.Query().Get("n"))
	if n <= 0 || n > speedTestMax {
		n = speedTestMax
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.Itoa(n))
	for n > 0 {
		chunk := min(n, len(speedTestData))
		if _, err := w.Write(speedTestData[:chunk]); err != nil {
			return
		}
		n -= chunk
	}
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	if host == "::1" {
		return "127.0.0.1"
	}
	return host
}

// unsignedRefusal is what a paired device is told when its request was not
// signed with the pairing's key, over the network or the relay.
const unsignedRefusal = "Unauthorized: this request was not signed with the key from pairing. " +
	"Update both devices to OpenSave 2.4.1 or later; if they were paired over the internet before 2.4.0, unpair and pair them again."

// requirePairedPeer allows localhost (dashboard/CLI) plus IPs matching a
// paired peer. A valid request from a paired peer also refreshes its
// online status, and a peer coming back online triggers a full auto-sync
// (matching the JS guard's side effects).
func (e *Engine) requirePairedPeer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if isLoopbackIP(ip) {
			// The dashboard and the CLI, which do not sign — and, on one
			// machine, another device's daemon, which does: a second install
			// for testing, or every device in the e2e suite. That one is still
			// told apart, so a handler that needs to know which device asked
			// (lanPeerID) can, as it could over the network. A signature that
			// fails is not refused here, as nothing on loopback ever was; it
			// just identifies nobody.
			if id, _, ok := e.verifyLANRequest(r); ok && id != "" {
				r = r.WithContext(context.WithValue(r.Context(), lanPeerKey{}, id))
			}
			next.ServeHTTP(w, r)
			return
		}

		// Proof of key first, source address only as a fallback.
		//
		// An address is not an identity on a network somebody else can join:
		// it can be taken by ARP spoofing, and it is handed out again when a
		// DHCP lease expires. A device that proves it holds the key pinned at
		// pairing has said something an address cannot.
		provenID, _, authOK := e.verifyLANRequest(r)
		if !authOK {
			jsonError(w, http.StatusUnauthorized, "Unauthorized: Request failed authentication.")
			return
		}

		peers, err := e.Store.ListPeers()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "peer lookup failed")
			return
		}
		var matched *store.Peer
		for i := range peers {
			if provenID != "" {
				if peers[i].ID == provenID {
					matched = &peers[i]
					break
				}
				continue
			}
			if peers[i].Address == ip {
				matched = &peers[i]
				break
			}
		}
		if matched == nil {
			e.Log("warn", "blocked unauthorized P2P request from unpaired IP "+ip)
			jsonError(w, http.StatusUnauthorized, "Unauthorized: Requesting peer is not paired.")
			return
		}
		// Unsigned, the request has only its source address to say who sent
		// it, and an address on a shared network can be taken. So nothing
		// that reads or writes saves is served on it (CVE-2026-103398) —
		// only unpairing, which gives nothing away and is how a pairing too
		// old to sign is cleared up.
		if provenID == "" && matched.AuthVerifiedMs == 0 && r.URL.Path != "/api/p2p/unpair" {
			e.Log("warn", fmt.Sprintf(
				"refused an unsigned request from %q: it was not signed with the key from pairing. "+
					"If it runs a version before 2.4.0, update it; if they were paired long ago, unpair and pair the two again.",
				matched.Name))
			jsonError(w, http.StatusUnauthorized, unsignedRefusal)
			return
		}
		// A peer that has proved itself before must keep doing so, or an
		// attacker simply omits the headers and falls back to the address
		// check this exists to replace.
		if provenID == "" && matched.AuthVerifiedMs > 0 {
			// The honest reading of this state is usually not an attack: it is
			// the same device on an older build after a downgrade or a
			// reinstall from a backup. Saying so, and saying how to recover,
			// costs nothing an attacker gains from — they already know
			// whether their forgery worked — and saves the one person who
			// would otherwise see syncing stop with no idea why.
			e.Log("warn", fmt.Sprintf(
				"refused an unsigned request from %q, which has authenticated before. "+
					"If that device was downgraded or reinstalled, unpair and pair the two again.",
				matched.Name))
			jsonError(w, http.StatusUnauthorized,
				"Unauthorized: this device has authenticated before and this request was not signed. "+
					"If it was downgraded or reinstalled, unpair and pair the two devices again.")
			return
		}

		// Throttled online-status refresh (10s), with auto-sync on
		// offline->online transition.
		//
		// Decided under onlineMu, on the peer as it is stored now rather than
		// as the list above read it. A device coming back sends several
		// requests at once; each read "offline" before any had written
		// "online", so each started a sync of every game — twenty in a second
		// for one device (GitHub #15). The lock is taken only when the list
		// says an update is due, so the ordinary request does not wait on it.
		const lastSeenLimit = 10_000
		now := time.Now().UnixMilli()
		if matched.Status != "online" || now-matched.LastSeenMs > lastSeenLimit {
			e.onlineMu.Lock()
			wasOffline := false
			if current, err := e.Store.GetPeer(matched.ID); err == nil &&
				(current.Status != "online" || now-current.LastSeenMs > lastSeenLimit) {
				wasOffline = current.Status != "online"
				current.Status = "online"
				current.LastSeenMs = now
				_ = e.Store.UpdatePeer(current)
			}
			e.onlineMu.Unlock()
			if wasOffline {
				e.Log("info", fmt.Sprintf("peer %q connected; triggering auto-sync for all games", matched.Name))
				e.GoSync(func(ctx context.Context) { e.SyncAllGames(ctx) })
				e.notifyPeerUpdate()
			}
		}
		// The identity this request was matched to — proven where the peer
		// signs, address-matched where it cannot yet. Handlers that act on a
		// peer's behalf act on this, never on an ID in the body.
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), lanPeerKey{}, matched.ID)))
	})
}

func isLoopbackIP(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (e *Engine) handlePing(w http.ResponseWriter, r *http.Request) {
	settings, err := e.Store.GetSettings()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	from := r.URL.Query().Get("from")
	paired := true
	if from != "" {
		_, err := e.Store.GetPeer(from)
		paired = err == nil
		if !paired {
			// A device checks on the devices it believes it is paired with.
			// One this device unpaired, still checking, missed its goodbye.
			e.remindUnpaired(from, clientIP(r))
		}
	}
	jsonOK(w, map[string]any{
		"status":     "ok",
		"paired":     paired,
		"deviceName": settings.DeviceName,
		"deviceType": settings.DeviceType,
		// No game list. This route answers ANY caller — it sits outside the
		// paired-peer group on purpose, so a device can be probed before
		// pairing — and it used to hand every caller the full tracked
		// library. Neither caller read it. See wanclient.go for the same
		// removal from relay presence.
		"appVersion":  version.Version,
		"buildTimeMs": version.BuildTimeMs(),
	})
}

func (e *Engine) handleHandshake(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PeerID     string `json:"peerId"`
		DeviceName string `json:"deviceName"`
		DeviceType string `json:"deviceType"`
		Port       int    `json:"port"`
		PublicKey  string `json:"publicKey"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.PeerID == "" {
		jsonError(w, http.StatusBadRequest, "peerId is required")
		return
	}

	e.Pairing.RecordIncoming(pairing.IncomingRequest{
		PeerID:     body.PeerID,
		DeviceName: body.DeviceName,
		DeviceType: orDefault(body.DeviceType, "desktop"),
		Address:    clientIP(r),
		Port:       body.Port,
		PublicKey:  body.PublicKey,
	})
	e.Log("info", fmt.Sprintf("pairing request from %q (%s) — awaiting approval", body.DeviceName, clientIP(r)))
	e.notifyPeerUpdate()

	jsonOK(w, map[string]any{"status": "pending", "message": "Pairing request received. Waiting for host approval."})
}

func (e *Engine) handleApproveConfirm(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PeerID     string `json:"peerId"`
		DeviceName string `json:"deviceName"`
		DeviceType string `json:"deviceType"`
		Port       int    `json:"port"`
		PublicKey  string `json:"publicKey"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.PeerID == "" {
		jsonError(w, http.StatusBadRequest, "peerId is required")
		return
	}
	ip := clientIP(r)

	_, alreadyPaired := func() (store.Peer, bool) {
		p, err := e.Store.GetPeer(body.PeerID)
		return p, err == nil
	}()

	if !e.Pairing.ValidateConfirm(body.PeerID, ip, body.Port, alreadyPaired) {
		e.Log("warn", fmt.Sprintf("blocked unsolicited approve-confirm from %s (peer %s)", ip, body.PeerID))
		jsonError(w, http.StatusBadRequest, "Pairing confirmation rejected: no matching handshake initiated.")
		return
	}

	// Consume any pending incoming record from a simultaneous initiation.
	e.Pairing.TakeIncoming(body.PeerID)

	if err := e.Store.UpsertPeer(store.Peer{
		ID: body.PeerID, Name: body.DeviceName, DeviceType: orDefault(body.DeviceType, "desktop"),
		Address: ip, Port: body.Port, Status: "online", LastSeenMs: time.Now().UnixMilli(),
	}); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Pinned after the peer row exists, and separately from it: writing a key
	// through UpsertPeer would mean every later status update carried one too,
	// and the ones that do not know about keys would blank it.
	if body.PublicKey != "" {
		if err := e.Store.SetPeerPublicKey(body.PeerID, body.PublicKey); err != nil {
			e.Log("warn", fmt.Sprintf("could not pin %q's encryption key, so syncs with it stay unencrypted: %v", body.DeviceName, err))
		}
	}

	// Same machine, fresh identity (reinstall/reset) — drop the ghost entry.
	if removed, _ := e.Store.PrunePeersAtAddress(ip, body.Port, body.PeerID); len(removed) > 0 {
		e.Log("info", fmt.Sprintf("removed stale pairing %v — same device re-paired with a new identity", removed))
	}

	e.Log("success", fmt.Sprintf("pairing confirmed with %q (%s:%d)", body.DeviceName, ip, body.Port))
	e.notifyPeerUpdate()
	jsonOK(w, map[string]any{"success": true, "message": "Pairing confirmed."})
}

func (e *Engine) handleUnpair(w http.ResponseWriter, r *http.Request) {
	// The sender is unpairing itself. Which peer that is comes from the
	// middleware's match, not from the body — a signed request from one
	// paired device could otherwise unpair a different one by naming it.
	// The body's peerId is still read for a loopback caller (the dashboard
	// and CLI), which the middleware lets through without matching a peer.
	peerID := lanPeerID(r)
	if peerID == "" {
		var body struct {
			PeerID string `json:"peerId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.PeerID == "" {
			jsonError(w, http.StatusBadRequest, "peerId is required")
			return
		}
		peerID = body.PeerID
	}
	_ = e.Store.UnpairPeer(peerID)
	e.notifyPeerUpdate()
	jsonOK(w, map[string]any{"success": true})
}

// PeerGame is one entry in the list a device offers a paired peer so the
// user can link two differently-named copies of the same game together.
type PeerGame struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	SavePath string `json:"savePath"`
	AppID    string `json:"appId,omitempty"`
}

// PeerGameList returns the games tracked here, for a paired peer to show in
// its "link a game" picker.
//
// Deliberately not folded into the games map carried by ping and hello:
// that goes out every few seconds to every peer, and names and save paths
// would put a device's whole library on the wire continuously to say
// nothing that changes. This is asked for once, when a human opens the
// picker.
func (e *Engine) PeerGameList() []PeerGame {
	games, err := e.Store.ListGames()
	if err != nil {
		return []PeerGame{}
	}
	out := make([]PeerGame, 0, len(games))
	for _, g := range games {
		out = append(out, PeerGame{ID: g.ID, Name: g.Name, SavePath: g.SavePath, AppID: g.AppID})
	}
	return out
}

func (e *Engine) handlePeerGameList(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, e.PeerGameList())
}

// FetchPeerGames asks a paired peer what it is tracking, over whichever
// transport that peer is currently reachable on.
func (e *Engine) FetchPeerGames(ctx context.Context, peerID string) ([]PeerGame, error) {
	peer, err := e.Store.GetPeer(peerID)
	if err != nil {
		return nil, fmt.Errorf("peer not found: %w", err)
	}

	if peer.Address == "relay" {
		if e.Wan == nil {
			return nil, fmt.Errorf("no relay connection")
		}
		raw, err := e.Wan.Request(ctx, peerID, "/games", "GET", nil)
		if err != nil {
			return nil, err
		}
		var out []PeerGame
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil, fmt.Errorf("peer sent an unreadable game list: %w", err)
		}
		return out, nil
	}

	settings, err := e.Store.GetSettings()
	if err != nil {
		return nil, err
	}
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	u := fmt.Sprintf("http://%s:%d/api/p2p/games?from=%s",
		peer.Address, peer.Port, url.QueryEscape(settings.NodeID))
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := lanClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("peer returned %d", resp.StatusCode)
	}
	var out []PeerGame
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("peer sent an unreadable game list: %w", err)
	}
	return out, nil
}

// handlePeerUntrack / handlePeerRetrack mirror a paired peer's game op.
func (e *Engine) handlePeerUntrack(w http.ResponseWriter, r *http.Request) {
	var body struct {
		GameID string `json:"gameId"`
		At     int64  `json:"at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.GameID == "" {
		jsonError(w, http.StatusBadRequest, "gameId is required")
		return
	}
	e.applyPeerUntrack(body.GameID, body.At)
	jsonOK(w, map[string]any{"success": true})
}

func (e *Engine) handlePeerRetrack(w http.ResponseWriter, r *http.Request) {
	var body struct {
		GameID string `json:"gameId"`
		At     int64  `json:"at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.GameID == "" {
		jsonError(w, http.StatusBadRequest, "gameId is required")
		return
	}
	e.applyPeerRetrack(body.GameID, body.At)
	jsonOK(w, map[string]any{"success": true})
}

// manifestGameQuery is what a peer's manifest request tells us about the
// game, used for auto-tracking and cover backfill.
type manifestGameQuery struct {
	Name     string
	SavePath string
	AppID    string
	CoverURL string
	IsFile   bool
}

func manifestQueryFromURL(q interface{ Get(string) string }) manifestGameQuery {
	return manifestGameQuery{
		Name:     q.Get("name"),
		SavePath: q.Get("savePath"),
		AppID:    q.Get("appId"),
		CoverURL: q.Get("coverUrl"),
		IsFile:   q.Get("isFile") == "true",
	}
}

// backfillCover fills in cover art (and App ID) on a game already tracked
// here from what the peer sent — e.g. a game tracked manually without an App
// ID. Returns the (possibly updated) game.
func (e *Engine) backfillCover(game store.Game, q manifestGameQuery) store.Game {
	if game.CoverURL == "" && q.CoverURL != "" {
		game.CoverURL = q.CoverURL
		if game.AppID == "" {
			game.AppID = q.AppID
		}
		if err := e.Store.UpdateGame(game); err == nil {
			e.notifyGamesUpdate()
		}
	}
	return game
}

// ensureManifestGame returns the tracked game for a peer's manifest
// request — resolving it to an existing local game by alias or App ID,
// auto-tracking it (translated save path, carrying the peer's cover art)
// when still unknown, and backfilling missing cover art on known games.
// Shared by the LAN route and the WAN relay handler so both paths behave
// identically.
// peerID identifies the device asking. It is only needed when this device is
// set to ask before tracking, to record who is waiting; empty is tolerated
// (an unidentified caller simply produces an offer with no named device).
func (e *Engine) ensureManifestGame(gameID string, q manifestGameQuery, peerID string) (store.Game, error) {
	if game, err := e.Store.GetGame(gameID); err == nil {
		return e.backfillCover(game, q), nil
	}

	// Not tracked under this exact id. Before creating anything, try to
	// resolve the peer's game to one already tracked here — this is what lets
	// the same title sync across devices when it was tracked under different
	// names (e.g. a Steam title vs. a portable/cracked folder name):
	//   1. an explicit user link (alias), then
	//   2. a shared Steam App ID, but only if the user enabled it (off by
	//      default so a cracked and a legit copy never merge unexpectedly).
	if canonical, ok := e.Store.ResolveGameAlias(gameID); ok {
		if game, err := e.Store.GetGame(canonical); err == nil {
			return e.backfillCover(game, q), nil
		}
	}
	// A Switch game's title id next: it is the same game whatever id each
	// device tracks it under (switchmatch.go).
	if game, ok := e.matchSwitchTitle(gameID, q.SavePath); ok {
		return e.backfillCover(game, q), nil
	}

	settings, sErr := e.Store.GetSettings()
	if sErr != nil {
		return store.Game{}, sErr
	}
	if settings.MatchByAppID && q.AppID != "" {
		game, err := e.Store.FindGameByAppID(q.AppID)
		switch {
		case err == nil:
			// Remember the match as an explicit alias. Only manifest requests
			// carry an App ID; everything else the peer sends — above all the
			// reverse-pull trigger, which arrives as a bare peer-side game id
			// — has nothing to match on. Without recording it, a push from the
			// peer couldn't be resolved locally and the two devices only
			// converged on the next periodic reconcile, up to a minute later.
			if err := e.Store.AddGameAlias(gameID, game.ID); err != nil {
				e.Log("warn", fmt.Sprintf("could not record App-ID match %s -> %s: %v", gameID, game.ID, err))
			} else {
				e.Log("info", fmt.Sprintf("matched peer's %q to local %q by App ID %s", gameID, game.ID, q.AppID))
			}
			return e.backfillCover(game, q), nil
		case errors.Is(err, store.ErrAmbiguousAppID):
			// Several local games share this App ID (e.g. the same title
			// tracked at more than one save location). Guessing could drop a
			// peer's saves into the wrong folder, so say so and let the user
			// link the right pair explicitly.
			e.Log("warn", fmt.Sprintf(
				"%q (app id %s) matches more than one tracked game here — link the correct one from its Manage tab to sync it",
				q.Name, q.AppID))
		}
	}

	if q.Name == "" || q.SavePath == "" {
		return store.Game{}, fmt.Errorf("Game not found.")
	}
	// The user deliberately untracked this game here — do NOT auto-recreate
	// it just because a peer that still tracks it asked for its manifest.
	// Re-tracking it explicitly clears the tombstone.
	if e.Store.IsUntracked(gameID) {
		return store.Game{}, fmt.Errorf("Game not found.")
	}
	// Ask before guessing, when the user has chosen that.
	//
	// Everything above still runs first: an exact id match, a user's explicit
	// link, and App-ID matching all resolve to a real game without ever
	// consulting the peer's path. This only governs what happens when nothing
	// matched and the alternative is to invent a folder.
	//
	// The offer is recorded rather than a game being created, and the peer is
	// told plainly that this device is waiting. Saying "not found" here would
	// be a lie the other device cannot see past: it reports the same thing for
	// a game deliberately untracked, so the user would be told nothing is
	// wrong while their save quietly stopped syncing.
	if settings.ShouldAskBeforeTracking() {
		return e.offerGame(gameID, peerID, q)
	}

	rules := make([]delta.TranslationRule, len(settings.PathTranslations))
	for i, tr := range settings.PathTranslations {
		rules[i] = delta.TranslationRule{FromPattern: tr.FromPattern, ToPattern: tr.ToPattern}
	}
	localPath := delta.TranslatePathToLocal(q.SavePath, rules)
	// A Switch save goes where this device's emulator keeps that game — not
	// under the other install's profile id, which no emulator here has.
	if titleID := switchtitle.FromSavePath(q.SavePath); titleID != "" && e.SwitchSaveFolder != nil {
		localPath = e.SwitchSaveFolder(titleID, localPath)
	}

	// Never auto-track at a profile/system-level folder: syncing it would
	// hash the user's whole profile. Send the requester a clear reason
	// instead of a mysterious walk error.
	if reason := delta.DangerousSyncRoot(localPath); reason != "" {
		return store.Game{}, fmt.Errorf("cannot auto-track %q at %s: %s — set the game's save path on this device manually", q.Name, logging.Quote(localPath), reason)
	}

	// The other device names the folder; only this one decides what may be
	// tracked. A save path in a manifest request is the peer's say, and a
	// paired device — or anyone posing as one — could name any folder this
	// user can read: ~/.ssh, a browser profile, another app's settings. Once
	// tracked, its files were served to the peer and the peer's written into
	// it (CVE-2026-103398). The old guard above refuses only whole profiles,
	// drives and system folders.
	//
	// So a game arriving from a peer is tracked by itself only where this
	// device's own scanner recognises a save folder: one it found and noted,
	// or one inside an emulator's own save folder here. Anywhere else, it is
	// offered, and the user picks the folder on this device.
	if e.KnownSaveLocation == nil || !e.KnownSaveLocation(localPath) {
		e.Log("warn", fmt.Sprintf(
			"a paired device asked to sync %q at %s, which this device does not know as a save folder — "+
				"it is offered on Home instead, for you to place", q.Name, logging.Quote(localPath)))
		return e.offerGame(gameID, peerID, q)
	}

	game := store.Game{
		ID: gameID, Name: q.Name, SavePath: localPath, ActiveBranch: "main",
		AutoSync: true, MaxSnapshots: 5,
		// Carry the cover art from the requesting peer so the game
		// doesn't show as a blank tile on this device.
		AppID:    q.AppID,
		CoverURL: q.CoverURL,
	}
	// The alias check above and this insert are not one step, and linking is
	// a separate write that can land between them — leaving the peer's game
	// tracked twice, once under its own id and once as the entry it was just
	// linked to. Re-check and insert together so whichever happens first wins.
	canonicalID, err := e.Store.CreateGameUnlessAliased(game)
	if err != nil {
		return store.Game{}, fmt.Errorf("auto-track failed: %w", err)
	}
	if canonicalID != game.ID {
		// It was linked while we were deciding; use the game it points at.
		if linked, lErr := e.Store.GetGame(canonicalID); lErr == nil {
			return e.backfillCover(linked, q), nil
		}
	}
	if q.IsFile {
		_ = os.MkdirAll(filepath.Dir(localPath), 0o777)
	} else {
		_ = os.MkdirAll(localPath, 0o777)
	}
	e.Log("info", fmt.Sprintf("auto-tracked %q at %s from peer manifest request", q.Name, logging.Quote(localPath)))
	if e.OnAutoTracked != nil {
		e.OnAutoTracked(game)
	}
	e.notifyGamesUpdate()
	return game, nil
}

// offerGame records a game a peer syncs as offered, for the user to place on
// this device, and tells the peer this device is waiting rather than that the
// game does not exist: the same answer, which the peer cannot see past, is
// given for a game deliberately untracked.
func (e *Engine) offerGame(gameID, peerID string, q manifestGameQuery) (store.Game, error) {
	if err := e.Store.RecordOfferedGame(store.OfferedGame{
		GameID: gameID, PeerID: peerID, Name: q.Name,
		AppID: q.AppID, CoverURL: q.CoverURL, PeerPath: q.SavePath,
	}); err != nil {
		e.Log("warn", fmt.Sprintf("could not record %q as an offered game: %v", q.Name, err))
	} else {
		e.notifyGamesUpdate()
	}
	return store.Game{}, fmt.Errorf("%s: %q", syncengine.AwaitingFolderMessage, q.Name)
}

// handleManifest serves a game's manifest + branch + latest-snapshot info.
// If the game isn't tracked here yet, it is auto-tracked using the
// requester's supplied name/savePath (translated to local conventions).
func (e *Engine) handleManifest(w http.ResponseWriter, r *http.Request) {
	gameID := chi.URLParam(r, "gameId")

	var askingPeer string
	if peer, ok := e.peerByAddress(clientIP(r)); ok {
		askingPeer = peer.ID
	}
	game, err := e.ensureManifestGame(gameID, manifestQueryFromURL(r.URL.Query()), askingPeer)
	if err != nil {
		jsonError(w, http.StatusNotFound, err.Error())
		return
	}
	if e.holdingBack(game) {
		jsonError(w, http.StatusNotFound, syncengine.HeldMessage)
		return
	}
	// Being played here: synced when the session ends (syncengine/playing.go).
	if e.Sync.PlayingHere(game.ID) {
		jsonError(w, http.StatusServiceUnavailable, syncengine.PlayingMessage)
		return
	}
	// Never describe a save a sync here is writing: part-way through, it is a
	// mixture no device holds, and the asker would judge it as a save that had
	// moved (syncengine/settle.go). Held still while it is read; a write in
	// progress is waited out if it ends soon, and otherwise answered as busy,
	// which the asker takes as "ask again".
	readDone, ok := e.holdForServing(r.Context(), game.ID)
	if !ok {
		jsonError(w, http.StatusServiceUnavailable, syncengine.SettlingMessage)
		return
	}
	defer readDone()

	// Extra save locations are included when this game has any; a game with
	// none produces exactly the manifest it always did, down to the absent
	// field. An unreadable extra location is logged and left out rather than
	// listed empty, which the peer would read as everything in it being
	// deleted.
	extra, rootsErr := e.Store.GameRootPaths(gameID)
	if rootsErr != nil {
		extra = nil
	}
	manifest, failures, err := delta.BuildMultiManifest(game.SavePath, extra)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "manifest build failed: "+err.Error())
		return
	}
	for name, failure := range failures {
		e.Log("warn", fmt.Sprintf("could not read the %q location of %q: %v — it is left out of this sync", name, game.Name, failure))
	}
	// Remembered, so the asker found holding exactly this later is known to
	// hold a state this device had (syncengine/served.go).
	e.Sync.NoteServed(game.ID, lanPeerID(r), manifest)

	// Proto tells the asking peer this device understands save locations
	// beyond the primary one, so it is safe to send a root name in a block or
	// delete request. A peer that predates this answers without it and is
	// only ever asked about the primary location.
	resp := syncengine.ManifestResponse{
		Manifest:          manifest,
		ActiveBranch:      game.ActiveBranch,
		Proto:             ServedProto(),
		DeletionConfirmed: e.Sync.DeletionConfirmed(game.ID),
		Version:           e.Sync.LocalVersion(game, manifest),
		Versions:          true,
	}
	if latest, err := e.Snapshots.LatestSnapshot(gameID, ""); err == nil {
		resp.LatestSnapshot = &syncengine.SnapshotInfo{ID: latest.ID, Timestamp: latest.Timestamp, Comment: latest.Comment, ContentHash: latest.ContentHash}
	}
	// The asker already holds the state both devices last agreed on and says
	// so; if this device still holds it too, "unchanged" is the whole answer.
	// For a save of a quarter-million files the full manifest is tens of MB
	// of JSON, and the minute-long reconcile used to fetch it from every peer
	// every minute to learn that nothing had changed. Single-location games
	// only: extra locations carry their own bases (see RootHash).
	if ifHash := r.URL.Query().Get("ifHash"); ifHash != "" && len(manifest.Extra) == 0 &&
		manifest.ManifestHash() == ifHash {
		resp.Manifest = delta.Manifest{}
		resp.Unchanged = true
		resp.ManifestHash = ifHash
	}
	jsonOKCompressed(w, r, resp)
}

// holdForServing holds the game's save still for a manifest to be served from
// it, waiting at most syncengine.ServeSettleWait for a sync writing it to
// finish. ok is false if it did not; otherwise done lets go.
func (e *Engine) holdForServing(ctx context.Context, gameID string) (done func(), ok bool) {
	ctx, cancel := context.WithTimeout(ctx, syncengine.ServeSettleWait)
	defer cancel()
	done, err := e.Sync.Reading(ctx, gameID)
	return done, err == nil
}

func (e *Engine) handleBlocks(w http.ResponseWriter, r *http.Request) {
	gameID := chi.URLParam(r, "gameId")
	var body struct {
		RelPath string `json:"relPath"`
		// Root names which save location the path is relative to. Absent on
		// every peer that predates multi-root, which is why it is only ever
		// sent to a peer that answered a manifest request with a proto.
		Root         string   `json:"root"`
		BlockIndices []int    `json:"blockIndices"`
		BlockSize    int      `json:"blockSize"`
		Encodings    []string `json:"encodings"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.RelPath == "" {
		jsonError(w, http.StatusBadRequest, "relPath is required")
		return
	}

	game, err := e.trackedGameForPeer(gameID)
	if err != nil {
		jsonError(w, http.StatusNotFound, "Game not found.")
		return
	}
	base, ok := e.resolveServeRoot(gameID, game, body.Root)
	if !ok {
		jsonError(w, http.StatusNotFound, "This device has no save location named "+strconv.Quote(body.Root)+" for that game.")
		return
	}
	if !delta.IsSafePath(base, body.RelPath) {
		jsonError(w, http.StatusBadRequest, "invalid path")
		return
	}

	// Resolved, not joined: this device's manifest advertises the agreed
	// (composed) spelling, while the file on this disk may be stored
	// decomposed — a macOS save. Joining the key verbatim would fail to
	// find the very file this device just offered.
	fullPath := delta.LocalNameFor(base, body.RelPath)
	if isFile, _ := delta.ResolveLocalSaveFilePath(base); isFile {
		fullPath = base
	}

	blocks, err := delta.ReadBlocks(fullPath, body.BlockIndices, body.BlockSize)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "read blocks failed: "+err.Error())
		return
	}

	jsonOK(w, map[string]any{"blocks": encodeBlocks(blocks, wantsGzip(body.Encodings))})
}

// Bounds on one batch request, so a peer cannot make this device read and
// hold an unbounded amount in a single response. The requester stays well
// under them (syncengine.batchMaxFiles / batchMaxBytes).
const (
	fileBatchMaxFiles = 1024
	fileBatchMaxBytes = 16 << 20
)

// handleFileBatch serves the blocks of many files of one save location in a
// single response: handleBlocks for a list. One round trip per file was what
// made a save of many small files take hours; see syncengine.ProtoBatchFiles.
//
// A file that cannot be read is reported in its own entry rather than failing
// the batch, so the requester can say which one it was.
func (e *Engine) handleFileBatch(w http.ResponseWriter, r *http.Request) {
	gameID := chi.URLParam(r, "gameId")
	var body struct {
		Root      string                         `json:"root"`
		Files     []syncengine.FileBlocksRequest `json:"files"`
		Encodings []string                       `json:"encodings"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Files) == 0 {
		jsonError(w, http.StatusBadRequest, "files are required")
		return
	}
	if len(body.Files) > fileBatchMaxFiles {
		jsonError(w, http.StatusBadRequest, fmt.Sprintf("at most %d files per batch", fileBatchMaxFiles))
		return
	}
	game, err := e.trackedGameForPeer(gameID)
	if err != nil {
		jsonError(w, http.StatusNotFound, "Game not found.")
		return
	}
	base, ok := e.resolveServeRoot(gameID, game, body.Root)
	if !ok {
		jsonError(w, http.StatusNotFound, "This device has no save location named "+strconv.Quote(body.Root)+" for that game.")
		return
	}
	singleFile, _ := delta.ResolveLocalSaveFilePath(base)

	// What the answer will hold: each file's requested blocks at its block
	// size, but never more than the file itself. Counting every block at full
	// size took 256 files of 4 KB for 16 MB — the whole limit — and refused
	// any batch of small files with a few larger ones among them, which failed
	// whole pulls of saves made of many small files.
	var requested int64
	for _, f := range body.Files {
		bs := f.BlockSize
		if bs <= 0 {
			bs = 64 * 1024
		}
		n := int64(len(f.BlockIndices)) * int64(bs)
		if f.RelPath != "" && delta.IsSafePath(base, f.RelPath) {
			p := delta.LocalNameFor(base, f.RelPath)
			if singleFile {
				p = base
			}
			if info, err := os.Stat(p); err == nil && info.Size() < n {
				n = info.Size()
			}
		}
		requested += n
	}
	if requested > fileBatchMaxBytes {
		jsonError(w, http.StatusBadRequest, "batch too large")
		return
	}
	gz := wantsGzip(body.Encodings)

	out := make([]syncengine.FileBlocks, 0, len(body.Files))
	for _, f := range body.Files {
		entry := syncengine.FileBlocks{RelPath: f.RelPath}
		if f.RelPath == "" || !delta.IsSafePath(base, f.RelPath) {
			entry.Error = "invalid path"
			out = append(out, entry)
			continue
		}
		// Resolved, not joined — see handleBlocks.
		fullPath := delta.LocalNameFor(base, f.RelPath)
		if singleFile {
			fullPath = base
		}
		blocks, err := delta.ReadBlocks(fullPath, f.BlockIndices, f.BlockSize)
		if err != nil {
			entry.Error = "read blocks failed: " + err.Error()
		} else {
			entry.Blocks = encodeBlocks(blocks, gz)
		}
		out = append(out, entry)
	}
	jsonOK(w, map[string]any{"files": out})
}

// peerDeleteSnapshotEvery is how far apart two safety snapshots before a
// peer's deletions may be: one for a burst of deletions, however many files
// it removes one request at a time.
const peerDeleteSnapshotEvery = 10 * time.Minute

// snapshotBeforePeerDeletions takes a safety snapshot of a game before the
// first of a burst of deletions a peer asks for.
func (e *Engine) snapshotBeforePeerDeletions(gameID string, r *http.Request) {
	if e.Snapshots == nil {
		return
	}
	e.peerDeleteSnapMu.Lock()
	if e.peerDeleteSnap == nil {
		e.peerDeleteSnap = map[string]time.Time{}
	}
	if last, ok := e.peerDeleteSnap[gameID]; ok && time.Since(last) < peerDeleteSnapshotEvery {
		e.peerDeleteSnapMu.Unlock()
		return
	}
	e.peerDeleteSnap[gameID] = time.Now()
	e.peerDeleteSnapMu.Unlock()

	asker := "another device"
	if peer, ok := e.peerByAddress(clientIP(r)); ok {
		asker = peer.Name
	}
	if _, err := e.Snapshots.CreateBeforeReplacing(gameID, fmt.Sprintf("Before %s deleted files here", asker)); err != nil {
		e.Log("warn", fmt.Sprintf("safety snapshot before %s's deletions failed: %v", asker, err))
	}
}

func (e *Engine) handleDeleteFile(w http.ResponseWriter, r *http.Request) {
	gameID := chi.URLParam(r, "gameId")
	var body struct {
		RelPath string `json:"relPath"`
		Root    string `json:"root"`
		// Versioned: the asking device keeps save versions. One that does
		// not decides deletions by the old file comparison, which reads
		// whatever its copy lacks as deleted — the deletions that emptied
		// whole saves when a copy had arrived only half-way.
		Versioned bool `json:"versioned"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.RelPath == "" {
		jsonError(w, http.StatusBadRequest, "relPath is required.")
		return
	}

	game, err := e.trackedGameForPeer(gameID)
	if err != nil {
		jsonError(w, http.StatusNotFound, "Game not found.")
		return
	}
	if e.Sync.PlayingHere(game.ID) {
		jsonError(w, http.StatusServiceUnavailable, syncengine.PlayingMessage)
		return
	}
	if game.AutoSync && !body.Versioned {
		from := clientIP(r)
		if p, ok := e.peerByAddress(from); ok {
			from = p.Name
		}
		e.Sync.RefusedOldBuildDelete(game.Name, from)
		jsonError(w, http.StatusConflict, syncengine.OldBuildDeleteMessage)
		return
	}
	base, ok := e.resolveServeRoot(gameID, game, body.Root)
	if !ok {
		jsonError(w, http.StatusNotFound, "This device has no save location named "+strconv.Quote(body.Root)+" for that game.")
		return
	}
	if !delta.IsSafePath(base, body.RelPath) {
		jsonError(w, http.StatusBadRequest, "invalid path")
		return
	}

	// Resolved, not joined: this device's manifest advertises the agreed
	// (composed) spelling, while the file on this disk may be stored
	// decomposed — a macOS save. Joining the key verbatim would fail to
	// find the very file this device just offered.
	full := delta.LocalNameFor(base, body.RelPath)
	_ = os.Chmod(full, 0o666)
	deleting := time.Now()
	if info, statErr := os.Stat(full); statErr == nil {
		// One copy of the save before a peer starts deleting from it, not
		// one per file: the watcher no longer snapshots changes OpenSave
		// applies (owntouch), and it was those, one every few seconds of a
		// half-deleted save, that had been the only copy before.
		e.snapshotBeforePeerDeletions(game.ID, r)
		// What is removed is remembered, so a sync of this device's own that
		// lands before the rest of the batch does not take it for a change
		// made here (syncengine/peerdeleted.go).
		var entry delta.FileEntry
		if !info.IsDir() {
			entry, _ = delta.FileEntryFor(full)
		}
		// Empty dirs only, for a folder, like rmdirSync.
		// A change made at a peer's request, not by the game (owntouch).
		owntouch.MarkRemoved(full)
		if os.Remove(full) == nil && (info.IsDir() || entry.Hash != "") {
			e.Sync.NotePeerDeletion(game.ID, body.Root, body.RelPath, entry, info.IsDir())
		}
		e.Log("info", fmt.Sprintf("peer-requested deletion applied: %s", body.RelPath))
		asker := "another device"
		if peer, ok := e.peerByAddress(clientIP(r)); ok {
			asker = peer.Name
		}
		e.Sync.NoteEmptiedByPeer(gameID, asker, deleting)

		// This side just changed without running a sync, so nothing has
		// updated its merge-base — it still describes a state that contains
		// the file that was removed. A base behind both sides does not merely
		// go stale, it manufactures conflicts: the next ordinary one-sided
		// edit here reads as a two-way divergence and prompts on a save the
		// peer never touched. Re-derive it now rather than waiting for some
		// later sync to happen along and put it right.
		if peer, ok := e.peerByAddress(clientIP(r)); ok {
			e.refreshLineageAfterDeletion(gameID, peer)
		}
	}
	jsonOK(w, map[string]any{"success": true})
}

// refreshLineageAfterDeletion re-reads the peer's manifest and re-records the
// shared lineage, ratcheting the merge-base when both sides now match. Run in
// the background: the peer is waiting on the delete response, and a deletion
// that reported failure because the follow-up fetch was slow would be worse
// than the stale bookkeeping this exists to prevent.
func (e *Engine) refreshLineageAfterDeletion(gameID string, peer syncengine.Peer) {
	key := gameID + "\x00" + peer.ID
	e.deleteRefreshMu.Lock()
	if e.deleteRefresh == nil {
		e.deleteRefresh = map[string]bool{}
	}
	if _, running := e.deleteRefresh[key]; running {
		// Coalesce, but do not drop: the refresh already in flight may have
		// read the peer's manifest before this deletion was applied, so
		// letting it stand would leave the base describing a file that is now
		// gone — the very thing this exists to prevent. Ask it to run once
		// more instead.
		e.deleteRefresh[key] = true
		e.deleteRefreshMu.Unlock()
		return
	}
	e.deleteRefresh[key] = false
	e.deleteRefreshMu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		for {
			e.Sync.RefreshLineage(ctx, gameID, peer)

			e.deleteRefreshMu.Lock()
			again := e.deleteRefresh[key]
			if !again {
				delete(e.deleteRefresh, key)
				e.deleteRefreshMu.Unlock()
				return
			}
			e.deleteRefresh[key] = false // consume the request and go round again
			e.deleteRefreshMu.Unlock()
		}
	}()
}

func (e *Engine) handleSyncEvent(w http.ResponseWriter, r *http.Request) {
	gameID := chi.URLParam(r, "gameId")
	var body struct {
		EventType string         `json:"eventType"`
		Data      map[string]any `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid body")
		return
	}

	ev := progressEventFromMap(body.Data)
	switch body.EventType {
	case "sync-start":
		if e.Sync.Progress.OnSyncStart != nil {
			e.Sync.Progress.OnSyncStart(gameID, ev)
		}
	case "sync-progress":
		if e.Sync.Progress.OnSyncProgress != nil {
			e.Sync.Progress.OnSyncProgress(gameID, ev)
		}
	case "sync-complete":
		if e.Sync.Progress.OnSyncComplete != nil {
			e.Sync.Progress.OnSyncComplete(gameID, ev)
		}
		if peer, ok := e.peerByAddress(clientIP(r)); ok {
			e.peerFinishedPulling(gameID, peer, body.Data)
		}
	case "in-sync":
		// The peer verified both sides hold identical content; confirm on
		// our side (hash re-check) before recording lineage + last-synced.
		claimedHash, _ := body.Data["manifestHash"].(string)
		if peer, ok := e.peerByAddress(clientIP(r)); ok {
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				defer cancel()
				e.Sync.ConfirmInSync(ctx, gameID, peer, claimedHash)
			}()
		}
	case "sync-error":
		if e.Sync.Progress.OnSyncError != nil {
			e.Sync.Progress.OnSyncError(gameID, ev)
		}
		// A sync here waiting for this peer to finish need not wait on.
		if peer, ok := e.peerByAddress(clientIP(r)); ok {
			e.Sync.NotePeerPulled(gameID, peer.ID)
		}
	}
	jsonOK(w, map[string]any{"success": true})
}

// peerByAddress finds a paired peer by its request IP, as a sync-engine
// peer descriptor.
func (e *Engine) peerByAddress(ip string) (syncengine.Peer, bool) {
	peers, err := e.Store.ListPeers()
	if err != nil {
		return syncengine.Peer{}, false
	}
	for _, p := range peers {
		if p.Address == ip {
			return syncengine.Peer{ID: p.ID, Name: p.Name, Address: p.Address, Port: p.Port, IsWan: p.Address == "relay"}, true
		}
	}
	return syncengine.Peer{}, false
}

// handleSyncTrigger is the reverse-sync endpoint a peer calls when it has
// newer content for us to pull.
func (e *Engine) handleSyncTrigger(w http.ResponseWriter, r *http.Request) {
	gameID := chi.URLParam(r, "gameId")
	e.GoSync(func(ctx context.Context) {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		defer cancel()
		if _, err := e.SyncGame(ctx, gameID); err != nil && !errors.Is(err, syncengine.ErrHeld) {
			e.Log("warn", fmt.Sprintf("triggered sync for %s: %v", gameID, err))
		}
	})
	jsonOK(w, map[string]any{"status": "triggered"})
}

func progressEventFromMap(data map[string]any) syncengine.ProgressEvent {
	raw, _ := json.Marshal(data)
	var ev syncengine.ProgressEvent
	_ = json.Unmarshal(raw, &ev)
	return ev
}

// jsonOKCompressed is jsonOK, gzipped when the asker accepts it.
//
// For manifests. A save of a few hundred thousand files makes a manifest of
// tens of MB of JSON, which over a VPN did not arrive inside a peer's request
// timeout — so the peer never learned what this device holds, every sync of
// that game failed, and the two devices never converged. Manifest JSON (paths
// and hex hashes) compresses several times over. Go's HTTP client asks for
// gzip and unpacks it on its own unless told not to, so every version of
// OpenSave reads these, including ones that predate this.
func jsonOKCompressed(w http.ResponseWriter, r *http.Request, v any) {
	if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		jsonOK(w, v)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Add("Vary", "Accept-Encoding")
	w.WriteHeader(http.StatusOK)
	zw, err := gzip.NewWriterLevel(w, gzip.BestSpeed)
	if err != nil {
		return
	}
	_ = json.NewEncoder(zw).Encode(v)
	_ = zw.Close()
}

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// resolveServeRoot maps a root name from a peer request to this device's
// path for it, defaulting to the game's primary location.
//
// An unknown or unmapped name is an error rather than a fallback to the
// primary path. Falling back would mean serving — or worse, deleting — a
// file in the save folder because a peer asked about a location this device
// does not have, which is precisely the "files end up somewhere they should
// not" failure the root name exists to prevent.
func (e *Engine) resolveServeRoot(gameID string, game store.Game, root string) (string, bool) {
	if root == delta.PrimaryRoot {
		return game.SavePath, true
	}
	paths, err := e.Store.GameRootPaths(gameID)
	if err != nil {
		return "", false
	}
	path, ok := paths[root]
	return path, ok
}

// servedProto is the sync protocol revision this device advertises.
//
// Atomic, not a plain var: it is read on request goroutines while a test
// writes it, and the race detector is right to object. The value is only ever
// changed by tests, but "only tests write it" is not a synchronisation
// argument — the read still happens concurrently.
var servedProto atomic.Int64

func init() { servedProto.Store(syncengine.ProtoVersions) }

// ServedProto reports the protocol revision advertised to peers.
func ServedProto() int { return int(servedProto.Load()) }

// SetServedProto overrides the advertised revision and returns the previous
// value, so a test can restore it.
//
// This exists for one reason: lowering it to 0 makes a real daemon answer
// exactly as a build that predates multi-root does, which is the only way to
// get an older peer into an end-to-end test. Nothing in the product calls it.
func SetServedProto(v int) int {
	return int(servedProto.Swap(int64(v)))
}

// peerFinishedPulling handles a peer's report that it finished pulling from
// this device, over the LAN or the relay alike: whatever was pushed is now on
// both sides, so the shared lineage is brought up to date. Until it is,
// freshly-pushed files deliberately stay out of it (see persistLineage), so
// deleting one here would pull it back instead of propagating the delete.
func (e *Engine) peerFinishedPulling(gameID string, peer syncengine.Peer, data map[string]any) {
	// A sync here waiting for this peer before slower ones goes on.
	defer e.Sync.NotePeerPulled(gameID, peer.ID)
	// Recorded first, and synchronously: the peer said exactly which files it
	// wrote, so the lineage can be updated now rather than after a manifest
	// round trip. That round trip is what left a window in which deleting a
	// just-synced file pulled it back instead of propagating the delete.
	//
	// Against the location it names. A report without one is from a version
	// that did not say, and means the main save folder, as it always has.
	took := stringsFromEventData(data, "pulledFiles")
	root, _ := data["root"].(string)
	e.Sync.AddConfirmedLineageForRoot(gameID, peer.ID, root, took)
	if len(took) > 0 {
		e.Sync.RecordActivity(store.ActivityEvent{GameID: e.localGameID(gameID), Kind: store.ActivitySent, Device: peer.Name, Files: len(took)})
	}
	refreshAfterPull(e, gameID, peer)
}

// refreshAfterPull re-checks the lineage against the peer's manifest after a
// report, in the background so the report is answered at once. A variable so
// a test can run it in line and look at the result without racing it.
var refreshAfterPull = func(e *Engine, gameID string, peer syncengine.Peer) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		e.Sync.RefreshLineage(ctx, gameID, peer)
	}()
}

// stringsFromEventData pulls a []string out of a sync-event payload, which
// arrives as []any after a JSON round trip.
func stringsFromEventData(data map[string]any, key string) []string {
	raw, ok := data[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

// holdingBack says whether this device holds a game back from its peers
// because its save was emptied here (syncengine/hold.go). A peer asking for
// its manifest is exactly the moment an emptied folder would be read as
// every file deleted, so this is checked here and not only when this device
// starts a sync.
func (e *Engine) holdingBack(game store.Game) bool {
	if e.Sync == nil {
		return false
	}
	held, err := e.Sync.CheckHold(game.ID, true)
	return err == nil && held
}
