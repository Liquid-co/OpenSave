package api

import (
	"net/http"

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

// GET /api/gamedb/running — games running on this device now.
func (s *Server) handleGameDBRunning(w http.ResponseWriter, r *http.Request) {
	out := s.Daemon.RunningGames()
	if out == nil {
		out = []daemon.RunningGame{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"games": out})
}
