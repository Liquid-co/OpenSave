package daemon

import (
	"errors"
	"testing"

	"github.com/opensave/opensave/internal/snapshot"
)

// A game whose automatic snapshots keep coming out incomplete is said on
// screen once per run of them; a snapshot that succeeds ends the run, and
// failures of any other kind are left to the log.
func TestIncompleteAutoSnapshotsAreSaidOncePerRun(t *testing.T) {
	var said []SnapshotsIncomplete
	d := &Daemon{OnSnapshotsIncomplete: func(s SnapshotsIncomplete) { said = append(said, s) }}
	incomplete := &snapshot.IncompleteError{Game: "Hades", Skipped: []string{"/s/a.log", "/s/b.lock", "/s/c", "/s/d"}}

	for i := 0; i < incompleteAutoSnapshotsToSay-1; i++ {
		d.autoSnapshotGaveUp("hades", incomplete)
	}
	d.autoSnapshotGaveUp("hades", errors.New("disk full"))
	if len(said) != 0 {
		t.Fatalf("said after %d incomplete passes: %v", incompleteAutoSnapshotsToSay-1, said)
	}
	d.autoSnapshotGaveUp("hades", incomplete)
	if len(said) != 1 {
		t.Fatalf("said %d time(s) after %d incomplete passes, want once", len(said), incompleteAutoSnapshotsToSay)
	}
	got := said[0]
	if got.GameID != "hades" || got.GameName != "Hades" || got.Count != 4 || len(got.Files) != 3 || got.Files[0] != "a.log" {
		t.Errorf("said %+v", got)
	}
	d.autoSnapshotGaveUp("hades", incomplete)
	if len(said) != 1 {
		t.Errorf("said again within the same run")
	}

	d.autoSnapshotTaken("hades")
	for i := 0; i < incompleteAutoSnapshotsToSay; i++ {
		d.autoSnapshotGaveUp("hades", incomplete)
	}
	if len(said) != 2 {
		t.Errorf("a new run after a good snapshot was said %d time(s), want once more", len(said)-1)
	}
}
