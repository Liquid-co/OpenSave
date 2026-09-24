package e2e

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/opensave/opensave/internal/cloud"
	"github.com/opensave/opensave/internal/daemon"
	"github.com/opensave/opensave/testutil"
)

// Two devices that never meet, sharing one cloud folder — the case the cloud
// mirror is for, and the one it could not serve while nothing read it back
// (GitHub issue #12). Real daemons: watchers snapshot the saves, uploads and
// announcements run in the background, and the answers go through the API the
// app and the CLI use.
func TestCloudReadback_OfferedThenFollowedAcrossDevices(t *testing.T) {
	desk := testutil.NewTestDaemon(t, "ReadbackDesk")
	deck := testutil.NewTestDaemon(t, "ReadbackDeck")
	dir := useLocalCloud(t, desk)
	deck.API(http.MethodPost, "/api/settings", map[string]any{
		"cloudSync": map[string]any{"enabled": true, "provider": "local", "url": dir},
	}, nil)

	deck.WriteSave("slot1.sav", "the deck's own start")
	game := deck.TrackGame("Readback Game")
	waitForHeads(t, dir, 1)

	desk.WriteSave("slot1.sav", "act 1")
	if id := desk.TrackGame("Readback Game"); id != game {
		t.Fatalf("the two devices track the game as %q and %q", game, id)
	}
	waitForHeads(t, dir, 2)

	var offers []daemon.CloudOffer
	deck.API(http.MethodPost, "/api/cloud/check", nil, &offers)
	if len(offers) != 1 || !offers[0].Diverged {
		t.Fatalf("offers = %+v, want the desktop's save, marked as replacing progress", offers)
	}
	if got := deck.ReadSave("slot1.sav"); got != "the deck's own start" {
		t.Fatalf("the deck's save changed before anyone said yes: %q", got)
	}

	deck.API(http.MethodPost, "/api/cloud/offers/accept",
		map[string]string{"gameId": game, "snapshotId": offers[0].SnapshotID}, nil)
	if got := deck.ReadSave("slot1.sav"); got != "act 1" {
		t.Fatalf("after bringing it here the deck's save is %q", got)
	}

	// The desktop plays on. Its watcher snapshots the change, uploads it and
	// announces it; the deck, whose save is the desktop's act 1, takes it
	// without being asked.
	desk.WriteSave("slot1.sav", "act 2")
	ok := testutil.WaitFor(60*time.Second, func() bool {
		deck.API(http.MethodPost, "/api/cloud/check", nil, &offers)
		return deck.ReadSave("slot1.sav") == "act 2"
	})
	if !ok {
		t.Fatalf("the deck never took the desktop's act 2; it has %q and was offered %+v",
			deck.ReadSave("slot1.sav"), offers)
	}
	if len(offers) != 0 {
		t.Errorf("a save that was taken is still offered: %+v", offers)
	}

	// And nothing bounces back to the desktop.
	var back []daemon.CloudOffer
	desk.API(http.MethodPost, "/api/cloud/check", nil, &back)
	if len(back) != 0 {
		t.Errorf("the desktop was offered its own save back: %+v", back)
	}
	if got := desk.ReadSave("slot1.sav"); got != "act 2" {
		t.Errorf("the desktop's save changed to %q", got)
	}
}

// waitForHeads waits until the cloud folder holds announcements from n
// devices.
func waitForHeads(t *testing.T, dir string, n int) {
	t.Helper()
	ok := testutil.WaitFor(30*time.Second, func() bool {
		devices := map[string]bool{}
		for _, name := range cloudFiles(t, dir) {
			if _, dev, _, ok := cloud.ParseHeadFileName(name); ok {
				devices[dev] = true
			}
		}
		return len(devices) >= n
	})
	if !ok {
		t.Fatalf("the cloud folder never held heads from %d devices: %s", n, strings.Join(cloudFiles(t, dir), ", "))
	}
}

// Removing a game's cloud copies removes this device's announcement for it
// too. Left behind, it would describe for good a game this device no longer
// follows.
func TestCloudReadback_RemovingAGamesCloudCopiesRemovesItsHead(t *testing.T) {
	a := testutil.NewTestDaemon(t, "ReadbackUntrack")
	dir := useLocalCloud(t, a)
	a.WriteSave("slot1.sav", "progress")
	game := a.TrackGame("Untrack Game")
	waitForHeads(t, dir, 1)

	a.API(http.MethodPost, "/api/cloud/delete-game/"+game, map[string]any{}, nil)
	for _, name := range cloudFiles(t, dir) {
		if g, _, _, ok := cloud.ParseHeadFileName(name); ok && g == game {
			t.Errorf("%s is still in the cloud after the game's copies were removed", name)
		}
	}
}
