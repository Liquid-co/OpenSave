package api

import (
	"errors"
	"net/http"

	"github.com/opensave/opensave/internal/presets"
)

// GET /api/steam/app?appId=<numeric>
//
// Answers the question a person is really asking when they type an App ID:
// "is this the right number?" Until now they found out by whether a cover
// eventually appeared, and a cover that does not appear says nothing about
// why — a typo, a game Steam has no art for, and a network that blocks
// Steam's CDN all look identical, which is how "I added the app ID but it
// doesn't do anything, am I doing something wrong?" gets asked.
//
// Three answers, because they call for three different next steps: the
// store title (it is right), no such app (check the number), or Steam could
// not be reached (not the number's fault, and covers will not load either).
func (s *Server) handleSteamApp(w http.ResponseWriter, r *http.Request) {
	appID := r.URL.Query().Get("appId")
	if !isNumericID(appID) {
		writeError(w, http.StatusBadRequest, "appId must be numeric")
		return
	}
	name, err := presets.LookupSteamApp(appID)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, map[string]any{"found": true, "name": name})
	case errors.Is(err, presets.ErrSteamAppUnknown):
		writeJSON(w, http.StatusOK, map[string]any{"found": false, "reason": "unknown"})
	default:
		writeJSON(w, http.StatusOK, map[string]any{"found": false, "reason": "unreachable"})
	}
}
