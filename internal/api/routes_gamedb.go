package api

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/opensave/opensave/internal/daemon"
	"github.com/opensave/opensave/internal/presets"
)

// GET /api/gamedb/search?q= — games in the game database by name.
func (s *Server) handleGameDBSearch(w http.ResponseWriter, r *http.Request) {
	out := s.Daemon.SearchGames(r.URL.Query().Get("q"))
	if out == nil {
		out = []presets.GameMatch{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"games": out})
}

// GET /api/gamedb/identify?path= — the game a program or folder belongs to.
func (s *Server) handleGameDBIdentify(w http.ResponseWriter, r *http.Request) {
	m, ok := s.Daemon.IdentifyGame(r.URL.Query().Get("path"))
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"found": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"found": true, "game": m})
}

// POST /api/games/{gameId}/gamedb-link {"accept": bool} — the answer to the
// game in the game database a game tracked without an App ID looks like.
func (s *Server) handleAnswerGameDBLink(w http.ResponseWriter, r *http.Request) {
	game, err := s.Daemon.Store.GetGame(chi.URLParam(r, "gameId"))
	if err != nil {
		writeError(w, http.StatusNotFound, "Game not found.")
		return
	}
	var body struct {
		Accept bool `json:"accept"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "accept is required")
		return
	}
	if err := s.Daemon.AnswerGameDBLink(game, body.Accept); err != nil {
		if errors.Is(err, daemon.ErrNoGameDBSuggestion) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.BroadcastGamesUpdate()
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}
