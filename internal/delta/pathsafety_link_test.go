package delta

import (
	"os"
	"path/filepath"
	"testing"
)

// A path that passes through a link below the save folder is refused, because
// the write, read or deletion would happen wherever the link points. A save
// folder that is itself a link is fine.
func TestIsSafePath_ThroughALink(t *testing.T) {
	root := t.TempDir()
	save := filepath.Join(root, "save")
	outside := filepath.Join(root, "outside")
	for _, d := range []string{filepath.Join(save, "slots"), outside} {
		if err := os.MkdirAll(d, 0o777); err != nil {
			t.Fatal(err)
		}
	}
	makeDirLink(t, filepath.Join(save, "link"), outside)

	for rel, want := range map[string]bool{
		"slot1.sav":           true,
		"slots/slot1.sav":     true,
		"new/folder/slot.sav": true, // not there yet: made as a plain folder
		"link":                false,
		"link/x.sav":          false,
		"link/deeper/x.sav":   false,
		"../outside/x.sav":    false,
		"slots/../link/x.sav": false,
	} {
		if got := IsSafePath(save, filepath.FromSlash(rel)); got != want {
			t.Errorf("IsSafePath(save, %q) = %v, want %v", rel, got, want)
		}
	}

	// The save folder kept behind a link, as on another drive: what is below
	// it is still the save.
	viaLink := filepath.Join(root, "save-on-other-drive")
	makeDirLink(t, viaLink, save)
	if !IsSafePath(viaLink, filepath.FromSlash("slots/slot1.sav")) {
		t.Error("a save folder that is itself a link was refused")
	}
}
