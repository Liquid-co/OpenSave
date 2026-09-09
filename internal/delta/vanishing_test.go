package delta

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// A save folder being used is a moving target, and the manifest has to cope.
//
// Games delete save slots and write-then-rename temp files while they run, and
// a walk of a folder someone is playing in races that by definition. When a
// file disappeared between the directory listing and the read, the whole
// manifest build failed — and a failed build means the sync does nothing at
// all, so a deletion made anywhere else in that folder never reached the other
// device and nothing reported a problem.
//
// That is the shape of three long-standing intermittent deletion failures:
// nothing is broken about deletion itself, the manifest that would have
// carried it just never got built.
func TestBuildManifest_SurvivesFilesVanishingDuringTheWalk(t *testing.T) {
	dir := t.TempDir()
	const files = 400
	for i := 0; i < files; i++ {
		name := filepath.Join(dir, fmt.Sprintf("slot%04d.sav", i))
		if err := os.WriteFile(name, []byte(fmt.Sprintf("save %d", i)), 0o666); err != nil {
			t.Fatal(err)
		}
	}

	// Delete a scattered half of them while the walk is in progress. Which
	// ones land inside the race is up to the scheduler; with this many, some
	// will.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < files; i += 2 {
			_ = os.Remove(filepath.Join(dir, fmt.Sprintf("slot%04d.sav", i)))
		}
	}()

	ClearHashCache()
	m, err := BuildManifest(dir)
	wg.Wait()

	if err != nil {
		t.Fatalf("a file disappearing mid-walk failed the whole manifest: %v — every sync of "+
			"this game does nothing until the folder stops changing, so any deletion in it "+
			"never reaches the other device", err)
	}
	// Whatever survived must be described correctly; the point is that the
	// build completes, not that it catches a particular instant.
	for rel := range m.Files {
		if _, statErr := os.Stat(filepath.Join(dir, rel)); statErr != nil {
			t.Errorf("manifest lists %q, which is not on disk: %v", rel, statErr)
		}
	}
	if len(m.Files) == 0 {
		t.Error("the manifest came back empty; the walk did not survive far enough to prove anything")
	}
	t.Logf("manifest completed with %d of %d files while half were being deleted", len(m.Files), files)
}

// The other half of the same rule, and the one that matters more.
//
// A file that EXISTS but cannot be read must NOT be quietly dropped. Omitting
// it describes the folder as no longer containing it, that description reaches
// the peer as a deletion, and the peer deletes its own copy. A delayed sync is
// the acceptable outcome here; a deleted save is not.
func TestBuildManifest_DoesNotTreatAnUnreadableFileAsDeleted(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "readable.sav"), []byte("fine"), 0o666); err != nil {
		t.Fatal(err)
	}
	locked := filepath.Join(dir, "locked.sav")
	if err := os.WriteFile(locked, []byte("held by the game"), 0o666); err != nil {
		t.Fatal(err)
	}

	restore, ok := makeUnreadable(t, locked)
	if !ok {
		t.Skip("cannot make a file unreadable on this system, so there is nothing to assert")
	}
	defer restore()

	ClearHashCache()
	m, err := BuildManifest(dir)
	if err != nil {
		return // Refusing to build is the safe outcome, and is what happens today.
	}
	if _, listed := m.Files["locked.sav"]; !listed {
		t.Error("a file that exists but could not be read was silently left out of the " +
			"manifest — the peer would read that as a deletion and remove its own copy")
	}
}
