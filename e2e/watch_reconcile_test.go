package e2e

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/opensave/opensave/testutil"
)

// The periodic watch reconcile.
//
// A watch is the only thing standing between a save being written and OpenSave
// noticing. When one fails to start — the folder did not exist yet because the
// game had not been run, the OS refused a handle, the drive was not mounted —
// the failure is a single warning in the log and then silence. Everything
// looks tracked. Nothing is watched. That is the shape of the one open user
// report of a save going missing, so what matters here is not that a reconcile
// function exists but that a watch it revives actually reports changes
// afterwards.
//
// None of this had a test. Unwatching directly is how these tests reproduce a
// watch that is not running, because a watch that failed to start and one that
// was stopped are the same state from the reconcile's point of view.

// watching reports whether the daemon currently watches a game.
func watching(td *testutil.TestDaemon, gameID string) bool {
	_, ok := td.Daemon.Watcher.Watching(gameID)
	return ok
}

// waitForWatch waits for the watch a track starts.
//
// Tracking a game returns before its watch exists: the initial snapshot runs
// on a goroutine and the watch is started after it finishes, so the API has
// already answered by the time anything is being watched. Harmless in use —
// the gap is a snapshot long, and the reconcile would catch a watch that never
// arrived — but a test that reads the watch set immediately reads it empty.
func waitForWatch(t *testing.T, td *testutil.TestDaemon, gameID string) {
	t.Helper()
	if !testutil.WaitFor(30*time.Second, func() bool { return watching(td, gameID) }) {
		t.Fatalf("tracking %s never started a watch", gameID)
	}
}

// countSnapshots is the observable proof that a watch is live.
func countSnapshots(td *testutil.TestDaemon, gameID string) int {
	return len(snapshotsOn(td, gameID, "main"))
}

// The case the reconcile exists for, tested by its effect rather than its
// return value: a watch that is not running comes back, and the folder it
// covers is genuinely being watched again.
func TestResyncWatchers_RevivesAWatchAndItWorksAfterwards(t *testing.T) {
	d := testutil.NewTestDaemon(t, "Reconcile")
	d.WriteSave("slot1.sav", "start")
	gameID := d.TrackGame("Reconcile Game")
	waitForWatch(t, d, gameID)

	// The state a failed start leaves behind: tracked, auto-sync on, unwatched.
	d.Daemon.Watcher.Unwatch(gameID)
	if watching(d, gameID) {
		t.Fatal("setup: the watch did not stop")
	}

	started, stopped := d.Daemon.ResyncWatchers()
	if started != 1 {
		t.Errorf("reconcile started %d watches, want 1 — a game that is tracked with auto-sync on "+
			"and not being watched is exactly what this is for", started)
	}
	if stopped != 0 {
		t.Errorf("reconcile stopped %d watches, want 0", stopped)
	}
	if !watching(d, gameID) {
		t.Fatal("the game is still not watched after a reconcile")
	}

	// The part that matters. A watch that is registered but not delivering
	// events would satisfy every check above and still lose the save.
	before := countSnapshots(d, gameID)
	d.WriteSave("slot1.sav", "changed after the watch was revived")
	if !testutil.WaitFor(60*time.Second, func() bool {
		return countSnapshots(d, gameID) > before
	}) {
		t.Error("a save written after the reconcile was never snapshotted; the watch was " +
			"restored in name only, which is indistinguishable from the bug it fixes")
	}
}

// Turning auto-sync off has to stop the watch. Otherwise a game the user has
// explicitly told OpenSave to leave alone keeps producing snapshots.
func TestResyncWatchers_StopsWatchingWhenAutoSyncIsOff(t *testing.T) {
	d := testutil.NewTestDaemon(t, "ReconcileOff")
	d.WriteSave("slot1.sav", "start")
	gameID := d.TrackGame("AutoSync Off Game")
	waitForWatch(t, d, gameID)

	d.API(http.MethodPatch, "/api/games/"+gameID, map[string]any{"autoSync": false}, nil)

	// As with untracking, the switch usually stops the watch itself. Put it
	// back so what is under test is the reconcile rather than the thing that
	// normally gets there first.
	if !watching(d, gameID) {
		if err := d.Daemon.Watcher.Watch(gameID, d.SaveDir); err != nil {
			t.Fatal(err)
		}
	}

	started, stopped := d.Daemon.ResyncWatchers()
	if stopped != 1 {
		t.Errorf("reconcile stopped %d watches, want 1 for a game with auto-sync switched off", stopped)
	}
	if started != 0 {
		t.Errorf("reconcile started %d watches for a game that should not be watched at all", started)
	}
	if watching(d, gameID) {
		t.Error("a game with auto-sync off is still being watched")
	}
}

// An untracked game must not keep a watch. A stale watch on a folder with no
// game behind it fires changes nothing can act on, and holds a handle on a
// directory the user may be trying to delete.
func TestResyncWatchers_StopsWatchingAnUntrackedGame(t *testing.T) {
	d := testutil.NewTestDaemon(t, "ReconcileUntrack")
	d.WriteSave("slot1.sav", "start")
	gameID := d.TrackGame("Untracked Game")
	waitForWatch(t, d, gameID)

	d.API(http.MethodDelete, "/api/games/"+gameID, nil, nil)

	// Untracking normally stops the watch itself; the reconcile is the
	// backstop for when it did not, so put the watch back to test the backstop
	// rather than the thing that usually beats it to it.
	if !watching(d, gameID) {
		if err := d.Daemon.Watcher.Watch(gameID, d.SaveDir); err != nil {
			t.Fatal(err)
		}
	}

	_, stopped := d.Daemon.ResyncWatchers()
	if stopped != 1 {
		t.Errorf("reconcile stopped %d watches, want 1 for an untracked game", stopped)
	}
	if watching(d, gameID) {
		t.Error("a game that is no longer tracked is still being watched")
	}
}

// A save that moved must be watched where it is now, not where it was. This is
// the branch that compares paths, and getting it wrong means watching an empty
// folder forever while the real saves change unobserved.
func TestResyncWatchers_FollowsARelocatedSave(t *testing.T) {
	d := testutil.NewTestDaemon(t, "ReconcileMove")
	d.WriteSave("slot1.sav", "start")
	gameID := d.TrackGame("Relocated Game")
	waitForWatch(t, d, gameID)

	moved := filepath.Join(t.TempDir(), "new-location")
	if err := os.MkdirAll(moved, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(moved, "slot1.sav"), []byte("start"), 0o666); err != nil {
		t.Fatal(err)
	}
	d.API(http.MethodPatch, "/api/games/"+gameID, map[string]any{"savePath": moved}, nil)

	started, _ := d.Daemon.ResyncWatchers()
	current, ok := d.Daemon.Watcher.Watching(gameID)
	if !ok {
		t.Fatal("the relocated game is not watched at all")
	}
	if !samePath(current, moved) {
		t.Errorf("still watching %q after the save moved to %q (reconcile started %d)",
			current, moved, started)
	}

	before := countSnapshots(d, gameID)
	if err := os.WriteFile(filepath.Join(moved, "slot1.sav"), []byte("changed at the new location"), 0o666); err != nil {
		t.Fatal(err)
	}
	if !testutil.WaitFor(60*time.Second, func() bool {
		return countSnapshots(d, gameID) > before
	}) {
		t.Error("a change at the new save location was never noticed")
	}
}

// Reconciling when nothing has changed must do nothing. It runs every minute
// for the life of the process, so a reconcile that restarted watches
// needlessly would churn every fsnotify handle on the machine once a minute
// and drop whatever events landed in the gap.
func TestResyncWatchers_IsQuietWhenNothingChanged(t *testing.T) {
	d := testutil.NewTestDaemon(t, "ReconcileQuiet")
	d.WriteSave("slot1.sav", "start")
	gameID := d.TrackGame("Steady Game")
	waitForWatch(t, d, gameID)

	for i := 0; i < 3; i++ {
		started, stopped := d.Daemon.ResyncWatchers()
		if started != 0 || stopped != 0 {
			t.Fatalf("reconcile %d changed the watch list with nothing to do: started=%d stopped=%d",
				i+1, started, stopped)
		}
	}
	if !watching(d, gameID) {
		t.Error("repeated reconciles left the game unwatched")
	}

	// And the watch still works after them.
	before := countSnapshots(d, gameID)
	d.WriteSave("slot1.sav", "changed after three reconciles")
	if !testutil.WaitFor(60*time.Second, func() bool {
		return countSnapshots(d, gameID) > before
	}) {
		t.Error("the watch stopped reporting changes after repeated reconciles")
	}
}

// samePath compares two paths the way the filesystem does, so a case or
// separator difference is not read as a relocation.
func samePath(a, b string) bool {
	ca, err1 := filepath.Abs(filepath.Clean(a))
	cb, err2 := filepath.Abs(filepath.Clean(b))
	if err1 != nil || err2 != nil {
		return a == b
	}
	if ra, err := filepath.EvalSymlinks(ca); err == nil {
		ca = ra
	}
	if rb, err := filepath.EvalSymlinks(cb); err == nil {
		cb = rb
	}
	return strings.EqualFold(ca, cb)
}
