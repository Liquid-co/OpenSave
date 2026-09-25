package snapshot

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompareSnapshots(t *testing.T) {
	env := setup(t)
	configDir := filepath.Join(t.TempDir(), "config")
	if err := env.store.AddGameRoot("game1", "config", configDir); err != nil {
		t.Fatal(err)
	}
	writeSave(t, env.saveDir, "slot1.sav", "chapter 1")
	writeSave(t, env.saveDir, "slot2.sav", "an old run")
	writeSave(t, env.saveDir, "profile.dat", "same")
	writeSave(t, configDir, "keys.cfg", "wasd")
	before, err := env.mgr.Create("game1", "", false)
	if err != nil {
		t.Fatal(err)
	}

	writeSave(t, env.saveDir, "slot1.sav", "chapter 2, longer")
	if err := os.Remove(filepath.Join(env.saveDir, "slot2.sav")); err != nil {
		t.Fatal(err)
	}
	writeSave(t, env.saveDir, "slot3.sav", "a new run")
	writeSave(t, configDir, "keys.cfg", "arrows")
	after, err := env.mgr.Create("game1", "", false)
	if err != nil {
		t.Fatal(err)
	}

	c, err := env.mgr.Compare("game1", before.ID, after.ID)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, d := range c.Changes {
		got = append(got, d.Location+":"+d.Path+" "+d.Change)
	}
	want := []string{":slot1.sav changed", ":slot2.sav removed", ":slot3.sav added", "config:keys.cfg changed"}
	if len(got) != len(want) {
		t.Fatalf("changes = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("change %d = %q, want %q (all: %v)", i, got[i], want[i], got)
		}
	}
	if c.Unchanged != 1 {
		t.Errorf("unchanged = %d, want 1 (profile.dat)", c.Unchanged)
	}
	if c.Changes[0].FromSize != int64(len("chapter 1")) || c.Changes[0].ToSize != int64(len("chapter 2, longer")) {
		t.Errorf("sizes = %d → %d", c.Changes[0].FromSize, c.Changes[0].ToSize)
	}

	// The same snapshot against itself: nothing.
	same, _ := env.mgr.Compare("game1", after.ID, after.ID)
	if len(same.Changes) != 0 {
		t.Errorf("a snapshot compared with itself: %v", same.Changes)
	}
	// Another game's snapshot is refused.
	if _, err := env.mgr.Compare("other", before.ID, after.ID); err == nil {
		t.Errorf("comparing another game's snapshots was allowed")
	}
}
