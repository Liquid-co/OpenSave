package snapshot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Emptying a folder is the most destructive thing here, and every restore does
// it first: rollback, restoring one snapshot, switching branches. If the path
// is not a save folder, none of them may proceed.
//
// delta.DangerousSyncRoot has always known which paths those are — its comment
// records a game that really did end up tracked at a profile root — but it was
// only wired into the READING side, in BuildManifest. That left the worst
// possible arrangement: a mis-tracked game fails to sync, and the obvious
// response to "sync is broken" is to roll back or switch branches, which is
// the one action that would have emptied the profile.
// The paths here are dangerous by SHAPE and belong to nobody: no real profile
// is named after this test, so a guard that failed would find nothing to
// delete.
//
// Never the machine's actual home directory. Pointing a folder-emptying
// function at a live profile only stays safe while the guard works, and the
// whole purpose of this test is the case where it does not — an earlier
// version did exactly that, and survived only because RemoveAll happened to
// hit a locked file first. A test for a destructive guard must be harmless
// when the guard is absent.
func TestClearSavePathRefusesAProfileRoot(t *testing.T) {
	for _, p := range []string{
		`C:\Users\opensave-guard-test-nobody`,
		`C:\Users\opensave-guard-test-nobody\Documents`,
		`/home/opensave-guard-test-nobody`,
		`/Users/opensave-guard-test-nobody`,
	} {
		if _, err := os.Stat(p); err == nil {
			t.Fatalf("%q exists on this machine — pick a name that does not, or this test "+
				"could delete something real when the guard is broken", p)
		}
		err := clearSavePathGuarded(p)
		if err == nil {
			t.Errorf("clearSavePathGuarded(%q) returned nil — a restore would empty a profile root", p)
			continue
		}
		for _, want := range []string{"refusing", "save path"} {
			if !strings.Contains(strings.ToLower(err.Error()), want) {
				t.Errorf("%q: the refusal should say what is wrong and what to do; got: %v", p, err)
			}
		}
	}
}

// An empty path is the dangerous case that looks harmless: it is what a
// half-configured game has, and the unguarded version would Stat("") and
// return nil, so a restore would carry on believing it had cleared something.
func TestClearSavePathRefusesAnEmptyPath(t *testing.T) {
	if err := clearSavePathGuarded(""); err == nil {
		t.Error("an empty save path was accepted for clearing")
	}
}

// The ordinary case still works, or restores would stop functioning — which
// would be its own kind of data loss, since a restore that cannot clear would
// unzip on top of whatever is already there.
func TestClearSavePathStillEmptiesARealSaveFolder(t *testing.T) {
	dir := t.TempDir()
	save := filepath.Join(dir, "MyGame", "SaveData")
	if err := os.MkdirAll(filepath.Join(save, "nested"), 0o777); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"slot1.sav", filepath.Join("nested", "slot2.sav")} {
		if err := os.WriteFile(filepath.Join(save, p), []byte("progress"), 0o666); err != nil {
			t.Fatal(err)
		}
	}

	if err := clearSavePathGuarded(save); err != nil {
		t.Fatalf("a genuine save folder was refused: %v", err)
	}
	entries, err := os.ReadDir(save)
	if err != nil {
		t.Fatalf("the save folder itself should survive, only its contents go: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("save folder still holds %d entries", len(entries))
	}
}

// A single-file save is a supported shape, and clearing one means removing
// that file rather than treating its parent as the folder to empty.
func TestClearSavePathHandlesASingleFileSave(t *testing.T) {
	dir := t.TempDir()
	sibling := filepath.Join(dir, "do-not-touch.txt")
	if err := os.WriteFile(sibling, []byte("another game's save"), 0o666); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "game.sav")
	if err := os.WriteFile(target, []byte("progress"), 0o666); err != nil {
		t.Fatal(err)
	}

	if err := clearSavePathGuarded(target); err != nil {
		t.Fatalf("a single-file save was refused: %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Error("the save file was not removed")
	}
	if _, err := os.Stat(sibling); err != nil {
		t.Error("clearing a single-file save removed something beside it")
	}
}
