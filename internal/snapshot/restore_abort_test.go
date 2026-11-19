package snapshot

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// failingReader hands over n bytes and then fails, as a read of a file the
// game is rewriting, or a disk going away, does part-way through.
type failingReader struct {
	r io.Reader
	n int
}

func (f *failingReader) Read(p []byte) (int, error) {
	if f.n <= 0 {
		return 0, errors.New("read error part-way through the file")
	}
	if len(p) > f.n {
		p = p[:f.n]
	}
	n, err := f.r.Read(p)
	f.n -= n
	return n, err
}

// cutShort has every file read into an archive from now on fail after n bytes.
func cutShort(t *testing.T, n int) {
	t.Helper()
	orig := archiveSource
	archiveSource = func(r io.Reader) io.Reader { return &failingReader{r: r, n: n} }
	t.Cleanup(func() { archiveSource = orig })
}

func readSave(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// A file that fails part-way into the archive is not kept cut short: the
// snapshot is refused rather than holding a short file that a restore would
// later put back as the save.
func TestCreate_RefusesAFileCutShort(t *testing.T) {
	env := setup(t)
	writeSave(t, env.saveDir, "slot1.sav", "a whole save file")
	cutShort(t, 4)

	if _, err := env.mgr.Create("game1", "", false); !errors.Is(err, errEntryCutShort) {
		t.Fatalf("Create = %v, want errEntryCutShort", err)
	}
	if snaps, _ := env.store.ListSnapshots("game1", "main"); len(snaps) != 0 {
		t.Errorf("kept %d snapshot(s) holding a file cut short", len(snaps))
	}
}

// A restore whose safety copy of the current save cannot be made changes
// nothing: emptying the save with no copy to go back to is the one outcome a
// restore must never have.
func TestRestore_StopsWhenTheSafetyCopyFails(t *testing.T) {
	env := setup(t)
	writeSave(t, env.saveDir, "slot1.sav", "old progress")
	old, err := env.mgr.Create("game1", "", false)
	if err != nil {
		t.Fatal(err)
	}
	writeSave(t, env.saveDir, "slot1.sav", "current progress")
	cutShort(t, 4)

	if _, err := env.mgr.Restore("game1", old.ID); err == nil {
		t.Fatal("Restore succeeded with no safety copy of the current save")
	}
	if got := readSave(t, filepath.Join(env.saveDir, "slot1.sav")); got != "current progress" {
		t.Errorf("save after refused restore = %q, want it untouched", got)
	}
}

// The safety copy is taken when the save's other locations hold something,
// even with the main folder empty: the restore empties those too.
func TestRestore_SafetyCopyCoversOtherLocations(t *testing.T) {
	env := setup(t)
	mods := filepath.Join(filepath.Dir(env.saveDir), "mods")
	if err := os.MkdirAll(mods, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := env.store.AddGameRoot("game1", "mods", mods); err != nil {
		t.Fatal(err)
	}
	writeSave(t, env.saveDir, "slot1.sav", "progress")
	old, err := env.mgr.Create("game1", "", false)
	if err != nil {
		t.Fatal(err)
	}
	// Main save gone, a mod config written since.
	if err := os.Remove(filepath.Join(env.saveDir, "slot1.sav")); err != nil {
		t.Fatal(err)
	}
	writeSave(t, mods, "config.ini", "hours of tuning")

	before, _ := env.store.ListSnapshots("game1", "main")
	if _, err := env.mgr.Restore("game1", old.ID); err != nil {
		t.Fatal(err)
	}
	after, _ := env.store.ListSnapshots("game1", "main")
	if len(after) != len(before)+1 {
		t.Fatalf("snapshots %d → %d: no safety copy of the mods folder was taken before it was emptied", len(before), len(after))
	}
}

// A save whose main folder is gone altogether still restores, with its other
// locations backed up first.
func TestRestore_MainFolderDeleted(t *testing.T) {
	env := setup(t)
	mods := filepath.Join(filepath.Dir(env.saveDir), "mods")
	if err := os.MkdirAll(mods, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := env.store.AddGameRoot("game1", "mods", mods); err != nil {
		t.Fatal(err)
	}
	writeSave(t, env.saveDir, "slot1.sav", "progress")
	old, err := env.mgr.Create("game1", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(env.saveDir); err != nil {
		t.Fatal(err)
	}
	writeSave(t, mods, "config.ini", "hours of tuning")

	if _, err := env.mgr.Restore("game1", old.ID); err != nil {
		t.Fatalf("restoring over a deleted save folder: %v", err)
	}
	if got := readSave(t, filepath.Join(env.saveDir, "slot1.sav")); got != "progress" {
		t.Errorf("slot1.sav = %q, want the snapshot's", got)
	}
}
