package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Whether snapshots can still be restored (daemon/verify.go): the last
// checks' findings, and a check now — of every game, or of one with ?game=.

func (s *Server) handleSnapshotChecks(w http.ResponseWriter, r *http.Request) {
	sum, err := s.Daemon.Store.SnapshotChecks()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sum)
}

func (s *Server) handleVerifySnapshots(w http.ResponseWriter, r *http.Request) {
	report, err := s.Daemon.VerifySnapshots(r.Context(), r.URL.Query().Get("game"), 0)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.BroadcastGamesUpdate()
	writeJSON(w, http.StatusOK, report)
}

// handleCompareSnapshots says what changed from one of a game's snapshots to
// another (snapshot.Manager.Compare).
func (s *Server) handleCompareSnapshots(w http.ResponseWriter, r *http.Request) {
	c, err := s.Daemon.Snapshots.Compare(chi.URLParam(r, "gameId"), chi.URLParam(r, "snapshotId"), chi.URLParam(r, "otherId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, c)
}
