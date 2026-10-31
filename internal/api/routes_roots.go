package api

import (
	"context"
	"errors"
	"time"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
)

// A game's extra save locations: the folders beyond its main one that belong
// to the same title, so a save split between (say) AppData and Documents is
// one game rather than two.

func (s *Server) handleListGameRoots(w http.ResponseWriter, r *http.Request) {
	gameID := chi.URLParam(r, "gameId")
	roots, err := s.Daemon.Store.ListGameRoots(gameID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if roots == nil {
		roots = nil // marshals as [], not null, via the slice below
	}
	out := make([]map[string]any, 0, len(roots))
	for _, root := range roots {
		out = append(out, map[string]any{
			"name": root.Name,
			"path": root.Path,
			// Whether this device knows where the location actually is. A
			// location learned from a peer arrives named but not placed, and
			// the UI has to be able to say so and ask.
			"mapped": root.Mapped(),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleAddGameRoot(w http.ResponseWriter, r *http.Request) {
	gameID := chi.URLParam(r, "gameId")
	var body struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := s.Daemon.Store.GetGame(gameID); err != nil {
		writeError(w, http.StatusNotFound, "Game not found.")
		return
	}
	// A save location is checked the same way a save path is. An extra
	// location is watched, hashed, synced to every peer and uploaded to the
	// cloud, so pointing one at a profile folder syncs a user's Documents to
	// their other devices — and a restore then refuses to clear it, leaving
	// the game unrestorable. The CLI has always run this check; the API went
	// straight to the store, so the same folder was accepted here and refused
	// there.
	//
	// An empty path is how a location learned from a peer is recorded before
	// this device knows where it lives, and must stay allowed.
	path := body.Path
	if strings.TrimSpace(path) != "" {
		abs, err := s.Daemon.CheckRestoreTarget(path)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		path = abs
	}
	// A root that is currently being written by an in-flight transfer must
	// not be remapped out from under that transfer (issue #60).
	done, ok := s.acquireRootTopology(w, r, gameID)
	if !ok { return }
	defer done()
	// Rejections here are the overlap and naming rules, which are the user's
	// to hear about rather than a server fault: "that folder is already
	// covered by the main save" is a sentence, not a 500.
	oldRoots, _ := s.Daemon.Store.GameRootPaths(gameID)
	if err := s.Daemon.Store.AddGameRoot(gameID, body.Name, path); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.Daemon.LocationsChanged(gameID, oldRoots)
	// Start watching it now, not at the next restart.
	s.Daemon.RewatchGame(gameID)
	s.BroadcastGamesUpdate()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleRemoveGameRoot(w http.ResponseWriter, r *http.Request) {
	gameID := chi.URLParam(r, "gameId")
	// Names travel in the path and can contain characters chi leaves encoded.
	name, err := url.PathUnescape(chi.URLParam(r, "root"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid location name")
		return
	}
	// Refuse while any transfer still holds a captured path for this game
	// (issue #60). The mapping must stay until the last write finishes.
	done, ok := s.acquireRootTopology(w, r, gameID)
	if !ok { return }
	defer done()
	oldRoots, _ := s.Daemon.Store.GameRootPaths(gameID)
	if err := s.Daemon.Store.RemoveGameRoot(gameID, name); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.Daemon.LocationsChanged(gameID, oldRoots)
	// Stop watching it now, or it keeps waking the game up over a folder
	// nobody is covering any more.
	s.Daemon.RewatchGame(gameID)
	s.BroadcastGamesUpdate()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// acquireRootTopology waits for interactive edits only. The 30-second bound
// covers acquiring the reservation; the database/watcher phase may take longer.
func (s *Server) acquireRootTopology(w http.ResponseWriter, r *http.Request, gameID string) (func(), bool) {
	if s.Daemon == nil || s.Daemon.P2P == nil || s.Daemon.P2P.Sync == nil {
		writeError(w, http.StatusServiceUnavailable, "Save-location admission is unavailable.")
		return nil, false
	}
	limit := s.topologyWaitFor
	if limit <= 0 { limit = 30 * time.Second }
	ctx, cancel := context.WithTimeout(r.Context(), limit)
	defer cancel()
	done, err := s.Daemon.P2P.Sync.WaitTopologyMutation(ctx, gameID, 100*time.Millisecond)
	if err == nil { return done, true }
	if errors.Is(err, context.DeadlineExceeded) && r.Context().Err() == nil {
		writeError(w, http.StatusConflict, "A sync of this game is in progress. Try again in a moment.")
	} else if r.Context().Err() == nil {
		writeError(w, http.StatusServiceUnavailable, "Save-location admission was cancelled or is unavailable.")
	}
	return nil, false
}
