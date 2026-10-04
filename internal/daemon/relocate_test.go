package daemon

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/opensave/opensave/internal/store"
)

// freeDriveLetter returns a drive letter not mounted here, or "".
func freeDriveLetter() string {
	for c := 'Z'; c >= 'D'; c-- {
		if _, err := os.Stat(string(c) + `:\`); err != nil {
			return string(c)
		}
	}
	return ""
}

// A game tracked elsewhere with its save on a drive this device lacks, while
// the identical path exists on another drive here, is switched to it — once.
func TestRelocateToOtherDrive(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("drive letters are a Windows thing")
	}
	free := freeDriveLetter()
	if free == "" {
		t.Skip("no free drive letter")
	}
	d := newTestDaemon(t)
	real := filepath.Join(t.TempDir(), "SteamLibrary", "steamapps", "common", "Game", "save")
	if err := os.MkdirAll(real, 0o777); err != nil {
		t.Fatal(err)
	}
	elsewhere := free + ":" + real[2:] // same path, absent drive
	if err := d.Store.CreateGame(store.Game{ID: "g", Name: "Game", SavePath: elsewhere, ActiveBranch: "main", AutoSync: true}); err != nil {
		t.Fatal(err)
	}

	d.noteMissing("g", "Game", elsewhere, true)

	g, err := d.Store.GetGame("g")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(g.SavePath, real) {
		t.Fatalf("save path = %s, want it switched to %s", g.SavePath, real)
	}
	d.missingMu.Lock()
	stillMissing := d.missing["g"]
	d.missingMu.Unlock()
	if stillMissing {
		t.Error("the game is still marked missing after being found")
	}
}

// Not found: said once, and not searched again on every reconcile.
func TestRelocateNotFound_IsNotRepeated(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("drive letters are a Windows thing")
	}
	free := freeDriveLetter()
	if free == "" {
		t.Skip("no free drive letter")
	}
	d := newTestDaemon(t)
	path := free + `:\NoSuchLibrary\steamapps\common\Nothing\save`
	if err := d.Store.CreateGame(store.Game{ID: "n", Name: "Nothing", SavePath: path, ActiveBranch: "main", AutoSync: true}); err != nil {
		t.Fatal(err)
	}
	if d.relocateToOtherDrive("n", "Nothing", path) {
		t.Fatal("relocated to a folder that does not exist")
	}
	first := d.relocateChecked["n"]
	d.relocateToOtherDrive("n", "Nothing", path)
	if !d.relocateChecked["n"].Equal(first) {
		t.Error("searched again straight away; it should wait relocateRecheck")
	}
}
