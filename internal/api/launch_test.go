package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// launchFixture is a daemon whose launches are recorded instead of made, on a
// machine of the test's making: a Steam library that has installed only what
// the test puts in it (none at all when steam is false), and a Games folder
// no launcher knows about. The game is Elden Ring, tracked with its App ID.
type launchFixture struct {
	ts           *testServer
	opened, ran  []string
	steam, games string
}

func newLaunchFixture(t *testing.T, steam bool) *launchFixture {
	t.Helper()
	f := &launchFixture{ts: startTestServer(t), games: filepath.Join(t.TempDir(), "Games")}
	prevOpen, prevRun := openURL, runExecutable
	openURL = func(u string) error { f.opened = append(f.opened, u); return nil }
	runExecutable = func(p string) error { f.ran = append(f.ran, p); return nil }
	t.Cleanup(func() { openURL, runExecutable = prevOpen, prevRun })

	sc := f.ts.daemon.Scanner
	sc.SteamRoots = []string{}
	if steam {
		f.steam = filepath.Join(t.TempDir(), "Steam")
		if err := os.MkdirAll(filepath.Join(f.steam, "steamapps"), 0o777); err != nil {
			t.Fatal(err)
		}
		sc.SteamRoots = []string{f.steam}
	}
	sc.InstallParentDirs = []string{f.games}
	sc.EpicManifestDirs = []string{}

	if err := os.WriteFile(filepath.Join(f.ts.saveDir, "slot1.sav"), []byte("x"), 0o666); err != nil {
		t.Fatal(err)
	}
	if resp, body := f.ts.do(t, http.MethodPost, "/api/games", map[string]string{"name": "Elden Ring", "savePath": f.ts.saveDir}); resp.StatusCode != http.StatusOK {
		t.Fatalf("track: %d %v", resp.StatusCode, body)
	}
	f.set(t, "1245620", "")
	return f
}

func (f *launchFixture) set(t *testing.T, appID, exe string) {
	t.Helper()
	game, err := f.ts.daemon.Store.GetGame("elden-ring")
	if err != nil {
		t.Fatal(err)
	}
	game.AppID, game.ExePath = appID, exe
	if err := f.ts.daemon.Store.UpdateGame(game); err != nil {
		t.Fatal(err)
	}
}

// launch asks for a launch and returns the status, the answer's error and
// reason, and forgets what earlier launches did.
func (f *launchFixture) launch(t *testing.T) (int, string, string) {
	t.Helper()
	f.opened, f.ran = nil, nil
	resp, body := f.ts.do(t, http.MethodPost, "/api/games/elden-ring/launch", nil)
	var msg, reason string
	_ = json.Unmarshal(body["error"], &msg)
	_ = json.Unmarshal(body["reason"], &reason)
	return resp.StatusCode, msg, reason
}

func writeProgram(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o777); err != nil {
		t.Fatal(err)
	}
	return path
}

// A game with a program set is started by that program even when it has a
// Steam App ID too: someone who set one chose how the game starts, and the App
// ID is often only there for its name and cover. Steam is for the rest.
func TestLaunchPrefersTheProgramOverSteam(t *testing.T) {
	f := newLaunchFixture(t, false)
	exe := writeProgram(t, filepath.Join(t.TempDir(), "Elden Ring", "Game", "eldenring.exe"))
	f.set(t, "1245620", exe)
	if code, msg, _ := f.launch(t); code != http.StatusOK {
		t.Fatalf("launch: %d %s", code, msg)
	}
	if len(f.ran) != 1 || f.ran[0] != exe || len(f.opened) != 0 {
		t.Errorf("with a program set: ran %q, opened %q; want only the program", f.ran, f.opened)
	}

	f.set(t, "1245620", "")
	if code, msg, _ := f.launch(t); code != http.StatusOK {
		t.Fatalf("launch: %d %s", code, msg)
	}
	if len(f.opened) != 1 || f.opened[0] != "steam://run/1245620" || len(f.ran) != 0 {
		t.Errorf("with no program: ran %q, opened %q; want Steam", f.ran, f.opened)
	}
}

// Steam asked to run a game it does not have offers to install it — not what
// Launch promised, and for a copy kept outside Steam a different copy
// altogether. Launch says so, and where the game is if it was found.
func TestLaunchSaysSoWhenSteamDoesNotHaveTheGame(t *testing.T) {
	f := newLaunchFixture(t, true)
	// The app is told too, and offers no Launch for it (lib/gameactions.js).
	_, games := f.ts.do(t, http.MethodGet, "/api/games", nil)
	var entry struct {
		Installed string `json:"installed"`
	}
	if err := json.Unmarshal(games["elden-ring"], &entry); err != nil || entry.Installed != "not-found" {
		t.Errorf("the game's entry says installed=%q (%v); want not-found", entry.Installed, err)
	}

	code, msg, reason := f.launch(t)
	if code != http.StatusConflict || reason != "not-installed" || len(f.opened) != 0 {
		t.Fatalf("Steam lacks the game: %d %q (%s), opened %q; want it said, and Steam left alone", code, msg, reason, f.opened)
	}

	copyDir := filepath.Join(f.games, "Elden Ring")
	writeProgram(t, filepath.Join(copyDir, "Game", "eldenring.exe"))
	code, msg, _ = f.launch(t)
	if code != http.StatusConflict || !strings.Contains(msg, copyDir) || len(f.opened) != 0 {
		t.Errorf("a copy in %s: %d %q, opened %q; want it named, and Steam left alone", copyDir, code, msg, f.opened)
	}

	// Installed by Steam after all: Steam starts it, read fresh — a game
	// installed a moment ago launches.
	acf := `"AppState" { "appid" "1245620" "name" "ELDEN RING" "installdir" "ELDEN RING" }`
	if err := os.WriteFile(filepath.Join(f.steam, "steamapps", "appmanifest_1245620.acf"), []byte(acf), 0o666); err != nil {
		t.Fatal(err)
	}
	if code, msg, _ := f.launch(t); code != http.StatusOK || len(f.opened) != 1 {
		t.Errorf("installed by Steam: %d %q, opened %q; want Steam to start it", code, msg, f.opened)
	}
}

// With no Steam on the machine there is no telling whether it has the game,
// so Steam is asked, as before.
func TestLaunchGoesToSteamWhenThereIsNoTelling(t *testing.T) {
	f := newLaunchFixture(t, false)
	if code, msg, _ := f.launch(t); code != http.StatusOK || len(f.opened) != 1 {
		t.Errorf("no Steam to ask: %d %q, opened %q; want Steam asked", code, msg, f.opened)
	}
}

// A program that has gone — uninstalled, a drive unplugged — is said to be
// gone, rather than failing somewhere in the operating system.
func TestLaunchSaysSoWhenTheProgramIsGone(t *testing.T) {
	f := newLaunchFixture(t, false)
	f.set(t, "", filepath.Join(t.TempDir(), "gone", "game.exe"))
	code, msg, reason := f.launch(t)
	if code != http.StatusConflict || reason != "program-missing" || len(f.ran) != 0 {
		t.Errorf("program gone: %d %q (%s), ran %q", code, msg, reason, f.ran)
	}
}

// Started in its own folder, where a game looks for its data; and a shortcut,
// which cannot be run, is opened the way double-clicking it would.
func TestLaunchCommand(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "Game", "game.exe")
	if cmd := launchCommand(exe); cmd.Dir != filepath.Dir(exe) {
		t.Errorf("started in %q, want the program's own folder %q", cmd.Dir, filepath.Dir(exe))
	}
	if runtime.GOOS == "windows" {
		lnk := filepath.Join(t.TempDir(), "Game.lnk")
		if cmd := launchCommand(lnk); filepath.Base(cmd.Args[0]) != "rundll32" || cmd.Args[len(cmd.Args)-1] != lnk {
			t.Errorf("a shortcut was started as %q, want it opened through the shell", cmd.Args)
		}
	}
}

// Browse for a game's program opens where the game is installed, when that
// is known: Steam's folder for it once Steam has it, and nothing before.
func TestInstallFolder(t *testing.T) {
	f := newLaunchFixture(t, true)
	folder := func() string {
		t.Helper()
		resp, body := f.ts.do(t, http.MethodGet, "/api/games/elden-ring/install-folder", nil)
		var dir string
		_ = json.Unmarshal(body["dir"], &dir)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status %d", resp.StatusCode)
		}
		return dir
	}
	if dir := folder(); dir != "" {
		t.Fatalf("not installed, yet the folder is %q", dir)
	}
	acf := `"AppState" { "appid" "1245620" "name" "ELDEN RING" "installdir" "ELDEN RING" }`
	if err := os.WriteFile(filepath.Join(f.steam, "steamapps", "appmanifest_1245620.acf"), []byte(acf), 0o666); err != nil {
		t.Fatal(err)
	}
	// Steam lists it, but its folder is not there: not offered.
	if dir := folder(); dir != "" {
		t.Fatalf("a folder that is not there was offered: %q", dir)
	}
	want := filepath.Join(f.steam, "steamapps", "common", "ELDEN RING")
	if err := os.MkdirAll(want, 0o777); err != nil {
		t.Fatal(err)
	}
	if dir := folder(); dir != want {
		t.Errorf("install folder %q, want %q", dir, want)
	}
	if resp, _ := f.ts.do(t, http.MethodGet, "/api/games/nope/install-folder", nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown game answered %d, want 404", resp.StatusCode)
	}
}
