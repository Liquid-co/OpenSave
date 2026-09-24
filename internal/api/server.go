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
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/opensave/opensave/internal/daemon"
	"github.com/opensave/opensave/internal/logging"
	"github.com/opensave/opensave/internal/p2p"
	"github.com/opensave/opensave/internal/p2p/syncengine"
	"github.com/opensave/opensave/internal/store"
	"github.com/opensave/opensave/internal/syncpause"
	"github.com/opensave/opensave/internal/transfers"
	"github.com/opensave/opensave/internal/version"
)

// Server hosts the REST API and dashboard WebSocket for one daemon.
type Server struct {
	Daemon *daemon.Daemon
	Hub    *Hub

	httpServer *http.Server
	listener   net.Listener
	// transfers remembers what moved between this device and others; fed
	// by the same progress reports the dashboard gets. See wireSyncProgress.
	transfers *transfers.Log
	// SteamCacheDirs overrides where Steam's already-downloaded library art is
	// looked for when non-nil. Tests only.
	SteamCacheDirs []string
}

// New assembles the router and hub around a daemon.
func New(d *daemon.Daemon) *Server {
	s := &Server{Daemon: d, Hub: NewHub(), transfers: transfers.New()}
	s.Hub.InitPayload = s.initPayload

	// Live-forward activity log entries to connected dashboards.
	d.Log.Subscribe(func(entry logging.Entry) {
		s.Hub.Broadcast("log", entry)
	})

	// Watcher auto-snapshots and async initial snapshots update the games
	// state outside any HTTP handler — push those to dashboards too.
	d.OnGameChanged = func(string) { s.BroadcastGamesUpdate() }

	// Saves in the cloud from other devices: the ones waiting for an answer,
	// and the ones taken without asking.
	d.OnCloudOffers = func(offers []daemon.CloudOffer) { s.Hub.Broadcast("cloud-offers", offers) }
	d.OnCloudPulled = func(p daemon.CloudPulled) { s.Hub.Broadcast("cloud-pulled", p) }

	// Newly installed games the background scan found.
	d.OnNewGames = func(games []daemon.NewGame) { s.Hub.Broadcast("new-games", games) }

	// Syncing paused or resumed — from here, the CLI, the tray, or a timer.
	d.P2P.Pause.OnChange(func(st syncpause.Status) { s.Hub.Broadcast("sync-pause", st) })
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
	builds := s.Daemon.P2P.PeerBuilds()
	peerMap := map[string]any{}
	for _, p := range peers {
		// Attach the peer's live app build info so the UI can offer
		// "update from this device" when a peer runs a newer build.
		entry := struct {
			store.Peer
			AppVersion    string `json:"appVersion,omitempty"`
			BuildTimeMs   int64  `json:"buildTimeMs,omitempty"`
			HasNewerBuild bool   `json:"hasNewerBuild,omitempty"`
			// What actually protects traffic with this device. Sent per peer
			// rather than described once in the interface, because the answer
			// differs per pairing and the reader cannot work out which case
			// they are in from a general statement.
			p2p.PeerProtection
		}{Peer: p, PeerProtection: s.Daemon.P2P.PeerProtection(p)}
		if b, ok := builds[p.ID]; ok {
			entry.AppVersion = b.AppVersion
			entry.BuildTimeMs = b.BuildTimeMs
			entry.HasNewerBuild = version.NewerThanLocal(b.AppVersion, b.BuildTimeMs)
		}
		peerMap[p.ID] = entry
	}
	discovered := []any{}
	if s.Daemon.P2P.Discovery != nil {
		for _, d := range s.Daemon.P2P.Discovery.DiscoveredPeers() {
			discovered = append(discovered, d)
		}
	}
	// WAN room members appear in the discovered list alongside LAN ones.
	for _, d := range s.Daemon.P2P.Wan.DiscoveredWanPeers() {
		discovered = append(discovered, d)
	}
	return map[string]any{
		"peers":             peerMap,
		"discoveredPeers":   discovered,
		"pairingRequests":   s.Daemon.P2P.Pairing.PendingRequests(),
		"wanRoom":           s.Daemon.P2P.Wan.Status(),
		"conflicts":         s.Daemon.P2P.Sync.ActiveConflicts(),
		"locationConflicts": s.Daemon.P2P.Sync.ActiveRootConflicts(),
	}
}

// wireSyncProgress forwards sync engine progress into the dashboard WS,
// using the same message types the JS daemon broadcast.
func (s *Server) wireSyncProgress() {
	sync := s.Daemon.P2P.Sync
	sync.Progress.OnSyncStart = func(gameID string, ev syncengine.ProgressEvent) {
		s.transfers.Started(gameID, transferEvent(ev))
		s.Hub.Broadcast("sync-start", map[string]any{"gameId": gameID, "data": ev})
	}
	sync.Progress.OnSyncProgress = func(gameID string, ev syncengine.ProgressEvent) {
		s.transfers.Progressed(gameID, transferEvent(ev))
		s.Hub.Broadcast("sync-progress", map[string]any{"gameId": gameID, "data": ev})
	}
	sync.Progress.OnSyncComplete = func(gameID string, ev syncengine.ProgressEvent) {
		s.transfers.Finished(gameID, transferEvent(ev))
		s.Hub.Broadcast("sync-complete", map[string]any{"gameId": gameID, "data": ev})
		s.BroadcastGamesUpdate()
	}
	sync.Progress.OnSyncError = func(gameID string, ev syncengine.ProgressEvent) {
		if ev.Error == "" {
			ev.Error = "the sync failed"
		}
		s.transfers.Finished(gameID, transferEvent(ev))
		s.Hub.Broadcast("sync-error", map[string]any{"gameId": gameID, "data": ev})
	}
	sync.Progress.OnConflict = func(gameID string) {
		s.BroadcastPeersUpdate()
	}
	// A peer finished pulling from us, or confirmed we match: the game's
	// last-synced time moved with no sync running here to announce it.
	sync.Progress.OnSyncConfirmed = func(gameID string) {
		s.BroadcastGamesUpdate()
	}
}

func transferEvent(ev syncengine.ProgressEvent) transfers.Event {
	return transfers.Event{
		Peer: ev.PeerName, Direction: ev.Direction,
		BytesTransferred: ev.BytesTransferred, TotalBytes: ev.TotalBytes,
		SpeedBytesPerSec: ev.SpeedBytesPerSec, Percentage: ev.Percentage,
		Error: ev.Error,
	}
}

// Start listens on 0.0.0.0:<port> (port 0 picks a free one) and serves
// until Stop. Dashboard routes are localhost-guarded per-request; the
// /api/p2p/* peer protocol is LAN-reachable behind its own paired-peer
// guard. Returns the bound address.
func (s *Server) Start(port int) (string, error) {
	r := chi.NewRouter()

	// CORS + preflight handling must be a TOP-LEVEL middleware: chi answers
	// an unmatched method (the browser's OPTIONS preflight) with 405 before
	// group middleware runs, so handling it inside the group would never
	// fire — which silently blocked every POST/PATCH/DELETE from the
	// webview with "Failed to fetch".
	r.Use(corsLocalhost)

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
	s.Daemon.P2P.OnGamesUpdate = s.BroadcastGamesUpdate
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
	s.writeAddrFile(ln.Addr().String())
	s.recordBoundPort(ln.Addr().String())
	return ln.Addr().String(), nil
}

// recordBoundPort stores the port actually listening in settings.
//
// Pairing hands the other device settings.Port to call back on, and the
// callback is what completes the handshake on the initiating side. When the
// configured port is taken the daemon falls back to an ephemeral one, so
// without this the initiator advertises a port nothing is listening on: the
// approval succeeds on the device that granted it, its approve-confirm goes
// nowhere, and the device that started the pairing is left showing no peers
// at all. Internet sync still works in that state, because relay-routed peers
// are never addressed by port — which makes it look like a LAN-only fault.
func (s *Server) recordBoundPort(addr string) {
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return
	}
	bound, err := strconv.Atoi(portStr)
	if err != nil || bound <= 0 {
		return
	}
	settings, err := s.Daemon.Store.GetSettings()
	if err != nil || settings.Port == bound {
		return
	}
	previous := settings.Port
	settings.Port = bound
	if err := s.Daemon.Store.UpdateSettings(settings); err != nil {
		s.Daemon.Log.Log("warn", fmt.Sprintf("could not record the listening port %d: %v", bound, err))
		return
	}
	s.Daemon.Log.Log("info", fmt.Sprintf(
		"listening on port %d (configured %d was unavailable); peers will be told to use %d",
		bound, previous, bound))
}

// addrFilePath is where the running daemon publishes the address it actually
// bound. The configured port can be taken (another instance, or an app already
// running), in which case the daemon falls back to an ephemeral port — so the
// configured value is not a reliable way to find it. Out-of-process clients,
// notably the Steam Deck's Decky plugin running in Game Mode, read this.
func (s *Server) addrFilePath() string {
	return filepath.Join(s.Daemon.Paths.HomeDir, "daemon.addr")
}

// writeAddrFile publishes the loopback address clients should dial. The
// listener binds 0.0.0.0 so LAN peers can reach the P2P routes, but
// "0.0.0.0" is not a connectable host — rewrite it the way the desktop app
// does before handing it out.
func (s *Server) writeAddrFile(addr string) {
	if _, port, err := net.SplitHostPort(addr); err == nil {
		addr = "127.0.0.1:" + port
	}
	if err := os.WriteFile(s.addrFilePath(), []byte(addr), 0o666); err != nil {
		s.Daemon.Log.Log("warn", "could not publish daemon address: "+err.Error())
	}
}

// Stop shuts the HTTP server down gracefully.
func (s *Server) Stop() {
	// Remove the published address first: a stale file points clients at a
	// port nothing is listening on.
	_ = os.Remove(s.addrFilePath())
	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.httpServer.Shutdown(ctx)
	}
}

// corsLocalhost adds permissive CORS headers for local requests and
// answers preflight OPTIONS directly (204). The Wails webview runs at its
// own origin (http://wails.localhost), so without this the browser blocks
// every non-simple request. Runs at the top level so the OPTIONS preflight
// is handled before chi's per-route method matching returns 405.
// allowedBrowserOrigins are the only web origins that may call this API.
//
// The app's own window is a browser: Wails serves the interface from these
// origins and it fetches the API cross-origin, so they need CORS headers. No
// other page does. The API used to answer every loopback request with
// Access-Control-Allow-Origin: *, and loopback is not a boundary a browser
// respects — a page on any website the user has open runs on this machine
// too, and could read the settings (node ID, room code, relay URL), untrack
// games, restore an old snapshot over a current save, or point the relay
// setting at a server of its choosing. The default port is 8383, so there
// was nothing to find first.
//
// The Steam Deck plugin and the CLI are not browsers: they send no Origin
// header at all, and requests without one pass untouched.
var allowedBrowserOrigins = map[string]bool{
	"http://wails.localhost": true, // Windows: WebView2 cannot use a custom scheme
	"wails://wails":          true, // macOS and Linux
	"http://localhost:34115": true, // `wails dev`
}

// corsLocalhost handles browser cross-origin rules for the API.
//
// A request carrying an Origin this API does not recognise is REFUSED, not
// merely denied CORS headers. Withholding the headers only stops the page
// reading the reply; a "simple" request — a POST with no custom headers —
// has already executed on the server by then, and several routes here act
// on a bare POST. Refusing at the door is the only version that holds.
func corsLocalhost(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// On every response, including the ones to a request with no Origin.
		// The headers below depend on the Origin, so a cached copy made without
		// one must not be handed to a request that has one. Covers are cached
		// for a week, and the app loads each one both as an <img> (no Origin)
		// and with fetch (Origin): the <img>'s copy, reused for the fetch,
		// fails the fetch's CORS check, and every retry after it.
		w.Header().Add("Vary", "Origin")
		origin := r.Header.Get("Origin")
		if origin != "" {
			if !allowedBrowserOrigins[origin] {
				writeError(w, http.StatusForbidden, "cross-origin access denied")
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// localhostOnly rejects any request that didn't originate from the local
// machine — the dashboard API must never be reachable from the network.
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
// connect, matching the JS "init" message shape.
func (s *Server) initPayload() any {
	payload := s.peersPayload()
	payload["settings"] = s.settingsWire()
	payload["games"] = s.gamesPayload()
	payload["logHistory"] = s.Daemon.Log.History()
	payload["cloudOffers"] = s.Daemon.CloudOffers()
	payload["newGames"] = s.Daemon.NewGames()
	payload["syncPause"] = s.Daemon.SyncPauseStatus()
	return payload
}

// settingsWire returns settings in the JS wire shape: the flat settings
// fields plus a nested cloudSync object. OAuth tokens are masked — the
// frontend only needs userEmail to show the connected account.
func (s *Server) settingsWire() map[string]any {
	settings, err := s.Daemon.Store.GetSettings()
	if err != nil {
		return map[string]any{}
	}
	raw, _ := json.Marshal(settings)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)

	cloud, err := s.Daemon.Store.GetCloudConfig()
	if err == nil {
		out["cloudSync"] = map[string]any{
			"enabled":             cloud.Enabled,
			"provider":            cloud.Provider,
			"url":                 cloud.URL,
			"username":            cloud.Username,
			"password":            cloud.Password,
			"headers":             cloud.HeadersJSON,
			"folderId":            cloud.FolderID,
			"customClientIds":     cloud.CustomClientIDs,
			"customClientSecrets": cloud.CustomClientSecrets,
			"tokens": map[string]any{
				"accessToken":  "", // never shipped to the UI
				"refreshToken": "",
				"expiryTime":   cloud.ExpiryTimeMs,
				"userEmail":    cloud.UserEmail,
			},
		}
	}
	return out
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
	// When this game was last confirmed the same as each paired device's
	// copy: peer id to ISO 8601. Never an error to the client — a game with
	// nothing recorded and a game whose lookup failed both show "never",
	// which is the honest answer in both cases.
	lastSyncedWith, _ := s.Daemon.Store.GameLastSynced(g.ID)
	if lastSyncedWith == nil {
		lastSyncedWith = map[string]string{}
	}
	return map[string]any{
		"id":                 g.ID,
		"name":               g.Name,
		"savePath":           g.SavePath,
		"activeBranch":       g.ActiveBranch,
		"autoSync":           g.AutoSync,
		"maxSnapshots":       g.MaxSnapshots,
		"maxManualSnapshots": g.MaxManualSnapshots,
		"appId":              g.AppID,
		"exePath":            g.ExePath,
		"coverUrl":           g.CoverURL,
		// Whether the art cached for this game is explicit, so the client can
		// blur it until someone asks to see it. An <img src> cannot read a
		// response header, so it travels with the game rather than the image.
		"coverExplicit":  s.CoverIsExplicit(coverKeyFor(g)),
		"syncIgnore":     g.SyncIgnore,
		"branches":       branches,
		"createdAt":      g.CreatedAt,
		"lastSyncedWith": lastSyncedWith,
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
