package daemon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/opensave/opensave/internal/store"
)

// One game that cannot be snapshotted is reported by name and does not stop
// the others.
func TestSnapshotAllCarriesOnPastAFailure(t *testing.T) {
	d := newTestDaemon(t)
	good := t.TempDir()
	if err := os.WriteFile(filepath.Join(good, "slot1.sav"), []byte("x"), 0o666); err != nil {
		t.Fatal(err)
	}
	// A save path inside a regular file: no folder can ever be made there.
	blocker := filepath.Join(t.TempDir(), "not-a-folder")
	if err := os.WriteFile(blocker, []byte("x"), 0o666); err != nil {
		t.Fatal(err)
	}
	for _, g := range []store.Game{
		{ID: "broken", Name: "Broken", SavePath: filepath.Join(blocker, "saves"), ActiveBranch: "main", MaxSnapshots: 5},
		{ID: "good", Name: "Good", SavePath: good, ActiveBranch: "main", MaxSnapshots: 5},
	} {
		if err := d.Store.CreateGame(g); err != nil {
			t.Fatal(err)
		}
	}

	res := d.SnapshotAll("before the reinstall")
	if res.Taken != 1 {
		t.Errorf("taken = %d, want 1", res.Taken)
	}
	if len(res.Failed) != 1 || res.Failed[0].Name != "Broken" || res.Failed[0].Error == "" {
		t.Errorf("failed = %+v, want Broken with its reason", res.Failed)
	}
	snaps, _ := d.Store.ListSnapshots("good", "main")
	if len(snaps) != 1 || snaps[0].Comment != "before the reinstall" || snaps[0].IsSystemAuto {
		t.Errorf("good game's snapshots = %+v, want one manual snapshot with the comment", snaps)
	}
}
