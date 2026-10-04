package snapshot

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/opensave/opensave/internal/delta"
)

// TestBench_SnapshotReuse times a full snapshot of a real save folder and a
// second one that reuses it. Opt-in: OPENSAVE_BENCH_SAVE=<folder>.
// Reads the folder only; archives go to a temp dir.
func TestBench_SnapshotReuse(t *testing.T) {
	src := os.Getenv("OPENSAVE_BENCH_SAVE")
	if src == "" {
		t.Skip("set OPENSAVE_BENCH_SAVE to run")
	}
	dir := t.TempDir()

	first := filepath.Join(dir, "first.zip")
	start := time.Now()
	skipped, captured, err := zipRootsCapturing(src, nil, first, nil)
	if err != nil {
		t.Fatal(err)
	}
	full := time.Since(start)
	t.Logf("full snapshot skipped %d unreadable files", len(skipped))

	// As in the app: the watcher's manifest build has hashed the save just
	// before a snapshot is taken, so the cache is warm.
	if _, err := delta.BuildManifest(src); err != nil {
		t.Fatal(err)
	}

	zr, err := zip.OpenReader(first)
	if err != nil {
		t.Fatal(err)
	}
	r := &reuseSource{zr: zr, files: map[string]*zip.File{}, hashes: map[string]string{}}
	for _, f := range zr.File {
		r.files[f.Name] = f
	}
	for _, c := range captured {
		r.hashes[c.Path] = c.Hash
	}
	second := filepath.Join(dir, "second.zip")
	start = time.Now()
	if _, _, err := zipRootsCapturing(src, nil, second, r); err != nil {
		t.Fatal(err)
	}
	incr := time.Since(start)
	r.Close()

	fi1, _ := os.Stat(first)
	fi2, _ := os.Stat(second)
	t.Logf("%d files: full snapshot %s (%d MB), reusing snapshot %s (%d MB), %d entries copied",
		len(captured), full.Round(time.Millisecond), fi1.Size()>>20, incr.Round(time.Millisecond), fi2.Size()>>20, lastReused.Load())
}
