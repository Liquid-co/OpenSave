package api

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// PATCH decodes straight into the stored game and wrote it back, so a save
// path that tracking would refuse could be set here instead — a profile root,
// a drive root, or one that does not exist at all. Everything downstream
// assumes the path it was handed came past the track-time check: the watcher
// walks it, snapshots zip it, and a restore empties it before unpacking. It is
// how a game comes to be tracked at a whole home folder despite `add`
// refusing to do it.
func TestUpdateGameRefusesASavePathTrackingWouldRefuse(t *testing.T) {
	// A temp directory standing in as the profile, so that a build with the
	// guard removed points the watcher at scratch space instead of the real
	// home folder. setHome moves every home-derived variable, which is what
	// checkSavePathShape reads.
	home := t.TempDir()
	setHome(t, home)
	for _, sub := range []string{"Documents", "Desktop"} {
		if err := os.MkdirAll(filepath.Join(home, sub), 0o777); err != nil {
			t.Fatal(err)
		}
	}

	ts := startTestServer(t)
	gameID := ts.mustTrack(t)
	driveRoot := "/"
	if runtime.GOOS == "windows" {
		driveRoot = filepath.VolumeName(home) + `\`
	}

	for _, bad := range []string{
		home,                             // the whole profile
		filepath.Join(home, "Documents"), // a whole profile folder
		driveRoot,                        // the drive itself
		filepath.Join(home, "no-such-dir-opensave-probe"), // does not exist
	} {
		resp, _ := ts.do(t, http.MethodPatch, "/api/games/"+gameID,
			map[string]any{"name": "probe", "savePath": bad})
		if resp.StatusCode == http.StatusOK {
			t.Errorf("PATCH accepted savePath %q — tracking refuses this path, and every "+
				"guard downstream assumes it was refused here too", bad)
		}
		// Whatever the answer, the stored path must not have moved.
		g, err := ts.daemon.Store.GetGame(gameID)
		if err != nil {
			t.Fatalf("GetGame: %v", err)
		}
		if g.SavePath != filepath.Clean(ts.saveDir) {
			t.Fatalf("savePath moved to %q after a refused PATCH", g.SavePath)
		}
	}
}

// The refusal must not block ordinary edits, including moving a save to a
// legitimate new folder.
func TestUpdateGameStillAcceptsARealMove(t *testing.T) {
	ts := startTestServer(t)
	gameID := ts.mustTrack(t)

	dest := filepath.Join(t.TempDir(), "MovedSaves")
	if err := os.MkdirAll(dest, 0o777); err != nil {
		t.Fatal(err)
	}
	resp, _ := ts.do(t, http.MethodPatch, "/api/games/"+gameID,
		map[string]any{"name": "probe", "savePath": dest})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH to a real folder returned %d, want 200", resp.StatusCode)
	}
	g, err := ts.daemon.Store.GetGame(gameID)
	if err != nil {
		t.Fatal(err)
	}
	if g.SavePath != filepath.Clean(dest) {
		t.Errorf("savePath = %q, want %q", g.SavePath, dest)
	}
}

// An edit that does not touch the save path must keep working even when the
// folder is currently unreachable — re-validating an unchanged path would
// reject the game against itself as a duplicate.
func TestUpdateGameLeavesAnUnchangedPathAlone(t *testing.T) {
	ts := startTestServer(t)
	gameID := ts.mustTrack(t)

	resp, _ := ts.do(t, http.MethodPatch, "/api/games/"+gameID,
		map[string]any{"name": "renamed", "savePath": ts.saveDir})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH with an unchanged path returned %d, want 200", resp.StatusCode)
	}
}

// mustTrack tracks ts.saveDir the way the app does and returns the id the
// daemon assigned, so these tests do not depend on how ids are derived.
func (ts *testServer) mustTrack(t *testing.T) string {
	t.Helper()
	resp, _ := ts.do(t, http.MethodPost, "/api/games",
		map[string]string{"name": "Probe Game", "savePath": ts.saveDir})
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("tracking the probe game returned %d", resp.StatusCode)
	}
	games, err := ts.daemon.Store.ListGames()
	if err != nil || len(games) != 1 {
		t.Fatalf("want exactly one tracked game, got %d (err %v)", len(games), err)
	}
	return games[0].ID
}
