package p2p

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/opensave/opensave/internal/delta"
	"github.com/opensave/opensave/internal/p2p/pairing"
	"github.com/opensave/opensave/internal/p2p/syncengine"
	"github.com/opensave/opensave/internal/store"
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
		r.Get("/api/p2p/manifest/{gameId}", e.handleManifest)
		r.Post("/api/p2p/blocks/{gameId}", e.handleBlocks)
		r.Post("/api/p2p/delete-file/{gameId}", e.handleDeleteFile)
		r.Post("/api/p2p/sync-event/{gameId}", e.handleSyncEvent)
		r.Get("/api/sync/trigger/{gameId}", e.handleSyncTrigger)
	})
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

// requirePairedPeer allows localhost (dashboard/CLI) plus IPs matching a
// paired peer. A valid request from a paired peer also refreshes its
// online status, and a peer coming back online triggers a full auto-sync
// (matching the JS guard's side effects).
func (e *Engine) requirePairedPeer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if isLoopbackIP(ip) {
			next.ServeHTTP(w, r)
			return
		}

		peers, err := e.Store.ListPeers()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "peer lookup failed")
			return
		}
		var matched *store.Peer
		for i := range peers {
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

		// Throttled online-status refresh (10s), with auto-sync on
		// offline->online transition.
		const lastSeenLimit = 10_000
		now := time.Now().UnixMilli()
		if matched.Status != "online" || now-matched.LastSeenMs > lastSeenLimit {
			wasOffline := matched.Status != "online"
			matched.Status = "online"
			matched.LastSeenMs = now
			_ = e.Store.UpsertPeer(*matched)
			if wasOffline {
				e.Log("info", fmt.Sprintf("peer %q connected; triggering auto-sync for all games", matched.Name))
				go e.SyncAllGames(context.Background())
				e.notifyPeerUpdate()
			}
		}
		next.ServeHTTP(w, r)
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
	}
	jsonOK(w, map[string]any{
		"status":     "ok",
		"paired":     paired,
		"deviceName": settings.DeviceName,
		"deviceType": settings.DeviceType,
		"games":      e.LocalGamesState(),
	})
}

func (e *Engine) handleHandshake(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PeerID     string `json:"peerId"`
		DeviceName string `json:"deviceName"`
		DeviceType string `json:"deviceType"`
		Port       int    `json:"port"`
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

	e.Log("success", fmt.Sprintf("pairing confirmed with %q (%s:%d)", body.DeviceName, ip, body.Port))
	e.notifyPeerUpdate()
	jsonOK(w, map[string]any{"success": true, "message": "Pairing confirmed."})
}

func (e *Engine) handleUnpair(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PeerID string `json:"peerId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.PeerID == "" {
		jsonError(w, http.StatusBadRequest, "peerId is required")
		return
	}
	_ = e.Store.UnpairPeer(body.PeerID)
	e.notifyPeerUpdate()
	jsonOK(w, map[string]any{"success": true})
}

// handleManifest serves a game's manifest + branch + latest-snapshot info.
// If the game isn't tracked here yet, it is auto-tracked using the
// requester's supplied name/savePath (translated to local conventions).
func (e *Engine) handleManifest(w http.ResponseWriter, r *http.Request) {
	gameID := chi.URLParam(r, "gameId")

	game, err := e.Store.GetGame(gameID)
	if err != nil {
		name := r.URL.Query().Get("name")
		remotePath := r.URL.Query().Get("savePath")
		if name == "" || remotePath == "" {
			jsonError(w, http.StatusNotFound, "Game not found.")
			return
		}
		settings, sErr := e.Store.GetSettings()
		if sErr != nil {
			jsonError(w, http.StatusInternalServerError, sErr.Error())
			return
		}
		rules := make([]delta.TranslationRule, len(settings.PathTranslations))
		for i, tr := range settings.PathTranslations {
			rules[i] = delta.TranslationRule{FromPattern: tr.FromPattern, ToPattern: tr.ToPattern}
		}
		localPath := delta.TranslatePathToLocal(remotePath, rules)

		game = store.Game{ID: gameID, Name: name, SavePath: localPath, ActiveBranch: "main", AutoSync: true, MaxSnapshots: 5}
		if err := e.Store.CreateGame(game); err != nil {
			jsonError(w, http.StatusInternalServerError, "auto-track failed: "+err.Error())
			return
		}
		if r.URL.Query().Get("isFile") == "true" {
			_ = os.MkdirAll(filepath.Dir(localPath), 0o777)
		} else {
			_ = os.MkdirAll(localPath, 0o777)
		}
		e.Log("info", fmt.Sprintf("auto-tracked %q at %q from peer manifest request", name, localPath))
	}

	manifest, err := delta.BuildManifest(game.SavePath)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "manifest build failed: "+err.Error())
		return
	}

	resp := syncengine.ManifestResponse{Manifest: manifest, ActiveBranch: game.ActiveBranch}
	if latest, err := e.Snapshots.LatestSnapshot(gameID, ""); err == nil {
		resp.LatestSnapshot = &syncengine.SnapshotInfo{ID: latest.ID, Timestamp: latest.Timestamp, Comment: latest.Comment}
	}
	jsonOK(w, resp)
}

func (e *Engine) handleBlocks(w http.ResponseWriter, r *http.Request) {
	gameID := chi.URLParam(r, "gameId")
	var body struct {
		RelPath      string `json:"relPath"`
		BlockIndices []int  `json:"blockIndices"`
		BlockSize    int    `json:"blockSize"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.RelPath == "" {
		jsonError(w, http.StatusBadRequest, "relPath is required")
		return
	}

	game, err := e.Store.GetGame(gameID)
	if err != nil {
		jsonError(w, http.StatusNotFound, "Game not found.")
		return
	}
	if !delta.IsSafePath(game.SavePath, body.RelPath) {
		jsonError(w, http.StatusBadRequest, "invalid path")
		return
	}

	fullPath := filepath.Join(game.SavePath, filepath.FromSlash(body.RelPath))
	if isFile, _ := delta.ResolveLocalSaveFilePath(game.SavePath); isFile {
		fullPath = game.SavePath
	}

	blocks, err := delta.ReadBlocks(fullPath, body.BlockIndices, body.BlockSize)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "read blocks failed: "+err.Error())
		return
	}

	out := make([]syncengine.BlockData, len(blocks))
	for i, b := range blocks {
		out[i] = syncengine.BlockData{Index: b.Index, Data: b.Data, Length: len(b.Data)}
	}
	jsonOK(w, map[string]any{"blocks": out})
}

func (e *Engine) handleDeleteFile(w http.ResponseWriter, r *http.Request) {
	gameID := chi.URLParam(r, "gameId")
	var body struct {
		RelPath string `json:"relPath"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.RelPath == "" {
		jsonError(w, http.StatusBadRequest, "relPath is required.")
		return
	}

	game, err := e.Store.GetGame(gameID)
	if err != nil {
		jsonError(w, http.StatusNotFound, "Game not found.")
		return
	}
	if !delta.IsSafePath(game.SavePath, body.RelPath) {
		jsonError(w, http.StatusBadRequest, "invalid path")
		return
	}

	full := filepath.Join(game.SavePath, filepath.FromSlash(body.RelPath))
	_ = os.Chmod(full, 0o666)
	if info, statErr := os.Stat(full); statErr == nil {
		if info.IsDir() {
			_ = os.Remove(full) // empty dirs only, like rmdirSync
		} else {
			_ = os.Remove(full)
		}
		e.Log("info", fmt.Sprintf("peer-requested deletion applied: %s", body.RelPath))
	}
	jsonOK(w, map[string]any{"success": true})
}

func (e *Engine) handleSyncEvent(w http.ResponseWriter, r *http.Request) {
	gameID := chi.URLParam(r, "gameId")
	var body struct {
		EventType string                 `json:"eventType"`
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
	case "sync-error":
		if e.Sync.Progress.OnSyncError != nil {
			e.Sync.Progress.OnSyncError(gameID, ev)
		}
	}
	jsonOK(w, map[string]any{"success": true})
}

// handleSyncTrigger is the reverse-sync endpoint a peer calls when it has
// newer content for us to pull.
func (e *Engine) handleSyncTrigger(w http.ResponseWriter, r *http.Request) {
	gameID := chi.URLParam(r, "gameId")
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		if _, err := e.SyncGame(ctx, gameID); err != nil {
			e.Log("warn", fmt.Sprintf("triggered sync for %s: %v", gameID, err))
		}
	}()
	jsonOK(w, map[string]any{"status": "triggered"})
}

func progressEventFromMap(data map[string]any) syncengine.ProgressEvent {
	raw, _ := json.Marshal(data)
	var ev syncengine.ProgressEvent
	_ = json.Unmarshal(raw, &ev)
	return ev
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
