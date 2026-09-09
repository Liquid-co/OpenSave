package watcher

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/opensave/opensave/internal/delta"
)

// Snapshotting a change that happened while nobody was watching.
//
// A watch reports what happens next; it says nothing about what happened while
// it was not running. Between OpenSave being closed and reopened, or a watch
// failing to start and being retried a minute later, a save could change with
// no snapshot ever taken of it — the peer reconcile would still carry the
// files to another device, so nothing was lost, but the local history you
// restore from skipped that state entirely.
//
// These tests exist because the feature that fixes that had none, and because
// two earlier attempts at it were wrong in ways only a test would catch: one
// snapshotted every game on every launch, and one wrote a baseline from a
// background worker and so could mask a real change instead of recording it.

// waitForSnapshots waits for the count to reach n, so the tests do not race the
// catch-up worker. Returns the count actually reached.
func waitForSnapshots(col *collector, n int, within time.Duration) int {
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if col.snapshotCount() >= n {
			return col.snapshotCount()
		}
		time.Sleep(20 * time.Millisecond)
	}
	return col.snapshotCount()
}

// settle gives the worker time to do nothing, for the tests asserting that
// nothing is what it should do.
func settle() { time.Sleep(1500 * time.Millisecond) }

// The case the feature exists for: the folder moved on while unwatched.
func TestCatchUp_SnapshotsAChangeMadeWhileUnwatched(t *testing.T) {
	dir := t.TempDir()
	save := filepath.Join(dir, "player.sav")
	if err := os.WriteFile(save, []byte("start"), 0o666); err != nil {
		t.Fatal(err)
	}

	col := newCollector()
	eng := New(col.callbacks())
	t.Cleanup(eng.Stop)

	// A baseline has to exist for a catch-up to have anything to compare
	// against, and in life it comes from an ordinary auto-snapshot. Write one
	// directly rather than racing the watcher for it.
	col.mu.Lock()
	col.manifestHashes["catchup"] = "a-hash-that-is-not-the-current-one"
	col.mu.Unlock()

	if err := eng.Watch("catchup", dir); err != nil {
		t.Fatalf("Watch: %v", err)
	}

	if got := waitForSnapshots(col, 1, 30*time.Second); got < 1 {
		t.Fatalf("a folder that changed while unwatched produced %d snapshots, want at least 1", got)
	}
}

// The opposite, and the one that keeps the history clean: nothing changed, so
// nothing should be recorded.
func TestCatchUp_DoesNothingWhenTheFolderIsUnchanged(t *testing.T) {
	dir := t.TempDir()
	save := filepath.Join(dir, "player.sav")
	if err := os.WriteFile(save, []byte("stable"), 0o666); err != nil {
		t.Fatal(err)
	}

	col := newCollector()
	eng := New(col.callbacks())
	t.Cleanup(eng.Stop)

	// Record the baseline the watcher itself would record for this content.
	manifest, _, err := buildForTest(dir)
	if err != nil {
		t.Fatal(err)
	}
	col.mu.Lock()
	col.manifestHashes["stable"] = manifest
	col.mu.Unlock()

	if err := eng.Watch("stable", dir); err != nil {
		t.Fatalf("Watch: %v", err)
	}
	settle()

	if got := col.snapshotCount(); got != 0 {
		t.Errorf("an unchanged folder produced %d snapshots; catching up must be silent when nothing moved", got)
	}
}

// A game with no baseline is left alone. Writing one from the worker looks
// harmless and is not: the write races whatever the user is doing, so a save
// made in that instant would become the baseline and the change it represents
// would never be snapshotted. Suppressing a real snapshot to gain a nominal
// one is the wrong trade here.
func TestCatchUp_LeavesAGameWithNoBaselineAlone(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "player.sav"), []byte("fresh"), 0o666); err != nil {
		t.Fatal(err)
	}

	col := newCollector()
	eng := New(col.callbacks())
	t.Cleanup(eng.Stop)

	if err := eng.Watch("nobaseline", dir); err != nil {
		t.Fatalf("Watch: %v", err)
	}
	settle()

	if got := col.snapshotCount(); got != 0 {
		t.Errorf("a game with no recorded baseline produced %d snapshots on watch; "+
			"that would be one per game per launch", got)
	}
	col.mu.Lock()
	recorded := col.manifestHashes["nobaseline"]
	col.mu.Unlock()
	if recorded != "" {
		t.Errorf("a baseline was written from the catch-up worker (%q) — that can mask a "+
			"concurrent change instead of recording it", recorded)
	}
}

// Re-watching after an Unwatch is the shape of the periodic reconcile
// recovering a watch that failed to start. The change made in between must be
// caught.
func TestCatchUp_RecoversAChangeMissedWhileTheWatchWasDown(t *testing.T) {
	dir := t.TempDir()
	save := filepath.Join(dir, "player.sav")
	if err := os.WriteFile(save, []byte("before"), 0o666); err != nil {
		t.Fatal(err)
	}

	col := newCollector()
	eng := New(col.callbacks())
	t.Cleanup(eng.Stop)

	manifest, _, err := buildForTest(dir)
	if err != nil {
		t.Fatal(err)
	}
	col.mu.Lock()
	col.manifestHashes["recover"] = manifest
	col.mu.Unlock()

	if err := eng.Watch("recover", dir); err != nil {
		t.Fatal(err)
	}
	settle()
	if got := col.snapshotCount(); got != 0 {
		t.Fatalf("setup: expected no snapshot for unchanged content, got %d", got)
	}

	// The watch goes away, the save changes, the watch comes back — exactly
	// what a failed start followed by the reconcile looks like.
	eng.Unwatch("recover")
	if err := os.WriteFile(save, []byte("changed while nobody was looking"), 0o666); err != nil {
		t.Fatal(err)
	}
	if err := eng.Watch("recover", dir); err != nil {
		t.Fatal(err)
	}

	if got := waitForSnapshots(col, 1, 30*time.Second); got < 1 {
		t.Errorf("a change made while the watch was down was never snapshotted (%d snapshots)", got)
	}
}

// Stopping the engine must not leave the catch-up worker running.
func TestCatchUp_WorkerStopsWithTheEngine(t *testing.T) {
	dir := t.TempDir()
	col := newCollector()
	eng := New(col.callbacks())
	if err := eng.Watch("stopping", dir); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() { eng.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("Stop() did not return; the catch-up worker is not being shut down")
	}
}

// buildForTest computes the content hash the watcher would record for a
// folder, so a test can seed a baseline that genuinely matches.
func buildForTest(dir string) (string, map[string]error, error) {
	m, failures, err := delta.BuildMultiManifest(dir, nil)
	if err != nil {
		return "", failures, err
	}
	return m.ContentHash(), failures, nil
}
