package api

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/opensave/opensave/internal/store"
)

// Turning the setting on has to do something a person can see: the old
// automatic snapshots are gone from the game's history without waiting for
// a scheduled sweep. The setting existed, in Settings, for a long time
// while nothing read it.
func TestSettings_AutoDeleteOldSnapshots_SweepsWhenSwitchedOn(t *testing.T) {
	ts := startTestServer(t)
	if err := os.WriteFile(filepath.Join(ts.saveDir, "slot1.sav"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	resp, body := ts.do(t, http.MethodPost, "/api/games", map[string]any{
		"name": "Aged Game", "savePath": ts.saveDir, "maxSnapshots": 0,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("track: %d %s", resp.StatusCode, body)
	}
	const gameID = "aged-game"

	// Tracking takes an initial snapshot in the background; wait for it, then
	// add an old automatic one behind it and a manual one older still. The
	// initial snapshot is the newest on the branch and must survive.
	if !waitFor(20*time.Second, func() bool {
		snaps, _ := ts.daemon.Store.ListSnapshots(gameID, "main")
		return len(snaps) == 1
	}) {
		t.Fatal("setup: the initial snapshot never landed")
	}
	newest, _ := ts.daemon.Store.ListSnapshots(gameID, "main")
	backups := filepath.Dir(newest[0].ZipPath)
	plant := func(id, stamp string, auto bool) {
		zip := filepath.Join(backups, id+".zip")
		if err := os.WriteFile(zip, []byte("zip"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := ts.daemon.Store.CreateSnapshot(store.Snapshot{
			ID: id, GameID: gameID, BranchName: "main", Timestamp: stamp,
			IsSystemAuto: auto, ZipPath: zip, SizeBytes: 3,
		}); err != nil {
			t.Fatal(err)
		}
	}
	plant("snap_old_auto", "2026-01-01T00:00:00.000Z", true)
	plant("snap_old_manual", "2025-12-01T00:00:00.000Z", false)

	resp, body = ts.do(t, http.MethodPost, "/api/settings", map[string]any{
		"autoDeleteBackups": true, "autoDeleteDays": 30,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("settings: %d %s", resp.StatusCode, body)
	}

	if !waitFor(10*time.Second, func() bool {
		_, err := ts.daemon.Store.GetSnapshot("snap_old_auto")
		return err != nil
	}) {
		t.Errorf("switching the setting on did not remove the old automatic snapshot")
	}
	if _, err := ts.daemon.Store.GetSnapshot("snap_old_manual"); err != nil {
		t.Errorf("the manual snapshot was deleted by age")
	}
	if _, err := ts.daemon.Store.GetSnapshot(newest[0].ID); err != nil {
		t.Errorf("the branch's newest snapshot was deleted")
	}
	if _, err := os.Stat(filepath.Join(backups, "snap_old_auto.zip")); err == nil {
		t.Errorf("the deleted snapshot's archive is still on disk")
	}
}

func waitFor(timeout time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return cond()
}
