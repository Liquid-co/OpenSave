package e2e

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/opensave/opensave/testutil"
)

// 2.4.2: a game arriving from a peer is tracked by itself wherever this device
// can vouch for the folder — including one that does not exist yet — and the
// user decides the rest (CVE-2026-103398).

// A new device fills itself in: a game in the save catalogue arrives before it
// was ever played here, and is tracked where the catalogue says it saves on
// this device, its saves in place before first launch.
func TestFolderChoice_ACatalogueGameArrivesBeforeItsFolderExists(t *testing.T) {
	a := testutil.NewTestDaemon(t, "Catalogue-A")
	b := testutil.NewTestDaemon(t, "Catalogue-B")
	a.PairWith(b)

	// Each device's own application-data folder, as the catalogue resolves
	// it; the path rule maps one to the other, as two users' profiles map.
	rootA, rootB := t.TempDir(), t.TempDir()
	t.Setenv("APPDATA", filepath.Join(rootB, "AppData", "Roaming"))
	b.API(http.MethodPost, "/api/settings", map[string]any{
		"pathTranslations": []map[string]string{{"fromPattern": rootA, "toPattern": rootB}},
	}, nil)
	manifest := `
Hollow Game:
  files:
    <winAppData>/MoonStudio/HollowGame:
      tags: [save]
      when:
        - os: windows
`
	if err := os.WriteFile(filepath.Join(filepath.Dir(b.Daemon.Scanner.CacheFile), "ludusavi-manifest.yaml"), []byte(manifest), 0o666); err != nil {
		t.Fatal(err)
	}
	b.Daemon.Scanner.GOOS = "windows"

	saveA := filepath.Join(rootA, "AppData", "Roaming", "MoonStudio", "HollowGame")
	writeAt(t, filepath.Join(saveA, "slot0.dat"), "act two")
	var tracked gameRow
	a.API(http.MethodPost, "/api/games", map[string]string{"name": "Hollow Game", "savePath": saveA}, &tracked)
	a.API(http.MethodPost, "/api/games/"+tracked.ID+"/sync", nil, nil)

	want := filepath.Join(rootB, "AppData", "Roaming", "MoonStudio", "HollowGame")
	if !testutil.WaitFor(45*time.Second, func() bool {
		return readAt(filepath.Join(want, "slot0.dat")) == "act two"
	}) {
		offers, _ := b.Daemon.Store.ListOfferedGames()
		t.Fatalf("the save never arrived in the game's folder on B; B has %+v, offers %+v", gamesOf(b), offers)
	}
	if offers, _ := b.Daemon.Store.ListOfferedGames(); len(offers) != 0 {
		t.Errorf("a game this device could vouch for was offered too: %+v", offers)
	}
}

// A device the user trusts to choose folders has its hand-picked folder
// synced by itself; the same game from a device not trusted is offered.
func TestFolderChoice_ATrustedDevicesOwnFolderSyncs(t *testing.T) {
	a := testutil.NewTestDaemon(t, "Trusted-A")
	b := testutil.NewTestDaemon(t, "Trusted-B")
	a.PairWith(b)

	var ok map[string]any
	b.API(http.MethodPost, "/api/peers/"+a.NodeID()+"/choose-folders", map[string]any{"allowed": true}, &ok)
	if ok["choosesFolders"] != true {
		t.Fatalf("could not trust A: %+v", ok)
	}

	a.WriteSave("homebrew.sav", "hand-picked")
	gameID := a.TrackGame("Homebrew Game")
	syncTo(a, gameID, b.NodeID())

	if !testutil.WaitFor(30*time.Second, func() bool {
		_, err := b.Daemon.Store.GetGame(gameID)
		return err == nil
	}) {
		offers, _ := b.Daemon.Store.ListOfferedGames()
		t.Fatalf("the trusted device's game was not tracked on B; offers %+v", offers)
	}
	if offers, _ := b.Daemon.Store.ListOfferedGames(); len(offers) != 0 {
		t.Errorf("tracked, yet still offered: %+v", offers)
	}
}

// An offer answers itself: once this device knows the folder, the peer's
// next sync tracks the game and the offer is gone from Home.
func TestFolderChoice_AnOfferClearsWhenTheFolderBecomesKnown(t *testing.T) {
	a := testutil.NewTestDaemon(t, "Clears-A")
	b := testutil.NewTestDaemon(t, "Clears-B")
	a.PairWith(b)

	a.WriteSave("slot1.sav", "from A")
	gameID := a.TrackGame("Later Game")
	syncTo(a, gameID, b.NodeID())
	if !testutil.WaitFor(30*time.Second, func() bool {
		offers, _ := b.Daemon.Store.ListOfferedGames()
		return len(offers) == 1
	}) {
		t.Fatal("the game was not offered on B first")
	}

	b.KnowPeersSaveFolder(a.SaveDir) // B's scan finds the folder
	syncTo(a, gameID, b.NodeID())

	if !testutil.WaitFor(30*time.Second, func() bool {
		_, err := b.Daemon.Store.GetGame(gameID)
		offers, _ := b.Daemon.Store.ListOfferedGames()
		return err == nil && len(offers) == 0
	}) {
		offers, _ := b.Daemon.Store.ListOfferedGames()
		t.Fatalf("not tracked, or the offer stayed: B has %+v, offers %+v", gamesOf(b), offers)
	}
}
