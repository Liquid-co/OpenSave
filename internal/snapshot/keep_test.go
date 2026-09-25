package snapshot

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/opensave/opensave/internal/ignore"
)

// RestoreKeeping puts the snapshot's save in place and leaves this device's
// excluded files exactly as they were: in the main folder and in an extra
// location, nested or not, present or absent.
func TestRestoreKeepingLeavesExcludedFilesAsTheyWere(t *testing.T) {
	env := setup(t)
	configDir := filepath.Join(t.TempDir(), "config")
	if err := env.store.AddGameRoot("game1", "config", configDir); err != nil {
		t.Fatal(err)
	}

	// The snapshot, as another device would have made it: its own copies of
	// the excluded files, and one this device does not have.
	writeSave(t, env.saveDir, "slot1.sav", "theirs")
	writeSave(t, env.saveDir, "settings.ini", "their graphics")
	writeSave(t, env.saveDir, "logs/theirs.log", "their log")
	writeSave(t, configDir, "keybinds.cfg", "their keys")
	snap, err := env.mgr.Create("game1", "", false)
	if err != nil {
		t.Fatal(err)
	}

	// This device, before taking it.
	for _, dir := range []string{env.saveDir, configDir} {
		if err := os.RemoveAll(dir); err != nil {
			t.Fatal(err)
		}
	}
	writeSave(t, env.saveDir, "slot1.sav", "mine")
	writeSave(t, env.saveDir, "settings.ini", "my graphics")
	writeSave(t, env.saveDir, "logs/mine.log", "my log")
	writeSave(t, configDir, "keybinds.cfg", "my keys")

	rules := ignore.Parse("settings.ini\nlogs/\nkeybinds.cfg")
	if _, err := env.mgr.RestoreKeeping("game1", snap.ID, rules); err != nil {
		t.Fatal(err)
	}

	read := func(dir, rel string) string {
		raw, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			return "<absent>"
		}
		return string(raw)
	}
	for _, c := range []struct{ dir, rel, want string }{
		{env.saveDir, "slot1.sav", "theirs"},
		{env.saveDir, "settings.ini", "my graphics"},
		{env.saveDir, "logs/mine.log", "my log"},
		{env.saveDir, "logs/theirs.log", "<absent>"},
		{configDir, "keybinds.cfg", "my keys"},
	} {
		if got := read(c.dir, c.rel); got != c.want {
			t.Errorf("%s is %q, want %q", c.rel, got, c.want)
		}
	}
}

// With no rules it is a plain restore.
func TestRestoreKeepingWithoutRulesRestoresEverything(t *testing.T) {
	env := setup(t)
	writeSave(t, env.saveDir, "settings.ini", "theirs")
	snap, err := env.mgr.Create("game1", "", false)
	if err != nil {
		t.Fatal(err)
	}
	writeSave(t, env.saveDir, "settings.ini", "mine")
	if _, err := env.mgr.RestoreKeeping("game1", snap.ID, ignore.Rules{}); err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(filepath.Join(env.saveDir, "settings.ini")); string(raw) != "theirs" {
		t.Errorf("settings.ini is %q, want the snapshot's", raw)
	}
}
