package presets

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestGameDB_FindsGamesByNameFolderAndProgram(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("templates below use Windows placeholders")
	}
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	local := filepath.Join(home, "AppData", "Local")
	t.Setenv("LOCALAPPDATA", local)
	save := filepath.Join(local, "Pearl", "CD", "save")
	if err := os.MkdirAll(filepath.Join(save, "123"), 0o777); err != nil {
		t.Fatal(err)
	}

	sc := manifestScanner(t, `
Pearl Game:
  files:
    "<winLocalAppData>/Pearl/CD/save":
      tags: [save]
  installDir:
    Pearl Game: {}
  steam:
    id: 3321460
Project Things:
  files:
    "<winLocalAppData>/Things/Saves":
      tags: [save]
  installDir:
    ProjectThings: {}
`)
	ds := sc.DeviceSettings()
	if ds == nil {
		t.Fatal("no index")
	}

	got := ds.Search("pearl", 5)
	if len(got) != 1 || got[0].AppID != "3321460" || len(got[0].SavePaths) != 1 || got[0].SavePaths[0] != save {
		t.Errorf("search = %+v", got)
	}

	for _, folder := range []string{save, filepath.Join(save, "123"), filepath.Join(local, "Pearl", "CD")} {
		if m, ok := ds.IdentifyFolder(folder); !ok || m.Name != "Pearl Game" {
			t.Errorf("folder %s: %+v %v", folder, m, ok)
		}
	}
	if _, ok := ds.IdentifyFolder(filepath.Join(home, "Elsewhere")); ok {
		t.Error("an unrelated folder was taken for a game")
	}

	m, ok := ds.IdentifyProgram(`G:\Games\Pearl Game\bin64\game.exe`)
	if !ok || m.Name != "Pearl Game" || m.InstallDir != `G:\Games\Pearl Game` {
		t.Errorf("program in a folder of its own = %+v %v", m, ok)
	}
	if m, ok := ds.IdentifyProgram(`D:\Stuff\Project Things\run.exe`); !ok || m.Name != "Project Things" {
		t.Errorf("install folder written with a space the database leaves out = %+v %v", m, ok)
	}
	if _, ok := ds.IdentifyProgram(`C:\Tools\editor.exe`); ok {
		t.Error("an unrelated program was taken for a game")
	}
}

// Naming a game nobody asked about: a folder one level above its save
// location counts, a folder holding many games' does not.
func TestGameDB_IdentifyFolderStrictly(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("templates below use Windows placeholders")
	}
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	low := filepath.Join(home, "AppData", "LocalLow")
	if err := os.MkdirAll(filepath.Join(low, "Soda", "Duck", "Saves"), 0o777); err != nil {
		t.Fatal(err)
	}
	sc := manifestScanner(t, `
Duck Game:
  files:
    "<home>/AppData/LocalLow/Soda/Duck/Saves/*.sav":
      tags: [save]
  steam:
    id: 3167020
`)
	ds := sc.DeviceSettings()
	if m, ok := ds.IdentifyFolderStrictly(filepath.Join(low, "Soda", "Duck")); !ok || m.AppID != "3167020" {
		t.Errorf("the folder above the save location: %+v %v", m, ok)
	}
	if _, ok := ds.IdentifyFolderStrictly(low); ok {
		t.Error("a folder holding many games' saves was named after one")
	}
	if _, ok := ds.IdentifyFolder(low); !ok {
		t.Error("asked about, a folder holding the save location further down is not recognised")
	}
}
