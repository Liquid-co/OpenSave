package e2e

import (
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/opensave/opensave/testutil"
)

// A save folder that has gone — deleted, on a drive that is not plugged in,
// renamed by a reinstall — is not a save that was emptied. Every file in it
// looks deleted, and passing that on to a paired device would delete that
// device's save too: the one copy left.
func TestMissingFolder_DoesNotWipeThePeer(t *testing.T) {
	a, b, gameID := pairAndTrack(t, "Missing Folder Game", map[string]string{
		"slot1.sav":        "hours of play",
		"slot2.sav":        "more hours",
		"profile/user.cfg": "settings",
	})
	testutil.SettleSync(t, gameID, a, b)

	// B's folder goes, whole.
	testutil.RemoveTree(t, b.SaveDir)

	// Both sides reach for each other, as they would on their own.
	for i := 0; i < 3; i++ {
		b.API(http.MethodPost, "/api/games/"+gameID+"/sync", nil, nil)
		a.API(http.MethodPost, "/api/games/"+gameID+"/sync", nil, nil)
		time.Sleep(time.Second)
	}
	testutil.SettleSync(t, gameID, a, b)

	for rel, want := range map[string]string{"slot1.sav": "hours of play", "slot2.sav": "more hours", "profile/user.cfg": "settings"} {
		if got := a.ReadSave(rel); got != want {
			t.Errorf("after B's folder went missing, A's %s is %q — the loss was passed on", rel, got)
		}
	}
}

// The same, across what a restart does: the watch is set up again. That used
// to create the folder afresh, empty — and the next sync read every file in
// it as deleted, and deleted them on the other device too. Then, when the
// folder comes back, it is watched and synced again.
func TestMissingFolder_ARestartDoesNotRecreateItEmpty(t *testing.T) {
	a, b, gameID := pairAndTrack(t, "Missing Restart Game", map[string]string{
		"slot1.sav": "hours of play",
		"slot2.sav": "more hours",
	})
	testutil.SettleSync(t, gameID, a, b)

	testutil.RemoveTree(t, b.SaveDir)
	// What starting up does, for this game.
	b.API(http.MethodPatch, "/api/games/"+gameID, map[string]any{"autoSync": false}, nil)
	b.API(http.MethodPatch, "/api/games/"+gameID, map[string]any{"autoSync": true}, nil)
	b.API(http.MethodPost, "/api/watch/reload", nil, nil)

	if _, err := os.Stat(b.SaveDir); err == nil {
		t.Errorf("the missing save folder was created again")
	}
	var games map[string]struct {
		SavePathMissing bool `json:"savePathMissing"`
	}
	b.API(http.MethodGet, "/api/games", nil, &games)
	if !games[gameID].SavePathMissing {
		t.Errorf("the game does not say its save folder is missing")
	}

	for i := 0; i < 3; i++ {
		b.API(http.MethodPost, "/api/games/"+gameID+"/sync", nil, nil)
		a.API(http.MethodPost, "/api/games/"+gameID+"/sync", nil, nil)
		time.Sleep(time.Second)
	}
	testutil.SettleSync(t, gameID, a, b)
	for rel, want := range map[string]string{"slot1.sav": "hours of play", "slot2.sav": "more hours"} {
		if got := a.ReadSave(rel); got != want {
			t.Errorf("after B's folder went missing and B restarted, A's %s is %q — the loss was passed on", rel, got)
		}
	}

	// It comes back — a drive plugged in again — and is watched again.
	if err := os.MkdirAll(b.SaveDir, 0o777); err != nil {
		t.Fatal(err)
	}
	b.WriteSave("slot1.sav", "hours of play")
	b.WriteSave("slot2.sav", "more hours")
	b.API(http.MethodPost, "/api/watch/reload", nil, nil)
	b.API(http.MethodGet, "/api/games", nil, &games)
	if games[gameID].SavePathMissing {
		t.Errorf("the save folder is back and the game still says it is missing")
	}
	if _, watching := b.Daemon.Watcher.Watching(gameID); !watching {
		t.Errorf("the save folder is back and it is not watched")
	}
}
