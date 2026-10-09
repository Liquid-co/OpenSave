package api

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/opensave/opensave/internal/store"
)

// handleReleaseProvisioning ends the hold on one game. It does not sync.
//
// An empty body turns AutoSync on and starts watching. Reconcile and a peer
// coming online will then sync this game with every online peer that is not
// still held. {"autoSync": false} clears the hold and leaves AutoSync off, so
// those paths do not sync. The caller syncs explicitly, then turns AutoSync
// on when that result is the one it wants kept. A repeat after the hold is
// gone does not change AutoSync. Staged release blocks this device
// from initiating automatic sync, not unheld peers from requesting files.
func (s *Server) handleReleaseProvisioning(w http.ResponseWriter, r *http.Request) {
	gameID := chi.URLParam(r, "gameId")
	var body struct {
		AutoSync *bool `json:"autoSync"`
	}
	if r.ContentLength != 0 {
		if err := readJSON(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	enableAutoSync := true
	if body.AutoSync != nil {
		enableAutoSync = *body.AutoSync
	}
	released, err := s.Daemon.ReleaseProvisioningMode(gameID, enableAutoSync)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, store.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err.Error())
		return
	}
	game, gerr := s.Daemon.Store.GetGame(gameID)
	auto := enableAutoSync
	if gerr == nil {
		auto = game.AutoSync
	}
	s.BroadcastGamesUpdate()
	writeJSON(w, http.StatusOK, map[string]any{
		"id":              gameID,
		"released":        released,
		"alreadyReleased": !released,
		"autoSync":        auto,
	})
}
