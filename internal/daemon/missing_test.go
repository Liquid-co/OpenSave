package daemon

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// A save path on a drive letter this machine does not have is "not on this
// device", not "folder gone": the UI informs rather than warns about it.
func TestSaveDriveMissing(t *testing.T) {
	existing := t.TempDir()
	if SaveDriveMissing(filepath.Join(existing, "saves")) {
		t.Error("a folder on an existing drive was reported as on a missing drive")
	}
	if SaveDriveMissing("relative/path") {
		t.Error("a path without a volume was reported as on a missing drive")
	}

	if runtime.GOOS != "windows" {
		return
	}
	// Find a drive letter that is not mounted.
	for c := 'Z'; c >= 'D'; c-- {
		root := string(c) + `:\`
		if _, err := os.Stat(root); err == nil {
			continue
		}
		p := string(c) + `:\SteamLibrary\steamapps\common\Game\save`
		if !SaveDriveMissing(p) {
			t.Errorf("%s: drive %c: is absent but was not reported missing", p, c)
		}
		if !SaveFolderMissing(p) {
			t.Errorf("%s: SaveFolderMissing should still report the folder missing", p)
		}
		return
	}
	t.Skip("every drive letter D-Z is mounted")
}
