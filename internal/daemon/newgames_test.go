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
	isolateProfile(t)
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
	if has, _ := m.d.Store.StockTaken(); !has {
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
	if has, _ := m.d.Store.StockTaken(); has {
		t.Error("the scan ran with the setting off")
	}
}

// On a machine with no saves yet the first scan finds nothing to remember.
// The first game installed there is still news.
func TestNewGames_AnEmptyMachinesFirstGameIsAnnounced(t *testing.T) {
	m := newScanMachine(t)
	if got := m.announced(); len(got) != 0 {
		t.Fatalf("an empty machine announced %v", got)
	}
	m.install("FirstEver", true)
	if got := m.announced(); !got["FirstEver"] {
		t.Errorf("the first game on an empty machine was taken for the stock-taking: announced %v", got)
	}
}

// Found while nobody was looking, then the app restarted: the game is still
// waiting. It was remembered as known the moment it was found, so losing the
// waiting list meant it was never mentioned at all.
func TestNewGames_WaitingGamesSurviveARestart(t *testing.T) {
	home := t.TempDir()
	scanHome := t.TempDir()
	isolateProfile(t)
	open := func() *scanMachine {
		d, err := New(Options{HomeOverride: home, DisableDiscovery: true})
		if err != nil {
			t.Fatal(err)
		}
		d.Scanner = &presets.Scanner{
			CacheFile: filepath.Join(t.TempDir(), "cache.json"), GOOS: "linux", HomeDir: scanHome,
			SteamRoots: []string{}, SteamUserdataPaths: []string{}, EpicManifestDirs: []string{},
			InstallParentDirs: []string{}, LocalLowDir: filepath.Join(t.TempDir(), "x"), MountRoots: []string{},
		}
		return &scanMachine{t: t, d: d, home: scanHome}
	}
	first := open()
	first.install("OldFavourite", true)
	first.announced()
	first.install("WhileAway", true)
	if got := first.announced(); !got["WhileAway"] {
		t.Fatalf("setup: WhileAway was not announced: %v", got)
	}
	first.d.Stop()

	second := open()
	defer second.d.Stop()
	waiting := second.d.NewGames()
	if len(waiting) != 1 || filepath.Base(waiting[0].SavePath) != "WhileAway" {
		t.Errorf("after a restart the waiting list is %+v, want WhileAway", waiting)
	}
}

// isolateProfile points the user-profile variables at empty folders. Parts of
// the scanner resolve Windows conventions — Saved Games, Documents\My Games,
// %APPDATA% — from the environment rather than from the scanner's home, so
// without this these tests read the real profile of whoever runs them and an
// "empty machine" had forty-odd games on it.
func isolateProfile(t *testing.T) {
	t.Helper()
	for _, v := range []string{"USERPROFILE", "HOME", "APPDATA", "LOCALAPPDATA"} {
		t.Setenv(v, t.TempDir())
	}
}

// Stock is taken of everything the first scan finds, empty or not. A folder
// already there at that moment is not news later — whether it was empty, or
// the scan ran out of time before measuring it, which on a big library it
// does. Letting those through had the live app announce twenty-odd old games
// as new.
func TestNewGames_AFolderThereAtStockTakingIsNotNewsLater(t *testing.T) {
	m := newScanMachine(t)
	m.install("OldFavourite", true)
	m.install("InstalledLongAgo", false) // there, but no save in it yet
	m.announced()

	m.play("InstalledLongAgo")
	if got := m.announced(); got["InstalledLongAgo"] {
		t.Errorf("a game that was already there when stock was taken was announced as new: %v", got)
	}
}
