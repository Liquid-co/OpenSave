package syncengine

import (
	"context"
	"testing"
)

// The snapshot recorded under the peer's id after a pull records what it
// holds, like any snapshot here: the next snapshot reuses it rather than
// compressing the whole save again.
func TestSync_TheSnapshotAfterAPullRecordsItsFiles(t *testing.T) {
	env := setupEngine(t)
	write(t, env.remoteDir, "save.dat", "remote progress")
	write(t, env.remoteDir, "profile.dat", "remote profile")
	env.transport.latestSnap = &SnapshotInfo{ID: "snap_777", Timestamp: "2026-06-01T00:00:00.000Z", Comment: "peer snap"}

	if _, err := env.engine.SyncWithPeer(context.Background(), "game1", env.peer); err != nil {
		t.Fatal(err)
	}
	if _, err := env.store.GetSnapshot("snap_777"); err != nil {
		t.Fatalf("mirror snapshot not recorded: %v", err)
	}
	files, err := env.store.SnapshotFiles("snap_777")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Errorf("the snapshot after the pull records %d file(s), want 2: %+v", len(files), files)
	}
}

// The snapshot recorded after a pull is the save as it now stands here, and
// goes to the cloud like any new snapshot. It used not to, so a device that
// took its save from another by syncing never had it backed up.
func TestSync_TheSnapshotAfterAPullIsUploaded(t *testing.T) {
	env := setupEngine(t)
	var sent []string
	env.engine.Snapshots.OnUpload = func(zipPath, remoteName string) { sent = append(sent, remoteName) }
	write(t, env.remoteDir, "save.dat", "remote progress")
	env.transport.latestSnap = &SnapshotInfo{ID: "snap_778", Timestamp: "2026-06-01T00:00:00.000Z", Comment: "peer snap"}

	if _, err := env.engine.SyncWithPeer(context.Background(), "game1", env.peer); err != nil {
		t.Fatal(err)
	}
	want := "game1__main__snap_778.zip"
	for _, n := range sent {
		if n == want {
			return
		}
	}
	t.Fatalf("uploads %v, want %s among them", sent, want)
}

// Which branch the other device answered with is kept, for the game page to
// say which devices are on which branch as of the last sync.
func TestSync_NotesTheBranchThePeerIsOn(t *testing.T) {
	env := setupEngine(t)
	write(t, env.remoteDir, "save.dat", "remote progress")
	if _, err := env.engine.SyncWithPeer(context.Background(), "game1", env.peer); err != nil {
		t.Fatal(err)
	}
	got, err := env.store.PeerBranches("game1")
	if err != nil || got[env.peer.ID].Branch != "main" || got[env.peer.ID].SeenMs == 0 {
		t.Fatalf("peer branches %+v (err %v), want main with a time", got, err)
	}
}
