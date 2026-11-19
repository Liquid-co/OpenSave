package syncengine

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// keep-remote is offered as undoable because it snapshots this device's
// version first. When that copy cannot be made, the overwrite does not
// happen: the conflict stays, and so does the local save.
func TestConflict_KeepRemoteStopsWhenTheSafetyCopyFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("read-only folders do not refuse new files on Windows")
	}
	env := setupEngine(t)
	write(t, env.localDir, "save.dat", "local version")
	write(t, env.remoteDir, "save.dat", "remote version")
	if err := env.store.SetSyncState("game1", env.peer.ID, []string{"save.dat"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := env.store.UpdatePeerLastSynced(env.peer.ID, "2026-01-01T00:00:00.000Z"); err != nil {
		t.Fatal(err)
	}
	if res, err := env.engine.SyncWithPeer(context.Background(), "game1", env.peer); err != nil || res.Status != "conflict" {
		t.Fatalf("expected conflict, got %+v err=%v", res, err)
	}

	// The backups folder refuses new snapshots from here on.
	backups := filepath.Join(filepath.Dir(env.localDir), "backups")
	if err := os.MkdirAll(backups, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(backups, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(backups, 0o777) })

	if _, err := env.engine.ResolveConflict(context.Background(), "game1", env.peer.ID, "keep-remote"); err == nil {
		t.Fatal("keep-remote succeeded with no copy of the local version")
	}
	if got, _ := os.ReadFile(filepath.Join(env.localDir, "save.dat")); string(got) != "local version" {
		t.Errorf("local after refused keep-remote = %q, want it untouched", got)
	}
	if len(env.engine.ActiveConflicts()) != 1 {
		t.Error("the conflict should still be pending")
	}
}
