package snapshot

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// holdOpen has the named file of the save fail to open while archiving, as a
// file the game holds open does.
func holdOpen(t *testing.T, name string) {
	t.Helper()
	open := openForArchive
	openForArchive = func(p string) (*os.File, error) {
		if filepath.Base(p) == name {
			return nil, errors.New("the process cannot access the file because it is being used by another process")
		}
		return open(p)
	}
	t.Cleanup(func() { openForArchive = open })
}

// An automatic snapshot that could not read every file is discarded: it would
// stand in the history as a save that never existed.
func TestCreateAutomatic_DiscardsAnIncompleteSnapshot(t *testing.T) {
	env := setup(t)
	writeSave(t, env.saveDir, "slot1.sav", "progress")
	writeSave(t, env.saveDir, "game.log", "running")
	holdOpen(t, "game.log")

	_, err := env.mgr.CreateAutomatic("game1")
	var inc *IncompleteError
	if !errors.As(err, &inc) {
		t.Fatalf("CreateAutomatic = %v, want an IncompleteError", err)
	}
	if len(inc.Skipped) != 1 || filepath.Base(inc.Skipped[0]) != "game.log" {
		t.Errorf("skipped %v, want game.log", inc.Skipped)
	}
	if snaps, _ := env.store.ListSnapshots("game1", "main"); len(snaps) != 0 {
		t.Errorf("%d snapshot(s) kept, want none", len(snaps))
	}
}

// Every other snapshot is kept with what could be read, as before: above all
// the copy kept before a sync replaces the save. Refusing it would stop the
// sync replacing files for as long as the game holds one open.
func TestOtherSnapshotsAreKeptWhenAFileCannotBeRead(t *testing.T) {
	for name, create := range map[string]func(m *Manager) error{
		"before replacing": func(m *Manager) error {
			_, err := m.CreateBeforeReplacing("game1", "before sync replaced local files")
			return err
		},
		"by hand": func(m *Manager) error { _, err := m.Create("game1", "mine", false); return err },
	} {
		t.Run(name, func(t *testing.T) {
			env := setup(t)
			writeSave(t, env.saveDir, "slot1.sav", "progress")
			writeSave(t, env.saveDir, "game.log", "running")
			holdOpen(t, "game.log")
			if err := create(env.mgr); err != nil {
				t.Fatalf("snapshot failed: %v", err)
			}
			if snaps, _ := env.store.ListSnapshots("game1", "main"); len(snaps) != 1 {
				t.Errorf("%d snapshot(s) kept, want 1", len(snaps))
			}
		})
	}
}
