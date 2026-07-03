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

// Start listens on 127.0.0.1:<port> (port 0 picks a free one) and serves
// until Stop. Returns the bound address.
func (s *Server) Start(port int) (string, error) {
	r := chi.NewRouter()
	r.Use(localhostOnly)
	s.routes(r)

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
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
	games := s.gamesPayload()
	return map[string]any{
		"settings":        settings,
		"games":           games,
		"peers":           map[string]any{},
		"discoveredPeers": []any{},
		"pairingRequests": []any{},
		"wanRoom":         nil,
		"conflicts":       []any{},
		"logHistory":      s.Daemon.Log.History(),
	}
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
