package api

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/opensave/opensave/internal/store"
)

// Games a peer syncs that this device has not been given a folder for.
//
// These exist only when the device is set to ask before tracking; with the
// default setting an unknown game is tracked at a guessed folder and never
// becomes an offer. See internal/store/migrations/0021_offered_games.sql.

func (s *Server) handleListOfferedGames(w http.ResponseWriter, r *http.Request) {
	offers, err := s.Daemon.Store.ListOfferedGames()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// A slice rather than nil, so the UI gets [] and not null.
	out := make([]store.OfferedGame, 0, len(offers))
	out = append(out, offers...)
	writeJSON(w, http.StatusOK, out)
}

// handlePlaceOfferedGame turns an offer into a tracked game at a folder the
// user chose.
//
// It goes through Daemon.TrackGame rather than writing the game directly, so a
// game placed by hand is indistinguishable afterwards from one auto-tracked:
// the same path validation, the same untrack-tombstone clearing, the same
// retrack notification to peers, the same defaults, the same watcher.
//
// The offer's game id is carried over deliberately. That id is the peer's, and
// it is what the two devices match on — placing under a locally-derived id
// instead would create a game that never syncs with the device that offered it.
func (s *Server) handlePlaceOfferedGame(w http.ResponseWriter, r *http.Request) {
	gameID := chi.URLParam(r, "gameId")

	var body struct {
		Path string `json:"path"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(body.Path) == "" {
		writeError(w, http.StatusBadRequest, "choose a folder for this game")
		return
	}

	offers, err := s.Daemon.Store.OfferedGame(gameID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(offers) == 0 {
		// Either it was placed or declined already, or the peer that offered
		// it has been unpaired. Not an error worth alarming anyone about.
		writeError(w, http.StatusNotFound, "that game is no longer being offered")
		return
	}
	offer := offers[0]

	game, err := s.Daemon.TrackGame(store.Game{
		ID:       offer.GameID,
		Name:     offer.Name,
		SavePath: body.Path,
		AppID:    offer.AppID,
		CoverURL: offer.CoverURL,
	})
	if err != nil {
		// Path validation lives in TrackGame and its messages are written for
		// people ("refusing to use a drive root", "already tracks this
		// folder"). Passing them straight through is more use than a generic
		// failure.
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Cleared only after the game exists. If tracking failed, the offer has to
	// survive — dropping it would leave the user with neither a game nor the
	// prompt that would let them try again.
	if err := s.Daemon.Store.ClearOfferedGame(gameID); err != nil {
		s.Daemon.Log.Log("warn", "placed "+game.Name+" but could not clear its offer: "+err.Error())
	}
	s.BroadcastGamesUpdate()
	writeJSON(w, http.StatusOK, s.gamePayload(game))
}

// handleDeclineOfferedGame refuses an offer and records that refusal, so the
// peer asking every few minutes does not put it straight back.
//
// The tombstone is the same one an explicit untrack writes, and it is cleared
// the same way: tracking the game deliberately later un-declines it. That
// keeps one concept — "this device does not want this game" — rather than two
// that could disagree.
func (s *Server) handleDeclineOfferedGame(w http.ResponseWriter, r *http.Request) {
	gameID := chi.URLParam(r, "gameId")

	if err := s.Daemon.Store.AddUntrackedTombstone(gameID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.Daemon.Store.ClearOfferedGame(gameID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.BroadcastGamesUpdate()
	writeJSON(w, http.StatusOK, map[string]any{"declined": gameID})
}
