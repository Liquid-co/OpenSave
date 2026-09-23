package daemon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/opensave/opensave/internal/presets"
	"github.com/opensave/opensave/internal/store"
)

// A machine the scanner sees and nothing else: a Linux home whose games are
// saves inside a Wine prefix, which is where the scanner looks without a
// Steam library or a network.
type scanMachine struct {
	t    *testing.T
	d    *Daemon
	home string
}

func newScanMachine(t *testing.T) *scanMachine {
	t.Helper()
	d := newTestDaemon(t)
	home := t.TempDir()
	d.Scanner = &presets.Scanner{
		CacheFile:          filepath.Join(t.TempDir(), "cache.json"),
		GOOS:               "linux",
		HomeDir:            home,
		SteamRoots:         []string{},
		SteamUserdataPaths: []string{},
		EpicManifestDirs:   []string{},
		InstallParentDirs:  []string{},
		LocalLowDir:        filepath.Join(t.TempDir(), "nolocallow"),
		MountRoots:         []string{},
	}
	return &scanMachine{t: t, d: d, home: home}
}

// install puts a game's save folder on the machine; with no files when
// played is false, the way a launcher makes a folder before the first save.
func (m *scanMachine) install(game string, played bool) string {
	m.t.Helper()
	dir := filepath.Join(m.home, ".wine", "drive_c", "users", "deck", "AppData", "Roaming", game)
	if err := os.MkdirAll(filepath.Join(dir, "profiles"), 0o777); err != nil {
		m.t.Fatal(err)
	}
	if played {
		m.play(game)
	}
	return dir
}

func (m *scanMachine) play(game string) {
	m.t.Helper()
	dir := filepath.Join(m.home, ".wine", "drive_c", "users", "deck", "AppData", "Roaming", game)
	if err := os.WriteFile(filepath.Join(dir, "profiles", "save.dat"), []byte("progress"), 0o666); err != nil {
		m.t.Fatal(err)
	}
}

func (m *scanMachine) announced() map[string]bool {
	m.t.Helper()
	m.d.DetectNewGames()
	out := map[string]bool{}
	for _, g := range m.d.NewGames() {
		out[filepath.Base(g.SavePath)] = true
	}
	return out
}

// The first scan takes stock and says nothing: everything untracked on a
// machine is not news, and announcing it would bury the one game that is.
func TestNewGames_FirstScanOnlyTakesStock(t *testing.T) {
	m := newScanMachine(t)
	m.install("OldFavourite", true)

	if got := m.announced(); len(got) != 0 {
		t.Errorf("the first scan announced %v", got)
	}
	if has, _ := m.d.Store.HasKnownSaves(); !has {
		t.Error("the first scan did not note what it found, so the next would announce it")
	}
}

// A game installed and played after that is announced, once.
func TestNewGames_AGameInstalledLaterIsAnnouncedOnce(t *testing.T) {
	m := newScanMachine(t)
	m.install("OldFavourite", true)
	m.announced()

	var told [][]NewGame
	m.d.OnNewGames = func(g []NewGame) { told = append(told, g) }
	m.install("BrandNew", true)
	got := m.announced()
	if !got["BrandNew"] || got["OldFavourite"] {
		t.Fatalf("announced %v, want only BrandNew", got)
	}
	if len(told) != 1 {
		t.Errorf("listeners were told %d times, want once", len(told))
	}

	// Looked at and dismissed: not announced again.
	m.d.DismissNewGames()
	if got := m.announced(); len(got) != 0 {
		t.Errorf("a game already announced was announced again: %v", got)
	}
}

// A launcher makes a folder for a game before its first save. That is not
// news until the game writes to it — and then it is.
func TestNewGames_AnEmptyFolderWaitsForItsFirstSave(t *testing.T) {
	m := newScanMachine(t)
	m.install("OldFavourite", true)
	m.announced()

	m.install("NotPlayedYet", false)
	if got := m.announced(); got["NotPlayedYet"] {
		t.Fatal("an empty folder was announced as a game with saves")
	}
	m.play("NotPlayedYet")
	if got := m.announced(); !got["NotPlayedYet"] {
		t.Errorf("the game was not announced after its first save: %v", got)
	}
}

// A game already tracked is not news, whatever the scan finds.
func TestNewGames_TrackedGamesAreNotAnnounced(t *testing.T) {
	m := newScanMachine(t)
	m.install("OldFavourite", true)
	m.announced()

	dir := m.install("TrackedFirst", true)
	if err := m.d.Store.CreateGame(store.Game{ID: "tracked-first", Name: "Tracked First",
		SavePath: dir, ActiveBranch: "main", AutoSync: true, MaxSnapshots: 20}); err != nil {
		t.Fatal(err)
	}
	// The listener, not just the list: the list is filtered again when it is
	// read, so a tracked game that slipped through would still reach the log
	// and the app as "newly installed" before being hidden.
	told := 0
	m.d.OnNewGames = func([]NewGame) { told++ }
	if got := m.announced(); got["TrackedFirst"] {
		t.Errorf("a tracked game was announced: %v", got)
	}
	if told != 0 {
		t.Errorf("listeners were told about new games %d time(s); the only one found is tracked", told)
	}
}

// Tracked after being announced: it stops waiting.
func TestNewGames_TrackingAnnouncedGameClearsIt(t *testing.T) {
	m := newScanMachine(t)
	m.install("OldFavourite", true)
	m.announced()

	dir := m.install("TrackMe", true)
	if got := m.announced(); !got["TrackMe"] {
		t.Fatalf("setup: TrackMe was not announced: %v", got)
	}
	if err := m.d.Store.CreateGame(store.Game{ID: "track-me", Name: "Track Me",
		SavePath: dir, ActiveBranch: "main", AutoSync: true, MaxSnapshots: 20}); err != nil {
		t.Fatal(err)
	}
	if left := m.d.NewGames(); len(left) != 0 {
		t.Errorf("a game tracked since it was announced is still waiting: %+v", left)
	}
}

// With the setting off there is no background scan at all.
func TestNewGames_SettingOffScansNothing(t *testing.T) {
	m := newScanMachine(t)
	settings, _ := m.d.Store.GetSettings()
	settings.DetectNewGames = false
	if err := m.d.Store.UpdateSettings(settings); err != nil {
		t.Fatal(err)
	}
	m.install("OldFavourite", true)
	m.announced()
	if has, _ := m.d.Store.HasKnownSaves(); has {
		t.Error("the scan ran with the setting off")
	}
}
