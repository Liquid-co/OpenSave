package watcher

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A watch whose picture of the folder has gone stale must fix itself.
//
// Everything the watcher does to correct itself is driven by an event: the
// debounce fires, the folder is re-read, the baseline is updated. That works
// until the event is the thing that went missing — a burst that overran the
// buffer, a folder created during one that ended up watched by nobody, a
// change landing in the gap between the last event and the debounce. Then the
// recorded baseline is wrong and nothing will ever raise another event to
// correct it. Observed sitting diverged for four minutes with no sign of
// recovering; the only reason it was not four hours is that the test gave up.
//
// The baseline decides whether a save holds changes a pull might overwrite, so
// leaving it stale is wrong in the direction that costs someone a save.
//
// The divergence is created directly rather than by racing a real burst. The
// burst is one way in and an unreliable one to reproduce; what matters is that
// a watch in that state recovers, whatever put it there.
func TestReconcile_AStaleBaselineRecoversWithoutAnyEvent(t *testing.T) {
	restore := shortenReconcile(t, 500*time.Millisecond)
	defer restore()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "player.sav"), []byte("original"), 0o666); err != nil {
		t.Fatal(err)
	}

	col := newCollector()
	eng := New(col.callbacks())
	t.Cleanup(eng.Stop)

	// A baseline that matches, so the watch starts settled and the catch-up
	// on Watch has nothing to do.
	manifest, _, err := buildForTest(dir)
	if err != nil {
		t.Fatal(err)
	}
	col.mu.Lock()
	col.manifestHashes["stale"] = manifest
	col.mu.Unlock()

	if err := eng.Watch("stale", dir); err != nil {
		t.Fatal(err)
	}
	settle()
	if got := col.snapshotCount(); got != 0 {
		t.Fatalf("setup: a matching baseline produced %d snapshots", got)
	}

	// Now put the LIVE watch into the state a dropped event leaves it in:
	// the folder has moved on and the recorded baseline still describes the
	// old contents.
	//
	// Set directly rather than by racing a real burst. Going through Unwatch
	// and Watch would prove nothing — Watch runs its own catch-up, so the
	// recovery would be that, not the reconcile. The watch here never stops,
	// and no event is ever raised for the divergence, which is exactly the
	// situation the reconcile exists for.
	col.mu.Lock()
	col.manifestHashes["stale"] = "a-hash-describing-contents-this-folder-no-longer-has"
	col.mu.Unlock()

	if !waitUntil(30*time.Second, func() bool { return finalStateRecorded(t, col, "stale", dir) }) {
		t.Error("a live watch whose baseline had gone stale never corrected itself; " +
			"with no further events in that folder it would stay wrong indefinitely")
	}
	if got := col.snapshotCount(); got == 0 {
		t.Error("the baseline was corrected without a snapshot being taken; the whole point " +
			"is to capture the state that was missed, not just to agree with disk again")
	}
}

// The reconcile must stay silent when nothing has changed, or it becomes the
// periodic disk-reading this codebase spent a release removing.
func TestReconcile_IsSilentWhenNothingChanged(t *testing.T) {
	restore := shortenReconcile(t, 200*time.Millisecond)
	defer restore()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "player.sav"), []byte("stable"), 0o666); err != nil {
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
	col.manifestHashes["quiet"] = manifest
	col.mu.Unlock()

	if err := eng.Watch("quiet", dir); err != nil {
		t.Fatal(err)
	}

	// Many reconcile passes over an untouched folder.
	time.Sleep(3 * time.Second)
	if got := col.snapshotCount(); got != 0 {
		t.Errorf("an untouched folder produced %d snapshots across ~15 reconcile passes; "+
			"this would be a snapshot per game per interval, forever", got)
	}
}

// Stopping the engine must stop the reconcile with it.
func TestReconcile_StopsWithTheEngine(t *testing.T) {
	restore := shortenReconcile(t, 100*time.Millisecond)
	defer restore()

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
		t.Fatal("Stop() did not return; the reconcile worker is not being shut down")
	}
}

// shortenReconcile drives the reconcile faster than five minutes, so a test
// can finish. Restores the real value afterwards.
func shortenReconcile(t *testing.T, d time.Duration) func() {
	t.Helper()
	previous := reconcileInterval
	reconcileInterval = d
	return func() { reconcileInterval = previous }
}
