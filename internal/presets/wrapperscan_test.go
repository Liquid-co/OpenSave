package presets

import (
	"os"
	"path/filepath"
	"testing"
)

// resolveWrapperChild being right is not enough — Scan has to call it. The
// unit tests around it all passed with the wiring disabled, which is exactly
// the shape of bug that ships: a correct helper nothing reaches.
//
// So this drives a whole scan over a Saved Games tree and checks the name that
// comes out the far end.
func TestScanNamesTheGameNotThePublisher(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))

	// A studio folder holding one game, the shape Cyberpunk 2077 arrives in.
	saveDir := filepath.Join(home, "Saved Games", "CD Projekt Red", "Cyberpunk 2077")
	if err := os.MkdirAll(saveDir, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(saveDir, "slot1.sav"), []byte("x"), 0o666); err != nil {
		t.Fatal(err)
	}

	sc := manifestScanner(t, `
Cyberpunk 2077:
  files:
    "<home>/Saved Games/CD Projekt Red/Cyberpunk 2077":
      tags: [save]
      when:
        - os: windows
  steam:
    id: 1091500
`)
	sc.SteamRoots = []string{t.TempDir()}
	sc.SteamUserdataPaths = []string{}

	found := sc.Scan(nil)

	var wrapperRow *DiscoveredSave
	for i := range found {
		if found[i].SavePath == saveDir || found[i].SavePath == filepath.Dir(saveDir) {
			wrapperRow = &found[i]
			break
		}
	}
	if wrapperRow == nil {
		t.Fatalf("the scan found nothing under Saved Games; got %d rows", len(found))
	}
	if wrapperRow.Name == "CD Projekt Red" {
		t.Error("the scan named the row after the studio — resolveWrapperChild is not " +
			"being reached from Scan")
	}
	if wrapperRow.SavePath != saveDir {
		t.Errorf("SavePath = %q, want the game folder %q — syncing the studio folder puts "+
			"every game it publishes in one unit", wrapperRow.SavePath, saveDir)
	}
}

// The other half of the rule, through a real scan: a folder the manifest knows
// is a game must not be descended into, or its profile id becomes the title.
func TestScanDoesNotDescendIntoAKnownGame(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))

	// The layout God of War uses: the game folder holds one profile folder.
	profile := filepath.Join(home, "Saved Games", "God of War", "76561198000000000")
	if err := os.MkdirAll(profile, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profile, "save.sav"), []byte("x"), 0o666); err != nil {
		t.Fatal(err)
	}

	sc := manifestScanner(t, `
God of War:
  files:
    "<home>/Saved Games/God of War":
      tags: [save]
      when:
        - os: windows
  steam:
    id: 1593500
`)
	sc.SteamRoots = []string{t.TempDir()}
	sc.SteamUserdataPaths = []string{}

	for _, d := range sc.Scan(nil) {
		if d.Name == "76561198000000000" {
			t.Errorf("the scan offered a profile id as the game name, at %q", d.SavePath)
		}
	}
}
