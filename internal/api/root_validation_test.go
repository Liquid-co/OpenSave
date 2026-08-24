package api

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func trackProbeGame(t *testing.T, ts *testServer) string {
	t.Helper()
	resp, _ := ts.do(t, http.MethodPost, "/api/games",
		map[string]string{"name": "Probe", "savePath": ts.saveDir})
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("tracking returned %d", resp.StatusCode)
	}
	games, err := ts.daemon.Store.ListGames()
	if err != nil || len(games) != 1 {
		t.Fatalf("want one tracked game, got %d (err %v)", len(games), err)
	}
	return games[0].ID
}

// An extra save location is watched, hashed, synced to every peer and uploaded
// to the cloud. Pointing one at a profile folder therefore syncs a user's
// Documents to their other devices — and a restore then refuses to clear it,
// which leaves the game unrestorable.
//
// The CLI has always run this check before adding a location. The API went
// straight to the store, so the same folder was accepted through the app and
// refused on the command line.
func TestAddingASaveLocationRefusesAProfileFolder(t *testing.T) {
	home := t.TempDir()
	setHome(t, home)
	for _, sub := range []string{"Documents", "Desktop"} {
		if err := os.MkdirAll(filepath.Join(home, sub), 0o777); err != nil {
			t.Fatal(err)
		}
	}
	ts := startTestServer(t)
	id := trackProbeGame(t, ts)

	for _, bad := range []string{
		home,
		filepath.Join(home, "Documents"),
		filepath.Join(home, "Desktop"),
	} {
		resp, _ := ts.do(t, http.MethodPost, "/api/games/"+id+"/roots",
			map[string]string{"name": "config", "path": bad})
		if resp.StatusCode == http.StatusOK {
			t.Errorf("POST roots accepted %q — the CLI refuses this folder, and syncing it "+
				"would send a user's own files to their peers", bad)
		}
		if roots, _ := ts.daemon.Store.GameRootPaths(id); roots["config"] != "" {
			t.Errorf("%q was stored as a save location anyway", bad)
			_ = ts.daemon.Store.RemoveGameRoot(id, "config")
		}
	}
}

// The refusal must not block an ordinary location, which is the whole feature.
func TestAddingAnOrdinarySaveLocationStillWorks(t *testing.T) {
	home := t.TempDir()
	setHome(t, home)
	ts := startTestServer(t)
	id := trackProbeGame(t, ts)

	dir := filepath.Join(t.TempDir(), "GameConfig")
	if err := os.MkdirAll(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	resp, _ := ts.do(t, http.MethodPost, "/api/games/"+id+"/roots",
		map[string]string{"name": "config", "path": dir})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("an ordinary folder was refused: %d", resp.StatusCode)
	}
	if roots, _ := ts.daemon.Store.GameRootPaths(id); roots["config"] != filepath.Clean(dir) {
		t.Errorf("stored %q, want %q", roots["config"], dir)
	}
}

// A location learned from a peer is recorded with no path until this device is
// told where it lives. That must stay allowed, or pairing breaks.
func TestASaveLocationWithNoPathIsStillAllowed(t *testing.T) {
	home := t.TempDir()
	setHome(t, home)
	ts := startTestServer(t)
	id := trackProbeGame(t, ts)

	resp, _ := ts.do(t, http.MethodPost, "/api/games/"+id+"/roots",
		map[string]string{"name": "config", "path": ""})
	if resp.StatusCode != http.StatusOK {
		t.Errorf("an unmapped location was refused: %d — a location learned from a peer "+
			"has no path here until someone points it somewhere", resp.StatusCode)
	}
}
