package syncengine

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/opensave/opensave/internal/delta"
	"github.com/opensave/opensave/internal/owntouch"
)

// A pull that finished and whose file the game then saved over is not a
// pull that stopped part-way: resuming it would replace the new save with
// the peer's.
func TestChangedSincePulled(t *testing.T) {
	dir := t.TempDir()
	pulled := filepath.Join(dir, "progress.sav")
	pull := func(content string) {
		owntouch.Mark(pulled)
		if err := os.WriteFile(pulled, []byte(content), 0o666); err != nil {
			t.Fatal(err)
		}
		owntouch.Settled(pulled)
	}
	build := func() delta.Manifest {
		m, err := delta.BuildManifest(dir)
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	d := Decision{FilesToPull: []string{"progress.sav"}}

	pull("from A")
	remote := build()

	// The game saves straight after the pull.
	time.Sleep(20 * time.Millisecond)
	if err := os.WriteFile(pulled, []byte("from B"), 0o666); err != nil {
		t.Fatal(err)
	}
	delta.InvalidateRoot(dir)
	if !changedSincePulled(dir, build(), remote, d) {
		t.Error("a save written over a finished pull was taken for the pull stopping part-way")
	}

	// The pull wrote an old copy and stopped: still as OpenSave left it.
	pull("old")
	delta.InvalidateRoot(dir)
	if changedSincePulled(dir, build(), remote, d) {
		t.Error("a pull that stopped part-way was taken for a change made here")
	}

	// A file the pull did not touch was the same on both sides when it was
	// planned; differing now, it was changed here meanwhile.
	if !changedSincePulled(dir, build(), remote, Decision{}) {
		t.Error("a file the pull did not touch, changed since, was taken for the pull stopping part-way")
	}
}

// A file the pull did not touch, deleted here the moment the pull is done
// (TestFullSyncFlow on CI): a deletion made here, not a part of the pull to
// fetch again.
func TestChangedSincePulled_AnUntouchedFileDeleted(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{"slot1.sav": "from B", "config/video.ini": "fullscreen=1"} {
		full := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o777); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o666); err != nil {
			t.Fatal(err)
		}
	}
	owntouch.Mark(filepath.Join(dir, "slot1.sav"))
	owntouch.Settled(filepath.Join(dir, "slot1.sav"))
	remote, err := delta.BuildManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "config", "video.ini")); err != nil {
		t.Fatal(err)
	}
	delta.InvalidateRoot(dir)
	fresh, err := delta.BuildManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !changedSincePulled(dir, fresh, remote, Decision{FilesToPull: []string{"slot1.sav"}}) {
		t.Error("a file deleted here right after the pull was taken for the pull stopping part-way, and would be fetched back")
	}
}
