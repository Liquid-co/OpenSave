package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

// Play sessions (see daemon/sessions.go): a game's recent ones, and marking
// one from outside — `opensave wrap` launching a game knows exactly when it
// starts and stops, without waiting to spot its process.

func (s *Server) handleGameSessions(w http.ResponseWriter, r *http.Request) {
	gameID := chi.URLParam(r, "gameId")
	if _, err := s.Daemon.Store.GetGame(gameID); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	list, err := s.Daemon.Store.ListPlaySessions(gameID, 50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	stats, err := s.Daemon.Store.PlayStatsFor(gameID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp := map[string]any{"sessions": list, "stats": stats, "playingSince": ""}
	if since := s.Daemon.PlayingSince(gameID); !since.IsZero() {
		resp["playingSince"] = since.UTC().Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleMarkSession(w http.ResponseWriter, r *http.Request) {
	gameID := chi.URLParam(r, "gameId")
	if _, err := s.Daemon.Store.GetGame(gameID); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	var body struct {
		State string `json:"state"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	tracker := s.Daemon.SessionTracker()
	switch body.State {
	case "start":
		tracker.Begin(gameID, time.Now())
	case "end":
		// Finishing runs the end of the session — the snapshot, the sync —
		// before answering, so the caller knows it is done.
		tracker.Finish(gameID, time.Now())
	default:
		writeError(w, http.StatusBadRequest, `state must be "start" or "end"`)
		return
	}
	s.BroadcastGamesUpdate()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
