package e2e

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/opensave/opensave/internal/store"
	"github.com/opensave/opensave/testutil"
)

// A title id no real library will have, so the emulators installed on the
// machine running the tests cannot be mistaken for the test's.
const switchTitle = "0100ABCDEF012000"

// nandTitle is a title's save folder in the NAND of emu under root.
func nandTitle(root, emu, profile string) string {
	return filepath.Join(root, emu, "nand", "user", "save", "0000000000000000", profile, switchTitle)
}

func writeAt(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o666); err != nil {
		t.Fatal(err)
	}
}

func readAt(path string) string {
	b, _ := os.ReadFile(path)
	return string(b)
}

type gameRow struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	SavePath string `json:"savePath"`
}

func gamesOf(td *testutil.TestDaemon) map[string]gameRow {
	var games map[string]gameRow
	td.API(http.MethodGet, "/api/games", nil, &games)
	return games
}

// A Switch save arriving from another device goes into this device's own
// emulator profile. It used to go to the other device's path translated here,
// which names the other install's profile id — a folder this emulator never
// reads, beside the one it does, which then went unsynced.
func TestSwitch_AnArrivingSaveGoesUnderThisDevicesProfile(t *testing.T) {
	a := testutil.NewTestDaemon(t, "SwitchA")
	b := testutil.NewTestDaemon(t, "SwitchB")
	a.PairWith(b)
	rootA, rootB := t.TempDir(), t.TempDir()

	onA := nandTitle(rootA, "eden", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	writeAt(t, filepath.Join(onA, "progress.sav"), "ninety hours in")
	// B has the same emulator, with a profile of its own and no save yet.
	bProfile := filepath.Dir(nandTitle(rootB, "eden", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"))
	if err := os.MkdirAll(bProfile, 0o777); err != nil {
		t.Fatal(err)
	}
	b.API(http.MethodPost, "/api/settings", map[string]any{
		"pathTranslations": []map[string]string{{"fromPattern": rootA, "toPattern": rootB}},
	}, nil)

	var tracked gameRow
	a.API(http.MethodPost, "/api/games", map[string]string{"name": "Tears of the Kingdom", "savePath": onA}, &tracked)
	if tracked.ID != "switch-0100abcdef012000" {
		t.Errorf("tracked as %q, want the title id's", tracked.ID)
	}
	a.API(http.MethodPost, "/api/games/"+tracked.ID+"/sync", nil, nil)

	want := filepath.Join(bProfile, switchTitle)
	if !testutil.WaitFor(45*time.Second, func() bool {
		return readAt(filepath.Join(want, "progress.sav")) == "ninety hours in"
	}) {
		t.Fatalf("the save never arrived under B's profile; B has %+v", gamesOf(b))
	}
	if got := gamesOf(b)[tracked.ID].SavePath; got != want {
		t.Errorf("B tracks it at %s, want %s", got, want)
	}
	if _, err := os.Stat(nandTitle(rootB, "eden", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")); err == nil {
		t.Error("a folder was made under A's profile id on B")
	}
}

// The same Switch game syncs whatever id each device tracks it under: here
// one tracked before 2.4 (an id made from the name the scan gave it, naming
// the emulator) and one tracked since (its title id), in different emulators.
// Both directions, and without B taking A's game as a second, new one.
func TestSwitch_TheSameGameUnderDifferentIDsSyncs(t *testing.T) {
	a := testutil.NewTestDaemon(t, "SwitchIdA")
	b := testutil.NewTestDaemon(t, "SwitchIdB")
	a.PairWith(b)
	root := t.TempDir()

	onA := nandTitle(root, "citron", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	writeAt(t, filepath.Join(onA, "progress.sav"), "from A")
	onB := nandTitle(root, "eden", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	if err := os.MkdirAll(onB, 0o777); err != nil {
		t.Fatal(err)
	}
	legacyID := "eden-switch-emulator-title-id-" + strings.ToLower(switchTitle)
	if _, err := b.Daemon.TrackGame(store.Game{ID: legacyID, Name: "Eden Switch Emulator - Title ID: " + switchTitle, SavePath: onB}); err != nil {
		t.Fatal(err)
	}

	var tracked gameRow
	a.API(http.MethodPost, "/api/games", map[string]string{"name": "Tears of the Kingdom", "savePath": onA}, &tracked)
	a.API(http.MethodPost, "/api/games/"+tracked.ID+"/sync", nil, nil)
	if !testutil.WaitFor(45*time.Second, func() bool { return readAt(filepath.Join(onB, "progress.sav")) == "from A" }) {
		t.Fatalf("A's save never reached B's copy; B has %+v", gamesOf(b))
	}

	writeAt(t, filepath.Join(onB, "progress.sav"), "from B")
	b.API(http.MethodPost, "/api/games/"+legacyID+"/sync", nil, nil)
	if !testutil.WaitFor(45*time.Second, func() bool { return readAt(filepath.Join(onA, "progress.sav")) == "from B" }) {
		t.Fatal("B's save never reached A's copy")
	}

	for id := range gamesOf(b) {
		if id != legacyID {
			t.Errorf("B tracks %q as well — A's game was taken as a new one", id)
		}
	}
	for id := range gamesOf(a) {
		if id != tracked.ID {
			t.Errorf("A tracks %q as well — B's game was taken as a new one", id)
		}
	}
}
