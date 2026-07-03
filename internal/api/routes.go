package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/opensave/opensave/internal/presets"
	"github.com/opensave/opensave/internal/store"
)

// routes registers the Phase 1 endpoint surface. Peer/cloud/p2p routes
// attach in Phases 2-3; window-control and dialog routes attach with the
// Wails app in Phase 4.
func (s *Server) routes(r chi.Router) {
	r.Get("/api/status", s.handleStatus)
	r.Get("/api/settings", s.handleGetSettings)
	r.Post("/api/settings", s.handleUpdateSettings)

	r.Get("/api/games", s.handleListGames)
	r.Post("/api/games", s.handleTrackGame)
	r.Patch("/api/games/{gameId}", s.handleUpdateGame)
	r.Delete("/api/games/{gameId}", s.handleUntrackGame)

	r.Post("/api/games/{gameId}/snapshot", s.handleCreateSnapshot)
	r.Post("/api/games/{gameId}/rollback", s.handleRollback)
	r.Get("/api/games/{gameId}/snapshot/{snapshotId}/files", s.handleSnapshotFiles)
	r.Post("/api/games/{gameId}/snapshot/{snapshotId}/restore-file", s.handleRestoreFile)

	r.Post("/api/games/{gameId}/branch", s.handleCreateBranch)
	r.Post("/api/games/{gameId}/branch/switch", s.handleSwitchBranch)

	r.Post("/api/backup/export", s.handleBackupExport)
	r.Post("/api/backup/restore", s.handleBackupRestore)

	r.Get("/api/presets/scan", s.handlePresetScan)

	s.peerRoutes(r)

	r.Get("/ws", s.Hub.ServeHTTP)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	settings, err := s.Daemon.Store.GetSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	games, _ := s.Daemon.Store.ListGames()
	peers, _ := s.Daemon.Store.ListPeers()
	writeJSON(w, http.StatusOK, map[string]any{
		"settings":  settings,
		"gameCount": len(games),
		"peerCount": len(peers),
	})
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.Daemon.Store.GetSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	current, err := s.Daemon.Store.GetSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Decode over the current settings so omitted fields keep their values
	// (the JS app's {...current, ...new} merge semantics).
	if err := readJSON(r, &current); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.Daemon.Store.UpdateSettings(current); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	updated, _ := s.Daemon.Store.GetSettings()
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleListGames(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.gamesPayload())
}

func (s *Server) handleTrackGame(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     string `json:"name"`
		SavePath string `json:"savePath"`
		AppID    string `json:"appId"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.Name == "" || body.SavePath == "" {
		writeError(w, http.StatusBadRequest, "name and savePath are required")
		return
	}

	game, err := s.Daemon.TrackGame(store.Game{Name: body.Name, SavePath: body.SavePath, AppID: body.AppID})
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	s.BroadcastGamesUpdate()
	writeJSON(w, http.StatusOK, s.gamePayload(game))
}

func (s *Server) handleUpdateGame(w http.ResponseWriter, r *http.Request) {
	gameID := chi.URLParam(r, "gameId")
	game, err := s.Daemon.Store.GetGame(gameID)
	if err != nil {
		writeError(w, notFoundToStatus(err), err.Error())
		return
	}

	oldSavePath := game.SavePath
	oldAutoSync := game.AutoSync
	if err := readJSON(r, &game); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	game.ID = gameID // id is not client-mutable

	if err := s.Daemon.Store.UpdateGame(game); err != nil {
		writeError(w, notFoundToStatus(err), err.Error())
		return
	}

	// Re-watch if the save location or autoSync flag changed.
	if game.SavePath != oldSavePath || game.AutoSync != oldAutoSync {
		s.Daemon.Watcher.Unwatch(gameID)
		if game.AutoSync {
			if err := s.Daemon.Watcher.Watch(gameID, game.SavePath); err != nil {
				s.Daemon.Log.Log("warn", "re-watch failed: "+err.Error())
			}
		}
	}

	s.BroadcastGamesUpdate()
	writeJSON(w, http.StatusOK, s.gamePayload(game))
}

func (s *Server) handleUntrackGame(w http.ResponseWriter, r *http.Request) {
	gameID := chi.URLParam(r, "gameId")
	if err := s.Daemon.UntrackGame(gameID); err != nil {
		writeError(w, notFoundToStatus(err), err.Error())
		return
	}
	s.BroadcastGamesUpdate()
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (s *Server) handleCreateSnapshot(w http.ResponseWriter, r *http.Request) {
	gameID := chi.URLParam(r, "gameId")
	var body struct {
		Comment string `json:"comment"`
	}
	_ = readJSON(r, &body) // empty body is fine

	snap, err := s.Daemon.Snapshots.Create(gameID, body.Comment, false)
	if err != nil {
		writeError(w, notFoundToStatus(err), err.Error())
		return
	}
	s.BroadcastGamesUpdate()
	writeJSON(w, http.StatusOK, snap)
}

func (s *Server) handleRollback(w http.ResponseWriter, r *http.Request) {
	gameID := chi.URLParam(r, "gameId")
	var body struct {
		SnapshotID string `json:"snapshotId"`
	}
	if err := readJSON(r, &body); err != nil || body.SnapshotID == "" {
		writeError(w, http.StatusBadRequest, "snapshotId is required")
		return
	}

	snap, err := s.Daemon.Snapshots.Restore(gameID, body.SnapshotID)
	if err != nil {
		writeError(w, notFoundToStatus(err), err.Error())
		return
	}
	s.BroadcastGamesUpdate()
	writeJSON(w, http.StatusOK, snap)
}

func (s *Server) handleCreateBranch(w http.ResponseWriter, r *http.Request) {
	gameID := chi.URLParam(r, "gameId")
	var body struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &body); err != nil || body.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	clean, err := s.Daemon.Snapshots.CreateBranch(gameID, body.Name)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	s.BroadcastGamesUpdate()
	writeJSON(w, http.StatusOK, map[string]string{"name": clean})
}

func (s *Server) handleSwitchBranch(w http.ResponseWriter, r *http.Request) {
	gameID := chi.URLParam(r, "gameId")
	var body struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &body); err != nil || body.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	if err := s.Daemon.Snapshots.SwitchBranch(gameID, body.Name); err != nil {
		writeError(w, notFoundToStatus(err), err.Error())
		return
	}
	s.BroadcastGamesUpdate()
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (s *Server) handlePresetScan(w http.ResponseWriter, r *http.Request) {
	settings, err := s.Daemon.Store.GetSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	found := s.Daemon.Scanner.Scan(settings.CustomScanPaths)
	if found == nil {
		found = []presets.DiscoveredSave{} // never null on the wire
	}
	writeJSON(w, http.StatusOK, found)
}
