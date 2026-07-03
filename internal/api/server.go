// Package api serves the local REST + WebSocket dashboard API, keeping
// route paths and JSON shapes wire-compatible with the original JS daemon
// (so the Decky plugin and any external tooling keep working unchanged).
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/opensave/opensave/internal/daemon"
	"github.com/opensave/opensave/internal/logging"
	"github.com/opensave/opensave/internal/p2p/syncengine"
	"github.com/opensave/opensave/internal/store"
)

// Server hosts the REST API and dashboard WebSocket for one daemon.
type Server struct {
	Daemon *daemon.Daemon
	Hub    *Hub

	httpServer *http.Server
	listener   net.Listener
}

// New assembles the router and hub around a daemon.
func New(d *daemon.Daemon) *Server {
	s := &Server{Daemon: d, Hub: NewHub()}
	s.Hub.InitPayload = s.initPayload

	// Live-forward activity log entries to connected dashboards.
	d.Log.Subscribe(func(entry logging.Entry) {
		s.Hub.Broadcast("log", entry)
	})
	return s
}

// BroadcastGamesUpdate pushes the current games state to all dashboard
// clients (called after any mutation).
func (s *Server) BroadcastGamesUpdate() {
	s.Hub.Broadcast("games-update", s.gamesPayload())
}

// BroadcastPeersUpdate pushes the full peer/pairing state.
func (s *Server) BroadcastPeersUpdate() {
	s.Hub.Broadcast("peers-update", s.peersPayload())
}

func (s *Server) peersPayload() map[string]any {
	peers, _ := s.Daemon.Store.ListPeers()
	peerMap := map[string]any{}
	for _, p := range peers {
		peerMap[p.ID] = p
	}
	discovered := []any{}
	if s.Daemon.P2P.Discovery != nil {
		for _, d := range s.Daemon.P2P.Discovery.DiscoveredPeers() {
			discovered = append(discovered, d)
		}
	}
	return map[string]any{
		"peers":           peerMap,
		"discoveredPeers": discovered,
		"pairingRequests": s.Daemon.P2P.Pairing.PendingRequests(),
		"wanRoom":         nil, // Phase 3
		"conflicts":       s.Daemon.P2P.Sync.ActiveConflicts(),
	}
}

// wireSyncProgress forwards sync engine progress into the dashboard WS,
// using the same message types the JS daemon broadcast.
func (s *Server) wireSyncProgress() {
	sync := s.Daemon.P2P.Sync
	sync.Progress.OnSyncStart = func(gameID string, ev syncengine.ProgressEvent) {
		s.Hub.Broadcast("sync-start", map[string]any{"gameId": gameID, "data": ev})
	}
	sync.Progress.OnSyncProgress = func(gameID string, ev syncengine.ProgressEvent) {
		s.Hub.Broadcast("sync-progress", map[string]any{"gameId": gameID, "data": ev})
	}
	sync.Progress.OnSyncComplete = func(gameID string, ev syncengine.ProgressEvent) {
		s.Hub.Broadcast("sync-complete", map[string]any{"gameId": gameID, "data": ev})
		s.BroadcastGamesUpdate()
	}
	sync.Progress.OnSyncError = func(gameID string, ev syncengine.ProgressEvent) {
		s.Hub.Broadcast("sync-error", map[string]any{"gameId": gameID, "data": ev})
	}
	sync.Progress.OnConflict = func(gameID string) {
		s.BroadcastPeersUpdate()
	}
}

// Start listens on 0.0.0.0:<port> (port 0 picks a free one) and serves
// until Stop. Dashboard routes are localhost-guarded per-request; the
// /api/p2p/* peer protocol is LAN-reachable behind its own paired-peer
// guard. Returns the bound address.
func (s *Server) Start(port int) (string, error) {
	r := chi.NewRouter()

	// Dashboard/CLI surface: localhost only.
	r.Group(func(r chi.Router) {
		r.Use(localhostOnly)
		s.routes(r)
	})

	// Peer-to-peer protocol: LAN-reachable, guarded by requirePairedPeer
	// inside RegisterRoutes.
	s.Daemon.P2P.RegisterRoutes(r)

	// Peer/dashboard state changes push live updates.
	s.Daemon.P2P.OnPeerUpdate = s.BroadcastPeersUpdate
	s.wireSyncProgress()

	ln, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", port))
	if err != nil {
		return "", fmt.Errorf("listen: %w", err)
	}
	s.listener = ln
	s.httpServer = &http.Server{Handler: r}

	go func() {
		if err := s.httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.Daemon.Log.Log("error", fmt.Sprintf("api server: %v", err))
		}
	}()
	return ln.Addr().String(), nil
}

// Stop shuts the HTTP server down gracefully.
func (s *Server) Stop() {
	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.httpServer.Shutdown(ctx)
	}
}

// localhostOnly rejects any request that didn't originate from the local
// machine — the dashboard API must never be reachable from the network.
// (P2P peer routes get their own paired-peer middleware in Phase 2.)
func localhostOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil || !isLoopback(host) {
			writeError(w, http.StatusForbidden, "external access denied")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isLoopback(host string) bool {
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// initPayload is the full-state dump sent to a dashboard client on WS
// connect, matching the JS "init" message shape (peers/conflicts arrive
// with Phase 2 — empty placeholders keep the shape stable).
func (s *Server) initPayload() any {
	settings, _ := s.Daemon.Store.GetSettings()
	payload := s.peersPayload()
	payload["settings"] = settings
	payload["games"] = s.gamesPayload()
	payload["logHistory"] = s.Daemon.Log.History()
	return payload
}

// gamesPayload returns every game with its branches+snapshots nested the
// way the JS frontend expects (game.branches[name].snapshots[]).
func (s *Server) gamesPayload() map[string]any {
	games, err := s.Daemon.Store.ListGames()
	if err != nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(games))
	for _, g := range games {
		out[g.ID] = s.gamePayload(g)
	}
	return out
}

func (s *Server) gamePayload(g store.Game) map[string]any {
	branchNames, _ := s.Daemon.Store.ListBranches(g.ID)
	branches := map[string]any{}
	for _, name := range branchNames {
		snaps, _ := s.Daemon.Store.ListSnapshots(g.ID, name)
		// JS keeps snapshots oldest-first in the array; ListSnapshots is
		// newest-first, so reverse for wire compatibility.
		wireSnaps := make([]store.Snapshot, len(snaps))
		for i, snap := range snaps {
			wireSnaps[len(snaps)-1-i] = snap
		}
		branches[name] = map[string]any{"name": name, "snapshots": wireSnaps}
	}
	return map[string]any{
		"id":           g.ID,
		"name":         g.Name,
		"savePath":     g.SavePath,
		"activeBranch": g.ActiveBranch,
		"autoSync":     g.AutoSync,
		"maxSnapshots": g.MaxSnapshots,
		"appId":        g.AppID,
		"exePath":      g.ExePath,
		"coverUrl":     g.CoverURL,
		"branches":     branches,
		"createdAt":    g.CreatedAt,
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func readJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("invalid JSON body: %w", err)
	}
	return nil
}

func notFoundToStatus(err error) int {
	if errors.Is(err, store.ErrNotFound) || strings.Contains(err.Error(), "not found") {
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}
