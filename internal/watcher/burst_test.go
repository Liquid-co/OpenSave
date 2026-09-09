package watcher

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/opensave/opensave/internal/delta"
)

// What happens when more changes arrive than the watcher can hold.
//
// fsnotify gives each watched directory a fixed buffer, and on Windows that
// buffer is the only thing between a burst of writes and losing them: when it
// fills, the OS reports an overflow and every event that would have been in it
// is simply gone. Three separate things then have to work — the error has to
// be noticed instead of discarded, the folder tree has to be re-registered
// because a missed Create means a new subfolder is watched by nobody, and the
// folder has to be rescanned because the events that said what changed no
// longer exist.
//
// None of that had a test. The buffer was also 64 KB per directory, which is
// what made a large library expensive enough to notice; the fix that shrank it
// to 8 KB is what makes an overflow plausible enough to be worth handling
// properly.
//
// The assertion in each is the claim that has to hold whether or not an
// overflow occurs: after the dust settles, what the watcher recorded matches
// what is actually on disk. The first test arranges the timing that makes an
// overflow likely — waves of writes landing while the debounced handler is
// mid-rescan and nothing is draining the buffer — and logs whether one was
// seen, so a run that never reached the branch says so.
//
// Be clear about what that does and does not establish. An overflow does occur
// here, but disabling the overflow branch entirely still leaves these tests
// passing, because any later event triggers a full rescan that repairs the
// baseline anyway. So they are a guard on the property, not a proof of the
// branch. What the branch uniquely buys is the case that cannot self-repair:
// the dropped event was a directory Create and nothing further ever happens in
// the parent, so no later event exists to trigger the rescan and the new
// subtree is watched by nobody, forever. Forcing that exact drop on demand is
// not something these tests can do, and pretending otherwise in a comment
// would be worse than saying so.

// logSink captures the watcher's log so a test can tell whether the overflow
// branch actually ran. Without this a burst test passes identically on a
// machine that overflowed and one that never came close, and only one of those
// has tested anything.
type logSink struct {
	mu    sync.Mutex
	lines []string
}

func (l *logSink) record(level, msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, level+": "+msg)
}

func (l *logSink) sawOverflow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, line := range l.lines {
		if strings.Contains(line, "overflowed") {
			return true
		}
	}
	return false
}

// withLog attaches a sink to a collector's callbacks.
func withLog(cb Callbacks, sink *logSink) Callbacks {
	cb.Log = sink.record
	return cb
}

// finalStateRecorded reports whether the baseline the watcher stored matches
// the folder's real contents.
func finalStateRecorded(t *testing.T, col *collector, gameID, dir string) bool {
	t.Helper()
	actual, _, err := buildForTest(dir)
	if err != nil {
		t.Fatalf("could not read the folder to compare against: %v", err)
	}
	col.mu.Lock()
	recorded := col.manifestHashes[gameID]
	col.mu.Unlock()
	return recorded == actual
}

// A burst far larger than one event buffer must still leave the watcher
// holding an accurate picture of the folder.
func TestBurst_ManyFilesAtOnceEndsWithAnAccurateBaseline(t *testing.T) {
	dir := t.TempDir()
	col := newCollector()
	sink := &logSink{}
	eng := New(withLog(col.callbacks(), sink))
	t.Cleanup(eng.Stop)

	if err := os.WriteFile(filepath.Join(dir, "slot0.sav"), []byte("start"), 0o666); err != nil {
		t.Fatal(err)
	}
	manifest, _, err := buildForTest(dir)
	if err != nil {
		t.Fatal(err)
	}
	col.mu.Lock()
	col.manifestHashes["burst"] = manifest
	col.mu.Unlock()

	if err := eng.Watch("burst", dir); err != nil {
		t.Fatal(err)
	}

	// Well past what an 8 KB buffer holds: each event costs roughly a fixed
	// header plus the name, so a few hundred is already the limit and a few
	// thousand is not close.
	//
	// Written in waves with the second and later ones timed to land while the
	// debounced handler is busy reading the folder, because that is when the
	// buffer has nobody draining it and an overflow is actually reachable. A
	// single flat burst is drained fast enough on a quick disk that the branch
	// under test never runs.
	const files = 3000
	for wave := 0; wave < 4; wave++ {
		for i := 0; i < files/4; i++ {
			n := wave*(files/4) + i
			name := filepath.Join(dir, fmt.Sprintf("slot%04d.sav", n))
			if err := os.WriteFile(name, []byte(fmt.Sprintf("save number %d", n)), 0o666); err != nil {
				t.Fatal(err)
			}
		}
		// Just past the debounce, so the next wave arrives mid-rescan.
		time.Sleep(2100 * time.Millisecond)
	}

	t.Logf("event overflow observed: %v", sink.sawOverflow())

	if !waitUntil(60*time.Second, func() bool { return finalStateRecorded(t, col, "burst", dir) }) {
		col.mu.Lock()
		recorded := col.manifestHashes["burst"]
		col.mu.Unlock()
		actual, _, _ := buildForTest(dir)
		t.Errorf("after %d files were written the watcher's baseline is %q but the folder is %q — "+
			"changes were dropped and never recovered", files, recorded, actual)
	}
}

// The specific thing a dropped event costs that a rescan alone cannot repair:
// a new subdirectory. The Create event is what puts a subfolder under watch,
// so losing it means the folder is found once and then never watched — a
// rescan today, silence tomorrow.
func TestBurst_ANewSubfolderStaysWatchedAfterABurst(t *testing.T) {
	dir := t.TempDir()
	col := newCollector()
	eng := New(col.callbacks())
	t.Cleanup(eng.Stop)

	if err := os.WriteFile(filepath.Join(dir, "slot0.sav"), []byte("start"), 0o666); err != nil {
		t.Fatal(err)
	}
	manifest, _, err := buildForTest(dir)
	if err != nil {
		t.Fatal(err)
	}
	col.mu.Lock()
	col.manifestHashes["subdir"] = manifest
	col.mu.Unlock()

	if err := eng.Watch("subdir", dir); err != nil {
		t.Fatal(err)
	}

	// A burst that includes new folders, so the Create events that matter are
	// buried among the ones that do not.
	for i := 0; i < 400; i++ {
		sub := filepath.Join(dir, fmt.Sprintf("profile%03d", i))
		if err := os.MkdirAll(sub, 0o777); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sub, "data.sav"), []byte("initial"), 0o666); err != nil {
			t.Fatal(err)
		}
	}
	if !waitUntil(45*time.Second, func() bool { return finalStateRecorded(t, col, "subdir", dir) }) {
		t.Fatalf("the burst itself was never fully recorded, so what follows would prove nothing")
	}

	// Now the real question: is a folder created during that burst actually
	// being watched? Change a file inside one and see whether it is noticed.
	target := filepath.Join(dir, "profile200", "data.sav")
	if err := os.WriteFile(target, []byte("changed long after the burst"), 0o666); err != nil {
		t.Fatal(err)
	}
	if !waitUntil(45*time.Second, func() bool { return finalStateRecorded(t, col, "subdir", dir) }) {
		t.Error("a change inside a subfolder created during the burst was never picked up; " +
			"that folder is being watched by nobody, and will stay that way")
	}
}

// Deletions in bulk are the dangerous direction: a burst of removals that goes
// unrecorded leaves the watcher believing files exist that do not, which is
// the state that decides what to copy over another device's save.
func TestBurst_BulkDeletionIsRecorded(t *testing.T) {
	dir := t.TempDir()
	col := newCollector()
	eng := New(col.callbacks())
	t.Cleanup(eng.Stop)

	const files = 800
	for i := 0; i < files; i++ {
		name := filepath.Join(dir, fmt.Sprintf("slot%04d.sav", i))
		if err := os.WriteFile(name, []byte(fmt.Sprintf("save %d", i)), 0o666); err != nil {
			t.Fatal(err)
		}
	}
	manifest, _, err := buildForTest(dir)
	if err != nil {
		t.Fatal(err)
	}
	col.mu.Lock()
	col.manifestHashes["bulkdel"] = manifest
	col.mu.Unlock()

	if err := eng.Watch("bulkdel", dir); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < files; i += 2 {
		if err := os.Remove(filepath.Join(dir, fmt.Sprintf("slot%04d.sav", i))); err != nil {
			t.Fatal(err)
		}
	}

	if !waitUntil(45*time.Second, func() bool { return finalStateRecorded(t, col, "bulkdel", dir) }) {
		t.Error("half the folder was deleted and the watcher still records the old contents")
	}
}

// waitUntil polls cond until it holds or the timeout expires.
func waitUntil(within time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return cond()
}

var _ = delta.InvalidateRoot // the burst path depends on it; see run()'s debounce branch
