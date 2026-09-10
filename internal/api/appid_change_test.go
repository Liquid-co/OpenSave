package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// Changing a game's App ID must make the next cover request actually ask.
//
// A cover that failed to load once was remembered as "no art" for six hours,
// and nothing done with the App ID field could shorten that: correct a typo,
// or watch the network come back, and the cover stayed blank until the memory
// expired or the app restarted. Someone doing everything right saw nothing
// happen, and asked whether they were doing something wrong.
func TestChangingAppIDForgetsACachedMiss(t *testing.T) {
	ts := startTestServer(t)

	resp, raw := ts.do(t, http.MethodPost, "/api/games",
		map[string]string{"name": "Dawnwalker", "savePath": ts.saveDir})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("track: %d", resp.StatusCode)
	}
	var gameID string
	if err := json.Unmarshal(raw["id"], &gameID); err != nil {
		t.Fatal(err)
	}

	// The state a failed fetch leaves behind: both orientations remembered
	// as missing, as of now.
	const appID = "3105730"
	coverMisses.Store(coverMissKey(appID, false), time.Now())
	coverMisses.Store(coverMissKey(appID, true), time.Now())
	if !recentCoverMiss(coverMissKey(appID, true)) {
		t.Fatal("setup: the miss was not recorded")
	}

	// The person types the App ID.
	resp, _ = ts.do(t, http.MethodPatch, "/api/games/"+gameID, map[string]any{"appId": appID})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("patch: %d", resp.StatusCode)
	}

	for _, portrait := range []bool{false, true} {
		if recentCoverMiss(coverMissKey(appID, portrait)) {
			t.Errorf("after setting the App ID, the cover (portrait=%v) is still remembered as "+
				"missing; the next request would answer 404 from memory without asking", portrait)
		}
	}
}

// Saving the form without touching the App ID must not throw away a miss that
// is still true. The miss cache exists so a scan does not re-walk the network
// for a hundred games with no art on every render.
func TestUnchangedAppIDKeepsACachedMiss(t *testing.T) {
	ts := startTestServer(t)

	resp, raw := ts.do(t, http.MethodPost, "/api/games",
		map[string]string{"name": "No Art Here", "savePath": ts.saveDir, "appId": "424242"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("track: %d", resp.StatusCode)
	}
	var gameID string
	_ = json.Unmarshal(raw["id"], &gameID)

	coverMisses.Store(coverMissKey("424242", false), time.Now())

	// An edit to something else entirely, App ID sent back as it was.
	resp, _ = ts.do(t, http.MethodPatch, "/api/games/"+gameID,
		map[string]any{"appId": "424242", "maxSnapshots": 7})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("patch: %d", resp.StatusCode)
	}
	if !recentCoverMiss(coverMissKey("424242", false)) {
		t.Error("an edit that left the App ID alone discarded the cached miss; every save of " +
			"the form would re-fetch art for a game known to have none")
	}
}
