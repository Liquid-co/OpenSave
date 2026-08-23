package presets

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// A location can be almost entirely folders. Steam makes a userdata folder per
// owned game, emulators make one per title, and most hold nothing — so the
// walk's budget has to be reachable without a single file being present.
//
// Counting only files left both guards unreachable in such a tree: the clock
// was checked every statClockCheckEvery *files*, and statFileCap counts files
// too. A scan over a large folder-heavy tree ran to completion however long
// that took, which is what "scan is stuck" turned out to be.
func TestMeasureStopsOnDeadlineInAFolderOnlyTree(t *testing.T) {
	root := t.TempDir()
	for a := 0; a < 40; a++ {
		for b := 0; b < 40; b++ {
			if err := os.MkdirAll(filepath.Join(root, "d"+strconv.Itoa(a), "e"+strconv.Itoa(b)), 0o755); err != nil {
				t.Fatal(err)
			}
		}
	}

	// Expired before the walk begins: it must stop at the first check.
	count, _, _, truncated, ok := measureOne(root, time.Now().Add(-time.Hour))
	if !truncated {
		t.Error("the walk ran to completion with an expired deadline — a folder-heavy " +
			"location can hang a scan")
	}
	if !ok {
		t.Error("a truncated measurement should still report ok; hiding it makes a real save look empty")
	}
	if count != 0 {
		t.Errorf("no files exist in this tree, got count=%d", count)
	}
}

// The stop must not cost accuracy on a location that fits inside the budget.
func TestMeasureStillMeasuresASmallTreeExactly(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 5; i++ {
		d := filepath.Join(root, "sub"+strconv.Itoa(i))
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "save.dat"), []byte("0123456789"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	count, bytes, _, truncated, ok := measureOne(root, time.Now().Add(time.Minute))
	if !ok || truncated {
		t.Fatalf("a small tree should measure whole: ok=%v truncated=%v", ok, truncated)
	}
	if count != 5 || bytes != 50 {
		t.Errorf("want 5 files / 50 bytes, got %d / %d", count, bytes)
	}
}
