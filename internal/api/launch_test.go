package api

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// A game with a program set is started by that program even when it has a
// Steam App ID too: someone who set one chose how the game starts, and the App
// ID is often only there for its name and cover. Steam is for the rest.
func TestLaunchPrefersTheProgramOverSteam(t *testing.T) {
	ts := startTestServer(t)
	var opened, ran []string
	prevOpen, prevRun := openURL, runExecutable
	openURL = func(u string) error { opened = append(opened, u); return nil }
	runExecutable = func(p string) error { ran = append(ran, p); return nil }
	t.Cleanup(func() { openURL, runExecutable = prevOpen, prevRun })

	if err := os.WriteFile(filepath.Join(ts.saveDir, "slot1.sav"), []byte("x"), 0o666); err != nil {
		t.Fatal(err)
	}
	resp, body := ts.do(t, http.MethodPost, "/api/games", map[string]string{"name": "Elden Ring", "savePath": ts.saveDir})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("track: %d %v", resp.StatusCode, body)
	}
	game, err := ts.daemon.Store.GetGame("elden-ring")
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(t.TempDir(), "Elden Ring", "Game", "eldenring.exe")
	game.AppID, game.ExePath = "1245620", exe
	if err := ts.daemon.Store.UpdateGame(game); err != nil {
		t.Fatal(err)
	}

	if resp, body := ts.do(t, http.MethodPost, "/api/games/elden-ring/launch", nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("launch: %d %v", resp.StatusCode, body)
	}
	if len(ran) != 1 || ran[0] != exe || len(opened) != 0 {
		t.Errorf("with a program set: ran %q, opened %q; want only the program", ran, opened)
	}

	game.ExePath = ""
	if err := ts.daemon.Store.UpdateGame(game); err != nil {
		t.Fatal(err)
	}
	ran, opened = nil, nil
	if resp, body := ts.do(t, http.MethodPost, "/api/games/elden-ring/launch", nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("launch: %d %v", resp.StatusCode, body)
	}
	if len(opened) != 1 || opened[0] != "steam://run/1245620" || len(ran) != 0 {
		t.Errorf("with no program: ran %q, opened %q; want Steam", ran, opened)
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
