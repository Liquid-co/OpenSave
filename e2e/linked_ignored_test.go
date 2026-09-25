package e2e

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/opensave/opensave/internal/daemon"
	"github.com/opensave/opensave/testutil"
)

// Files a device is told not to sync stay that device's own, whichever way a
// save arrives. Syncing between paired devices has always honoured the rule;
// these hold it to the same promise everywhere else a save comes in, and hold
// linked copies to theirs.

// Sanity, for the path that is meant to work: a paired device's save arrives,
// and the file each device keeps for itself stays as it was on both. Set up
// the way the rule is used — each device has its own settings.ini and ignores
// it — rather than with two different saves, which is a conflict.
func TestIgnored_PeerSyncLeavesAnIgnoredFileAlone(t *testing.T) {
	a := testutil.NewTestDaemon(t, "IgnorePeer-A")
	b := testutil.NewTestDaemon(t, "IgnorePeer-B")
	a.PairWith(b)

	b.WriteSave("settings.ini", "b's graphics")
	gameID := b.TrackGame("Ignore Peer Game")
	b.API(http.MethodPatch, "/api/games/"+gameID, map[string]any{"syncIgnore": "settings.ini"}, nil)

	a.WriteSave("settings.ini", "a's graphics")
	a.WriteSave("slot1.sav", "a's act 1")
	if id := a.TrackGame("Ignore Peer Game"); id != gameID {
		t.Fatalf("the devices track the game as %q and %q", gameID, id)
	}
	a.API(http.MethodPatch, "/api/games/"+gameID, map[string]any{"syncIgnore": "settings.ini"}, nil)

	if !testutil.WaitFor(60*time.Second, func() bool {
		a.API(http.MethodPost, "/api/games/"+gameID+"/sync", nil, nil)
		return b.ReadSave("slot1.sav") == "a's act 1"
	}) {
		t.Fatalf("a's save never reached b: b has %q", b.ReadSave("slot1.sav"))
	}
	testutil.SettleSync(t, gameID, a, b)
	if got := b.ReadSave("settings.ini"); got != "b's graphics" {
		t.Errorf("b's ignored settings.ini became %q", got)
	}
	if got := a.ReadSave("settings.ini"); got != "a's graphics" {
		t.Errorf("a's ignored settings.ini became %q", got)
	}
}

// A save brought from another device through the cloud replaces the save,
// not the files this device keeps for itself. It used to put the other
// device's snapshot in place whole: the ignored file came back as that
// device's copy — or went, if that device had none.
func TestIgnored_ASaveFromTheCloudLeavesAnIgnoredFileAlone(t *testing.T) {
	desk := testutil.NewTestDaemon(t, "IgnoreCloudDesk")
	deck := testutil.NewTestDaemon(t, "IgnoreCloudDeck")
	dir := useLocalCloud(t, desk)
	deck.API(http.MethodPost, "/api/settings", map[string]any{
		"cloudSync": map[string]any{"enabled": true, "provider": "local", "url": dir},
	}, nil)

	deck.WriteSave("settings.ini", "the deck's graphics")
	deck.WriteSave("slot1.sav", "the deck's own start")
	game := deck.TrackGame("Ignore Cloud Game")
	deck.API(http.MethodPatch, "/api/games/"+game, map[string]any{"syncIgnore": "settings.ini\nlogs/"}, nil)
	deck.WriteSave("logs/deck.log", "the deck's log")
	waitForHeads(t, dir, 1)

	desk.WriteSave("settings.ini", "the desktop's graphics")
	desk.WriteSave("slot1.sav", "act 1")
	if id := desk.TrackGame("Ignore Cloud Game"); id != game {
		t.Fatalf("the two devices track the game as %q and %q", game, id)
	}
	waitForHeads(t, dir, 2)

	var offers []daemon.CloudOffer
	deck.API(http.MethodPost, "/api/cloud/check", nil, &offers)
	if len(offers) != 1 {
		t.Fatalf("offers = %+v, want the desktop's save", offers)
	}
	deck.API(http.MethodPost, "/api/cloud/offers/accept",
		map[string]string{"gameId": game, "snapshotId": offers[0].SnapshotID}, nil)

	if got := deck.ReadSave("slot1.sav"); got != "act 1" {
		t.Fatalf("the desktop's save did not arrive: slot1.sav is %q", got)
	}
	if got := deck.ReadSave("settings.ini"); got != "the deck's graphics" {
		t.Errorf("the deck's ignored settings.ini became %q", got)
	}
	if got := deck.ReadSave("logs/deck.log"); got != "the deck's log" {
		t.Errorf("a file in the deck's ignored logs folder became %q", got)
	}
}

// The same, for a cloud copy restored by hand from the cloud browser: it may
// be another device's, and the files this device keeps for itself stay.
func TestIgnored_RestoringACloudCopyLeavesAnIgnoredFileAlone(t *testing.T) {
	desk := testutil.NewTestDaemon(t, "IgnoreBrowseDesk")
	deck := testutil.NewTestDaemon(t, "IgnoreBrowseDeck")
	dir := useLocalCloud(t, desk)
	deck.API(http.MethodPost, "/api/settings", map[string]any{
		"cloudSync": map[string]any{"enabled": true, "provider": "local", "url": dir},
	}, nil)

	desk.WriteSave("settings.ini", "the desktop's graphics")
	desk.WriteSave("slot1.sav", "act 1")
	game := desk.TrackGame("Ignore Browse Game")
	desk.API(http.MethodPost, "/api/games/"+game+"/snapshot", map[string]string{"comment": "on the desktop"}, nil)
	desk.API(http.MethodPost, "/api/cloud/sync-local/"+game, nil, nil)
	names := waitForUpload(t, dir)

	deck.WriteSave("settings.ini", "the deck's graphics")
	deck.WriteSave("slot1.sav", "the deck's own start")
	if id := deck.TrackGame("Ignore Browse Game"); id != game {
		t.Fatalf("the two devices track the game as %q and %q", game, id)
	}
	deck.API(http.MethodPatch, "/api/games/"+game, map[string]any{"syncIgnore": "settings.ini"}, nil)

	if code := deck.APIStatus(http.MethodPost, "/api/cloud/restore/"+game, map[string]string{"fileName": names[len(names)-1]}, nil); code != http.StatusOK {
		t.Fatalf("restoring the cloud copy returned %d: %s", code, deck.LastError())
	}
	if got := deck.ReadSave("slot1.sav"); got != "act 1" {
		t.Fatalf("the cloud copy did not arrive: slot1.sav is %q", got)
	}
	if got := deck.ReadSave("settings.ini"); got != "the deck's graphics" {
		t.Errorf("the deck's ignored settings.ini became %q", got)
	}
}

// Linking two copies of a game keeps the history of the one merged in. Its
// snapshots used to go with its entry — the database dropped them with it —
// while their archives stayed on disk where nothing listed, restored or
// cleaned them up.
func TestLinked_MergingACopyKeepsItsSnapshots(t *testing.T) {
	td := testutil.NewTestDaemon(t, "LinkHistory")
	td.WriteSave("slot1.sav", "the steam copy")
	canonical := td.TrackGame("Link History Game")

	otherDir := filepath.Join(testutil.TempDir(t), "portable")
	if err := os.MkdirAll(otherDir, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(otherDir, "slot1.sav"), []byte("the portable copy"), 0o666); err != nil {
		t.Fatal(err)
	}
	var created struct {
		ID string `json:"id"`
	}
	td.API(http.MethodPost, "/api/games", map[string]string{"name": "Link History Game (Portable)", "savePath": otherDir}, &created)
	merged := created.ID
	td.API(http.MethodPost, "/api/games/"+merged+"/snapshot", map[string]string{"comment": "before the last boss"}, nil)
	before := snapshotComments(td, merged)
	if len(before) == 0 {
		t.Fatal("setup: the portable copy has no snapshots")
	}

	td.API(http.MethodPost, "/api/games/"+canonical+"/link", map[string]string{"alias": merged}, nil)

	after := snapshotComments(td, canonical)
	for c := range before {
		if !after[c] {
			t.Errorf("after linking, the merged copy's snapshot %q is not among the game's: %v", c, after)
		}
	}
}

// snapshotComments is every snapshot comment a game has, on any branch.
func snapshotComments(td *testutil.TestDaemon, gameID string) map[string]bool {
	var games map[string]struct {
		Branches map[string]struct {
			Snapshots []struct {
				Comment string `json:"comment"`
			} `json:"snapshots"`
		} `json:"branches"`
	}
	td.API(http.MethodGet, "/api/games", nil, &games)
	out := map[string]bool{}
	for _, b := range games[gameID].Branches {
		for _, s := range b.Snapshots {
			out[s.Comment] = true
		}
	}
	return out
}

// A newer save on another device reaches this one through the cloud when
// the two track the game under different names and have been linked — the
// very case linking exists for. The cloud's read-back looked only for the
// name this device uses.
func TestLinked_ANewerSaveArrivesThroughTheCloudUnderTheOtherName(t *testing.T) {
	desk := testutil.NewTestDaemon(t, "LinkCloudDesk")
	deck := testutil.NewTestDaemon(t, "LinkCloudDeck")
	dir := useLocalCloud(t, desk)
	deck.API(http.MethodPost, "/api/settings", map[string]any{
		"cloudSync": map[string]any{"enabled": true, "provider": "local", "url": dir},
	}, nil)

	deck.WriteSave("slot1.sav", "the deck's own start")
	deckID := deck.TrackGame("Link Cloud Game (Deck)")
	waitForHeads(t, dir, 1)

	desk.WriteSave("slot1.sav", "act 1")
	deskID := desk.TrackGame("Link Cloud Game")
	if deskID == deckID {
		t.Fatalf("setup: both devices named the game %q", deskID)
	}
	waitForHeads(t, dir, 2)

	deck.API(http.MethodPost, "/api/games/"+deckID+"/link", map[string]string{"alias": deskID}, nil)

	var offers []daemon.CloudOffer
	deck.API(http.MethodPost, "/api/cloud/check", nil, &offers)
	if len(offers) != 1 || offers[0].GameID != deckID {
		t.Fatalf("offers = %+v, want the desktop's save for %q", offers, deckID)
	}
	deck.API(http.MethodPost, "/api/cloud/offers/accept",
		map[string]string{"gameId": deckID, "snapshotId": offers[0].SnapshotID}, nil)
	if got := deck.ReadSave("slot1.sav"); got != "act 1" {
		t.Errorf("after taking it, the deck's save is %q", got)
	}
}
