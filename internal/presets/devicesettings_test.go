package presets

import (
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

// Crimson Desert's shape: the save is a per-account folder inside the tracked
// one, and the graphics settings sit beside it. The settings are named; the
// save is not; an entry tagged both counts as save.
func TestDeviceSettings_SettingsInsideTheSaveFolder(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("templates below use Windows placeholders")
	}
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	local := filepath.Join(home, "AppData", "Local")
	t.Setenv("LOCALAPPDATA", local)

	sc := manifestScanner(t, `
Pearl Game:
  files:
    "<winLocalAppData>/Pearl/CD/save/<storeUserId>":
      tags: [save]
    "<winLocalAppData>/Pearl/CD/save/user_engine_option_save.xml":
      tags: [config]
    "<winLocalAppData>/Pearl/CD/save/both.cfg":
      tags: [config, save]
    "<winLocalAppData>/Pearl/CD/save/*":
      tags: [config]
    "<winLocalAppData>/Pearl/Elsewhere/settings.ini":
      tags: [config]
  steam:
    id: 123
`)
	ds := sc.DeviceSettings()
	if ds == nil {
		t.Fatal("no index")
	}
	root := filepath.Join(local, "Pearl", "CD", "save")
	want := []string{"/user_engine_option_save.xml"}
	if got := ds.Patterns("Pearl Game", "123", []string{root}); !reflect.DeepEqual(got, want) {
		t.Errorf("by App ID: %v, want %v", got, want)
	}
	if got := ds.Patterns("Pearl Game", "", []string{root}); !reflect.DeepEqual(got, want) {
		t.Errorf("by name: %v, want %v", got, want)
	}
	// Tracked at the per-account folder: the settings are not inside it.
	if got := ds.Patterns("Pearl Game", "123", []string{filepath.Join(root, "22202")}); len(got) != 0 {
		t.Errorf("at the account folder: %v, want none", got)
	}
	if got := ds.Patterns("Another Game", "999", []string{root}); len(got) != 0 {
		t.Errorf("unknown game: %v", got)
	}
}

func TestRelativeUnder(t *testing.T) {
	for _, c := range []struct {
		root, pattern, want string
		ok                  bool
	}{
		{`C:\Users\A\Saved Games\Game`, `C:\Users\A\Saved Games\Game\game.cfg`, "game.cfg", true},
		{`C:\Users\A/Saved Games/Game`, `c:\users\a\saved games\game\Config\x.ini`, "config/x.ini", true},
		{`C:\Users\A\Game\save\22202`, `C:\Users\A\Game\save\*\opt.xml`, "opt.xml", true},
		{`C:\Users\A\Game\save`, `C:\Users\A\*\save\opt.xml`, "opt.xml", true},
		{`C:\Users\A\Game`, `C:\Users\A\Game`, "", false},
		{`C:\Users\A\Game`, `C:\Users\A\Other\x.ini`, "", false},
	} {
		got, ok := relativeUnder(c.root, c.pattern)
		if got != c.want || ok != c.ok {
			t.Errorf("relativeUnder(%q, %q) = %q, %v; want %q, %v", c.root, c.pattern, got, ok, c.want, c.ok)
		}
	}
	for rel, want := range map[string]bool{"*": true, "*.*": true, "**/x": true, "*.cfg": false, "config": false} {
		if coversEverything(rel) != want {
			t.Errorf("coversEverything(%q) = %v", rel, !want)
		}
	}
}
