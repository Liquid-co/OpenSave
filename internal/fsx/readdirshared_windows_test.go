package fsx

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func folderWithSaves(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "slot1")
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.sav"), []byte("x"), 0o666); err != nil {
		t.Fatal(err)
	}
	return dir
}

// A folder being listed can still be removed: what a restore emptying the save
// folder, or a game deleting a folder of its own, does while the watcher looks
// inside it. The same with os.Open is refused, which is why this exists.
func TestReadDirShared_TheFolderCanBeRemovedWhileListed(t *testing.T) {
	dir := folderWithSaves(t)
	f, err := openDirShared(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Errorf("a folder being listed could not be removed: %v", err)
	}
	f.Close()

	dir = folderWithSaves(t)
	plain, err := os.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer plain.Close()
	if err := os.RemoveAll(dir); err == nil {
		t.Log("NOTE: a folder held open by os.Open can now be removed on this Windows build")
	}
}

func TestReadDirShared_ListsAsReadDirDoes(t *testing.T) {
	dir := folderWithSaves(t)
	got, err := ReadDirShared(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range got {
		names = append(names, e.Name())
		if (e.Name() == "sub") != e.IsDir() {
			t.Errorf("%s: IsDir %v", e.Name(), e.IsDir())
		}
	}
	sort.Strings(names)
	if len(names) != 2 || names[0] != "a.sav" || names[1] != "sub" {
		t.Errorf("listed %v, want [a.sav sub]", names)
	}
	if _, err := ReadDirShared(filepath.Join(dir, "missing")); !os.IsNotExist(err) {
		t.Errorf("a missing folder: %v, want not-exist", err)
	}
}
