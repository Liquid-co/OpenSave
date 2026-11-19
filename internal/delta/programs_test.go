package delta

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHoldsPrograms(t *testing.T) {
	saves := t.TempDir()
	os.MkdirAll(filepath.Join(saves, "slot1"), 0o777)
	os.WriteFile(filepath.Join(saves, "slot1", "save.sav"), []byte("x"), 0o666)
	os.WriteFile(filepath.Join(saves, "settings.ini"), []byte("x"), 0o666)
	if HoldsPrograms(saves) {
		t.Error("a folder of saves was said to hold programs")
	}
	if HoldsPrograms(filepath.Join(saves, "not-there")) {
		t.Error("a missing folder was said to hold programs")
	}

	game := t.TempDir()
	os.MkdirAll(filepath.Join(game, "bin"), 0o777)
	os.WriteFile(filepath.Join(game, "bin", "Engine.DLL"), []byte("x"), 0o666)
	if !HoldsPrograms(game) {
		t.Error("a library in a subfolder was missed")
	}
}
