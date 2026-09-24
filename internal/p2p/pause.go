package p2p

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/opensave/opensave/internal/syncpause"
)

// Pausing, on the wire.
//
// A paused device fetches nothing (syncengine.SyncGame refuses to start), and
// it serves nothing either: a manifest, a block or a delete asked of it gets
// 503 with "paused": true. Serving would let the other device go on syncing
// from this one while it is paused, which is the opposite of what pausing
// means to the person who pressed it.
//
// The asking device recognises the answer (isPausedRefusal) and skips this
// peer quietly rather than reporting a failed sync; both sides catch up when
// the pause ends. A build from before pausing existed sees an ordinary 503,
// logs it and retries later — no worse than a device that went offline.

// pausedAnswer is what a paused device says to a transfer request.
func pausedAnswer() map[string]any {
	return map[string]any{"error": syncpause.ErrPaused.Error(), "paused": true}
}

// isPausedRefusal reports whether a peer's answer is "I have paused syncing".
func isPausedRefusal(status int, body []byte) bool {
	if status != http.StatusServiceUnavailable {
		return false
	}
	var answer struct {
		Paused bool `json:"paused"`
	}
	return json.Unmarshal(body, &answer) == nil && answer.Paused
}

// refuseWhilePaused turns transfer requests away while this device is paused.
func (e *Engine) refuseWhilePaused(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if e.Pause.Paused() {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(pausedAnswer())
			return
		}
		next.ServeHTTP(w, r)
	})
}

// isTransferRoute names the relay routes that move save data or start a sync,
// which a paused device refuses. Pings, pairing and untrack notices are not
// transfers and keep working, so the two devices still know about each other.
func isTransferRoute(route string) bool {
	for _, prefix := range []string{"/manifest/", "/blocks/", "/snapshot/", "/sync/trigger/", "/delete-file/"} {
		if strings.HasPrefix(route, prefix) {
			return true
		}
	}
	return false
}
