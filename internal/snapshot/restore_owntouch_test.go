package snapshot

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/opensave/opensave/internal/owntouch"
)

// A restore's own writes and removals are OpenSave's (owntouch), so the
// watcher does not take the restored save for the game saving and snapshot it
// again straight after.
func TestRestoreMarksItsOwnChanges(t *testing.T) {
	env := setup(t)
	writeSave(t, env.saveDir, "slot1.sav", "old")
	writeSave(t, env.saveDir, "profiles/a/profile.sav", "old profile")
	snap, err := env.mgr.Create("game1", "before", false)
	if err != nil {
		t.Fatal(err)
	}

	// Played on: a file changed, a new folder and file the restore removes.
	writeSave(t, env.saveDir, "slot1.sav", "newer progress")
	writeSave(t, env.saveDir, "later/extra.sav", "made after the snapshot")

	if _, err := env.mgr.Restore("game1", snap.ID); err != nil {
		t.Fatal(err)
	}

	for _, rel := range []string{"slot1.sav", "profiles/a/profile.sav", "profiles/a", "profiles"} {
		if !owntouch.Recent(filepath.Join(env.saveDir, filepath.FromSlash(rel))) {
			t.Errorf("%s, put back by the restore, does not count as OpenSave's", rel)
		}
	}
	for _, rel := range []string{"later/extra.sav", "later"} {
		p := filepath.Join(env.saveDir, filepath.FromSlash(rel))
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("%s should be gone after the restore", rel)
		}
		if !owntouch.Recent(p) {
			t.Errorf("%s, removed by the restore, does not count as OpenSave's", rel)
		}
	}

	// A game saving over a restored file afterwards is the game's change.
	slot := filepath.Join(env.saveDir, "slot1.sav")
	if err := os.WriteFile(slot, []byte("the game saved again, longer"), 0o666); err != nil {
		t.Fatal(err)
	}
	if owntouch.Recent(slot) {
		t.Error("a save written after the restore still counts as OpenSave's")
	}
}
