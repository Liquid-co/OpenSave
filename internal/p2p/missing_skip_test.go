package p2p

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveFolderAbsent(t *testing.T) {
	dir := t.TempDir()
	if saveFolderAbsent(dir) {
		t.Error("an existing folder was reported absent")
	}
	if !saveFolderAbsent(filepath.Join(dir, "gone")) {
		t.Error("a missing folder was not reported absent")
	}
	// A single-file save not written yet: its folder is what counts.
	file := filepath.Join(dir, "save.sav")
	if err := os.WriteFile(file, []byte("x"), 0o666); err != nil {
		t.Fatal(err)
	}
	if saveFolderAbsent(file) {
		t.Error("an existing single-file save was reported absent")
	}
	if saveFolderAbsent("") {
		t.Error("an empty path was reported absent")
	}
}
