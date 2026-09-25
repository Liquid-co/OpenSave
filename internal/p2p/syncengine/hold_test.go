package syncengine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/opensave/opensave/internal/store"
)

// What an emptied location held is taken from the newest snapshot with files
// as well as from the record of what the two devices share. The record lags:
// a file pushed to the other device joins it only once that device says it
// has the file, while the other device's own record — the one it deletes by
// — has it already. The snapshot does not lag.
func TestHoldCountsTheNewestSnapshotAsWellAsTheSharedRecord(t *testing.T) {
	env := setupEngine(t)
	write(t, env.localDir, "old.sav", "shared long ago")
	if err := env.store.SetSyncState("game1", env.peer.ID, []string{"old.sav"}, nil); err != nil {
		t.Fatal(err)
	}
	// A new save, snapshotted, and not yet in the record.
	write(t, env.localDir, "new.sav", "just written")
	if _, err := env.engine.Snapshots.Create("game1", "", true); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"old.sav", "new.sav"} {
		if err := os.Remove(filepath.Join(env.localDir, f)); err != nil {
			t.Fatal(err)
		}
	}

	held, err := env.engine.CheckHold("game1", false)
	if err != nil || !held {
		t.Fatalf("CheckHold = %v, %v; want held", held, err)
	}
	h, _, _ := env.store.GetDeletionHold("game1")
	got := h.Files()[""]
	if len(got) != 2 || got[0] != "new.sav" || got[1] != "old.sav" {
		t.Errorf("the hold covers %v; want new.sav from the snapshot and old.sav from the record", got)
	}

	// Only new.sav back is not every file back.
	write(t, env.localDir, "new.sav", "just written")
	if held, _ := env.engine.CheckHold("game1", false); !held {
		t.Error("the hold let go with old.sav still gone")
	}
	write(t, env.localDir, "old.sav", "shared long ago")
	if held, _ := env.engine.CheckHold("game1", false); held {
		t.Error("the hold did not let go with every file back")
	}
	if _, has, _ := env.store.GetDeletionHold("game1"); has {
		t.Error("the hold is still recorded")
	}
}

// A game that has never synced with another device has nothing to hold back
// from, and an emptied folder there is left alone.
func TestNoHoldForAGameThatNeverSynced(t *testing.T) {
	env := setupEngine(t)
	write(t, env.localDir, "a.sav", "x")
	if _, err := env.engine.Snapshots.Create("game1", "", true); err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(env.localDir, "a.sav"))
	if held, err := env.engine.CheckHold("game1", false); err != nil || held {
		t.Errorf("CheckHold = %v, %v; want nothing held", held, err)
	}
	if _, has, _ := env.store.GetDeletionHold("game1"); has {
		t.Error("a hold was recorded")
	}
}

// While files are still being fetched after "put them back", this device
// syncs and the other devices are still shown nothing.
func TestFetchingSyncsHereAndServesNothing(t *testing.T) {
	env := setupEngine(t)
	if err := env.store.SetDeletionHold("game1", store.HoldFetching, 1, map[string][]string{"": {"a.sav"}}); err != nil {
		t.Fatal(err)
	}
	if err := env.store.SetSyncState("game1", env.peer.ID, []string{"a.sav"}, nil); err != nil {
		t.Fatal(err)
	}
	if held, _ := env.engine.CheckHold("game1", false); held {
		t.Error("this device may not sync to fetch the files")
	}
	if held, _ := env.engine.CheckHold("game1", true); !held {
		t.Error("another device was shown the folder while files were still to come")
	}
}
