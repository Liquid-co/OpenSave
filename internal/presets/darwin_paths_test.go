package presets

import (
	"os"
	"path/filepath"
	"testing"
)

// entryIsSaveEntry decides once, for every platform, whether a manifest entry
// belongs in the index at all — so a Windows-shaped path with no OS
// restriction has to survive that decision (it might run under Wine on
// Linux), and the platform-specific question is left to ludusaviVarSets. This
// pins that ludusaviVarSets does not try to resolve a Windows path against a
// macOS home, whatever the index carries.
func TestDarwinDoesNotResolveWindowsPlaceholders(t *testing.T) {
	sc := &Scanner{GOOS: "darwin", HomeDir: t.TempDir()}
	sets := sc.ludusaviVarSets(indexedGame{Name: "X"}, nil)
	if len(sets) != 1 {
		t.Fatalf("darwin produced %d var sets, want 1", len(sets))
	}
	for key := range sets[0] {
		if key != "<home>" {
			t.Errorf("darwin var set contains %q, which nothing but <home> should reach", key)
		}
	}
}

// A mac-only entry used to be dropped at index build time, before any
// platform-specific resolver ran — 3,181 file templates in the manifest are
// restricted to os: mac, and every one of them was invisible everywhere.
func TestMacOnlyManifestEntriesSurviveIndexing(t *testing.T) {
	var entry manifestFileEntry
	entry.Tags = []string{"save"}
	entry.When = append(entry.When, struct {
		OS    string `yaml:"os"`
		Store string `yaml:"store"`
	}{OS: "mac"})
	if !entryIsSaveEntry("<home>/Library/Application Support/Thing/save.dat", entry) {
		t.Error("a mac-only entry was dropped before any platform resolver could try it")
	}
}

// The manifest's own shape for a macOS save: no dedicated placeholder, just
// <home>/Library/... resolved directly. Proven against a Scanner set to
// darwin, without needing a Mac to run it on.
func TestAMacOnlySaveResolvesOnDarwin(t *testing.T) {
	home := t.TempDir()
	saveDir := filepath.Join(home, "Library", "Application Support", "SomeGame")
	if err := os.MkdirAll(saveDir, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(saveDir, "save.dat"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	sc := manifestScanner(t, `
SomeGame:
  files:
    "<home>/Library/Application Support/SomeGame":
      tags: [save]
      when:
        - os: mac
  steam:
    id: 555555
`)
	sc.GOOS = "darwin"
	sc.HomeDir = home
	sc.SteamRoots = []string{t.TempDir()}
	sc.SteamUserdataPaths = []string{}

	found := sc.Scan(nil)
	hit := false
	for _, f := range found {
		if f.SavePath == saveDir {
			hit = true
		}
	}
	if !hit {
		t.Fatalf("the mac-only save was not found; got %d rows: %+v", len(found), found)
	}
}

// The same manifest entry must resolve on Linux too — <home> is shared, so a
// mac-only entry with no windows/linux restriction is not itself the bug; the
// bug was dropping the OS-restricted ones outright. This is the control case.
func TestALinuxOnlySaveStillResolvesOnLinux(t *testing.T) {
	home := t.TempDir()
	saveDir := filepath.Join(home, ".config", "somegame")
	if err := os.MkdirAll(saveDir, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(saveDir, "save.dat"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	sc := manifestScanner(t, `
SomeGame:
  files:
    "<xdgConfig>/somegame":
      tags: [save]
      when:
        - os: linux
  steam:
    id: 555556
`)
	sc.GOOS = "linux"
	sc.HomeDir = home
	sc.SteamRoots = []string{t.TempDir()}
	sc.SteamUserdataPaths = []string{}

	found := sc.Scan(nil)
	hit := false
	for _, f := range found {
		if f.SavePath == saveDir {
			hit = true
		}
	}
	if !hit {
		t.Fatalf("the linux-only save was not found; got %d rows: %+v", len(found), found)
	}
}
