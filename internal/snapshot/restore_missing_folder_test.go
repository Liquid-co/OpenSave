package snapshot

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/opensave/opensave/internal/store"
)

// A game is uninstalled and its save folder goes with it. That is precisely
// when someone reaches for a restore, and it was the case that failed.
//
// With the folder gone there is nothing to stat, and the archive cannot answer
// the question either: a snapshot of a lone save.dat and a snapshot of a folder
// containing only save.dat hold exactly the same entry. Deciding by entry count
// meant a single-file snapshot restored into the PARENT of the save folder —
// where the game never looks — while reporting success.
func TestRestoreIntoAMissingFolderPutsFilesInIt(t *testing.T) {
	env := setup(t)
	writeSave(t, env.saveDir, "save.dat", "precious")
	snap, err := env.mgr.Create("game1", "one file only", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(env.saveDir); err != nil {
		t.Fatal(err)
	}

	if _, err := env.mgr.Restore("game1", snap.ID); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	if raw, err := os.ReadFile(filepath.Join(env.saveDir, "save.dat")); err != nil {
		t.Errorf("the save is not in the save folder after a restore that reported success: %v", err)
	} else if string(raw) != "precious" {
		t.Errorf("save.dat = %q, want %q", raw, "precious")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(env.saveDir), "save.dat")); err == nil {
		t.Error("the save was written into the parent folder, where the game will never find it")
	}
}

// The count must not decide it either way round, so the same case with two
// files is pinned beside it.
func TestRestoreIntoAMissingFolderWorksForSeveralFiles(t *testing.T) {
	env := setup(t)
	writeSave(t, env.saveDir, "a.dat", "one")
	writeSave(t, env.saveDir, "b.dat", "two")
	snap, err := env.mgr.Create("game1", "two files", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(env.saveDir); err != nil {
		t.Fatal(err)
	}
	if _, err := env.mgr.Restore("game1", snap.ID); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.dat", "b.dat"} {
		if _, err := os.Stat(filepath.Join(env.saveDir, name)); err != nil {
			t.Errorf("%s missing after restore: %v", name, err)
		}
	}
}

// The heuristic being replaced existed for a real reason: a tracked location
// can be a single file, which several RPG Maker titles use. That case has to
// keep working when its file is gone too — restoring must recreate the file at
// its own path, not a folder named after it.
func TestRestoreOfASingleFileSaveWhoseFileIsGone(t *testing.T) {
	env := setup(t)
	root := t.TempDir()
	savePath := filepath.Join(root, "Save01.rvdata2")
	if err := os.WriteFile(savePath, []byte("rpg maker save"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := env.store.CreateGame(store.Game{
		ID: "filegame", Name: "File Save Game", SavePath: savePath, MaxSnapshots: 3,
	}); err != nil {
		t.Fatal(err)
	}

	snap, err := env.mgr.Create("filegame", "single file", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(savePath); err != nil {
		t.Fatal(err)
	}

	if _, err := env.mgr.Restore("filegame", snap.ID); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	info, err := os.Stat(savePath)
	if err != nil {
		t.Fatalf("the save file was not restored: %v", err)
	}
	if info.IsDir() {
		t.Fatal("a folder was created where the save FILE should be")
	}
	if raw, _ := os.ReadFile(savePath); string(raw) != "rpg maker save" {
		t.Errorf("restored content = %q", raw)
	}
}
