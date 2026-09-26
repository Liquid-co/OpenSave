package daemon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/opensave/opensave/internal/presets"
	"github.com/opensave/opensave/internal/store"
)

const testTitle = "0100ABCDEF012000"

// switchSave makes a title's save folder in an Eden NAND under root, with a
// file in it, and returns it.
func switchSave(t *testing.T, root, profile string) string {
	t.Helper()
	dir := filepath.Join(root, ".local", "share", "eden", "nand", "user", "save", "0000000000000000", profile, testTitle)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "progress.sav"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// A Switch game is tracked under its title id, whatever it is named: the same
// id on every device. A second copy on this one is told apart as any other is.
func TestSwitchGameIsTrackedUnderItsTitleID(t *testing.T) {
	d := newTestDaemon(t)
	home := t.TempDir()
	first, err := d.TrackGame(store.Game{Name: "Tears of the Kingdom", SavePath: switchSave(t, home, "11111111111111111111111111111111")})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != "switch-0100abcdef012000" {
		t.Errorf("id = %q", first.ID)
	}
	second, err := d.TrackGame(store.Game{Name: "Zelda", SavePath: switchSave(t, home, "22222222222222222222222222222222")})
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != "switch-0100abcdef012000-2" {
		t.Errorf("second copy's id = %q", second.ID)
	}
	other, err := d.TrackGame(store.Game{Name: "Hades", SavePath: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if other.ID != "hades" {
		t.Errorf("a game that is not a Switch save: id = %q", other.ID)
	}
}

// Games tracked under the name the scan made up are given their emulator's
// name for them; a name the person chose is left alone, and no id changes.
func TestSwitchGamesTrackedUnnamedAreNamed(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")
	d := newTestDaemon(t)
	home := t.TempDir()
	d.Scanner = &presets.Scanner{GOOS: "linux", HomeDir: home}
	cache := filepath.Join(home, ".cache", "eden", "game_list")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cache, testTitle+".appname.txt"), []byte("Tears of the Kingdom"), 0o644); err != nil {
		t.Fatal(err)
	}

	madeUp, err := d.TrackGame(store.Game{
		ID:       "eden-switch-emulator-title-id-0100abcdef012000",
		Name:     "Eden Switch Emulator - Title ID: " + testTitle,
		SavePath: switchSave(t, home, "11111111111111111111111111111111"),
	})
	if err != nil {
		t.Fatal(err)
	}
	chosen, err := d.TrackGame(store.Game{Name: "My Zelda", SavePath: switchSave(t, home, "22222222222222222222222222222222")})
	if err != nil {
		t.Fatal(err)
	}

	if n := d.nameSwitchGames(); n != 1 {
		t.Errorf("renamed %d, want 1", n)
	}
	if g, _ := d.Store.GetGame(madeUp.ID); g.Name != "Tears of the Kingdom" {
		t.Errorf("the made-up name is now %q", g.Name)
	}
	if g, _ := d.Store.GetGame(chosen.ID); g.Name != "My Zelda" {
		t.Errorf("the chosen name became %q", g.Name)
	}
}
