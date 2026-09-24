package snapshot

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/opensave/opensave/internal/store"
)

// changeList flattens a preview to "kind location/path" lines, sorted, for
// comparison.
func changeList(p RestorePreview) []string {
	var out []string
	for _, c := range p.Changes {
		out = append(out, c.Change+" "+c.Location+"/"+c.Path)
	}
	sort.Strings(out)
	return out
}

func equalLists(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// filesNow reads a folder into path -> contents, for checking what a restore
// actually did against what the preview said it would do.
func filesNow(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		b, _ := os.ReadFile(path)
		out[filepath.ToSlash(rel)] = string(b)
		return nil
	})
	return out
}

// The preview names every kind of change, and a restore then does exactly
// that: what it called changed, restored and removed is what changed, came
// back and went, and nothing else moved.
func TestPreviewRestoreMatchesWhatRestoreDoes(t *testing.T) {
	env := setup(t)
	writeSave(t, env.saveDir, "slot1.sav", "old slot one")
	writeSave(t, env.saveDir, "slot2.sav", "same in both")
	writeSave(t, env.saveDir, "profiles/p1.dat", "profile")
	snap, err := env.mgr.Create("game1", "before", false)
	if err != nil {
		t.Fatal(err)
	}

	// After the snapshot: one file grows, one changes without changing size
	// (so only the checksum can tell), one is untouched, one is new.
	writeSave(t, env.saveDir, "slot1.sav", "new slot one, longer")
	writeSave(t, env.saveDir, "profiles/p1.dat", "PROFILE")
	writeSave(t, env.saveDir, "slot3.sav", "made after the snapshot")

	preview, err := env.mgr.PreviewRestore("game1", snap.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"changed /profiles/p1.dat",
		"changed /slot1.sav",
		"removed /slot3.sav",
	}
	if got := changeList(preview); !equalLists(got, want) {
		t.Errorf("preview = %v, want %v", got, want)
	}
	if preview.Unchanged != 1 {
		t.Errorf("unchanged = %d, want 1 (slot2.sav)", preview.Unchanged)
	}
	for _, c := range preview.Changes {
		if c.Path == "slot1.sav" && (c.CurrentSize != int64(len("new slot one, longer")) || c.SnapshotSize != int64(len("old slot one"))) {
			t.Errorf("sizes for slot1.sav = %d -> %d", c.CurrentSize, c.SnapshotSize)
		}
	}

	// Now do it, and hold the preview to what happened.
	before := filesNow(t, env.saveDir)
	if _, err := env.mgr.Restore("game1", snap.ID); err != nil {
		t.Fatal(err)
	}
	after := filesNow(t, env.saveDir)
	var happened []string
	for path, was := range before {
		now, still := after[path]
		switch {
		case !still:
			happened = append(happened, "removed /"+path)
		case now != was:
			happened = append(happened, "changed /"+path)
		}
	}
	for path := range after {
		if _, had := before[path]; !had {
			happened = append(happened, "restored /"+path)
		}
	}
	sort.Strings(happened)
	if !equalLists(happened, changeList(preview)) {
		t.Errorf("the restore did %v; the preview said %v", happened, changeList(preview))
	}
}

// A file the snapshot has and the save no longer does is one the restore
// brings back.
func TestPreviewRestoreListsFilesThatComeBack(t *testing.T) {
	env := setup(t)
	writeSave(t, env.saveDir, "slot1.sav", "a")
	writeSave(t, env.saveDir, "deleted-since.sav", "b")
	snap, _ := env.mgr.Create("game1", "", false)
	os.Remove(filepath.Join(env.saveDir, "deleted-since.sav"))

	preview, err := env.mgr.PreviewRestore("game1", snap.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := changeList(preview); !equalLists(got, []string{"restored /deleted-since.sav"}) {
		t.Errorf("preview = %v", got)
	}
	if preview.Changes[0].SnapshotSize != 1 || preview.Changes[0].CurrentSize != 0 {
		t.Errorf("sizes = %+v", preview.Changes[0])
	}
}

// Restoring the snapshot that matches the save exactly changes nothing, and
// the preview says so.
func TestPreviewRestoreOfTheCurrentStateIsEmpty(t *testing.T) {
	env := setup(t)
	writeSave(t, env.saveDir, "slot1.sav", "a")
	writeSave(t, env.saveDir, "sub/slot2.sav", "b")
	snap, _ := env.mgr.Create("game1", "", false)
	preview, err := env.mgr.PreviewRestore("game1", snap.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !preview.Identical() || preview.Unchanged != 2 {
		t.Errorf("preview of the current state = %+v, want no changes and 2 unchanged", preview)
	}
}

// Extra save locations are compared as the restore treats them: one with a
// folder here is emptied and refilled like the main one; one without a folder
// here is left out, and named.
func TestPreviewRestoreCoversExtraLocations(t *testing.T) {
	env := setup(t)
	configDir := filepath.Join(t.TempDir(), "config")
	writeSave(t, env.saveDir, "slot1.sav", "a")
	writeSave(t, configDir, "settings.ini", "vsync=1")
	if err := env.store.AddGameRoot("game1", "config", configDir); err != nil {
		t.Fatal(err)
	}
	snap, err := env.mgr.Create("game1", "", false)
	if err != nil {
		t.Fatal(err)
	}
	writeSave(t, configDir, "settings.ini", "vsync=0")
	writeSave(t, configDir, "extra.ini", "new")

	preview, err := env.mgr.PreviewRestore("game1", snap.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"changed config/settings.ini", "removed config/extra.ini"}
	sort.Strings(want)
	if got := changeList(preview); !equalLists(got, want) {
		t.Errorf("preview = %v, want %v", got, want)
	}

	// Without a folder for "config" on this device, the restore skips it, and
	// so does the preview.
	if err := env.store.RemoveGameRoot("game1", "config"); err != nil {
		t.Fatal(err)
	}
	preview, err = env.mgr.PreviewRestore("game1", snap.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Changes) != 0 || len(preview.Unplaced) != 1 || preview.Unplaced[0] != "config" {
		t.Errorf("with the location unplaced: %+v, want no changes and config named as unplaced", preview)
	}
}

// A save that is one file, not a folder, is compared as that file.
func TestPreviewRestoreOfASingleFileSave(t *testing.T) {
	env := setup(t)
	file := filepath.Join(t.TempDir(), "game.sav")
	if err := os.WriteFile(file, []byte("first"), 0o666); err != nil {
		t.Fatal(err)
	}
	if err := env.store.CreateGame(store.Game{ID: "single", Name: "Single", SavePath: file, MaxSnapshots: 5}); err != nil {
		t.Fatal(err)
	}
	snap, err := env.mgr.Create("single", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("later"), 0o666); err != nil {
		t.Fatal(err)
	}
	preview, err := env.mgr.PreviewRestore("single", snap.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := changeList(preview); !equalLists(got, []string{"changed /game.sav"}) {
		t.Errorf("preview = %v", got)
	}
}

func TestPreviewRestoreChecksTheGame(t *testing.T) {
	env := setup(t)
	if err := env.store.CreateGame(store.Game{ID: "game2", Name: "Two", SavePath: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	writeSave(t, env.saveDir, "slot1.sav", "a")
	snap, _ := env.mgr.Create("game1", "", false)
	if _, err := env.mgr.PreviewRestore("game2", snap.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("previewing through the wrong game: err = %v, want not found", err)
	}
	if _, err := env.mgr.PreviewRestore("game1", "snap_nope"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("previewing a snapshot that does not exist: err = %v, want not found", err)
	}
}
