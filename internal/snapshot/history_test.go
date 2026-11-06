package snapshot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A file that changes with every change of the save is a save; one changed
// once among many is settings; too little history says nothing.
func TestChangesLikeASave(t *testing.T) {
	env := setup(t)
	write := func(name, data string) {
		if err := os.WriteFile(filepath.Join(env.saveDir, name), []byte(data), 0o666); err != nil {
			t.Fatal(err)
		}
	}
	snap := func() {
		if _, err := env.mgr.Create("game1", "", false); err != nil {
			t.Fatal(err)
		}
	}
	write("graphics.cfg", "resolution=1080")
	write("progress.cfg", "level=0")
	write("slot1.sav", "0")
	snap()
	ask := []string{"graphics.cfg", "progress.cfg"}
	for i := 1; i <= 3; i++ {
		write("slot1.sav", strings.Repeat("x", i))
		write("progress.cfg", "level="+strings.Repeat("1", i))
		snap()
	}
	if got := env.mgr.ChangesLikeASave("game1", ask); len(got) != 0 {
		t.Fatalf("3 changes of the save already judged: %v", got)
	}
	for i := 4; i <= 6; i++ {
		write("slot1.sav", strings.Repeat("x", i))
		write("progress.cfg", "level="+strings.Repeat("1", i))
		if i == 5 {
			write("graphics.cfg", "resolution=2160")
		}
		snap()
	}
	got := env.mgr.ChangesLikeASave("game1", ask)
	if _, ok := got["progress.cfg"]; !ok {
		t.Errorf("a file that changed with every save is not judged a save: %v", got)
	}
	if _, ok := got["graphics.cfg"]; ok {
		t.Errorf("a file changed once in six saves is judged a save: %v", got)
	}
}
