package presets

import (
	"os"
	"path/filepath"
	"testing"
)

// What a device vouches for, for a game arriving from a peer: the folder the
// catalogue gives that game here, even before it exists — and nothing else.

const verifyManifest = `
Hollow Game:
  files:
    <winAppData>/MoonStudio/HollowGame:
      tags: [save]
      when:
        - os: windows
  steam:
    id: 4242
File Game:
  files:
    <winAppData>/FileStudio/FileGame/*.sav:
      tags: [save]
      when:
        - os: windows
Loose Game:
  files:
    <winDocuments>/*.loose:
      tags: [save]
      when:
        - os: windows
`

func verifyScanner(t *testing.T) (*Scanner, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	appdata := filepath.Join(home, "AppData", "Roaming")
	t.Setenv("APPDATA", appdata)
	sc := manifestScanner(t, verifyManifest)
	sc.SteamRoots = []string{}
	sc.SteamUserdataPaths = []string{}
	return sc, appdata
}

func TestCatalogueSaveLocation_TheGamesOwnFolderEvenBeforeItExists(t *testing.T) {
	sc, appdata := verifyScanner(t)
	folder := filepath.Join(appdata, "MoonStudio", "HollowGame") // not created

	if !sc.CatalogueSaveLocation("Hollow Game", "", folder) {
		t.Error("the game's catalogue folder was not vouched for by name")
	}
	if !sc.CatalogueSaveLocation("Some Other Name", "4242", folder) {
		t.Error("the game's catalogue folder was not vouched for by Steam app ID")
	}
	if !sc.CatalogueSaveLocation("Hollow Game (HollowGame)", "", folder) {
		t.Error("a scan's \"Name (folder)\" name was not recognised")
	}
	// A save file template: the folder the files are in.
	if !sc.CatalogueSaveLocation("File Game", "", filepath.Join(appdata, "FileStudio", "FileGame")) {
		t.Error("the folder of a save-file template was not vouched for")
	}
}

func TestCatalogueSaveLocation_NothingElse(t *testing.T) {
	sc, appdata := verifyScanner(t)
	home := filepath.Dir(filepath.Dir(appdata))

	refused := map[string]struct{ name, appID, path string }{
		"another game's folder":       {"File Game", "", filepath.Join(appdata, "MoonStudio", "HollowGame")},
		"the studio folder above it":  {"Hollow Game", "", filepath.Join(appdata, "MoonStudio")},
		"a sibling of it":             {"Hollow Game", "", filepath.Join(appdata, "MoonStudio", "Other")},
		"a game not in the catalogue": {"No Such Game", "", filepath.Join(appdata, "MoonStudio", "HollowGame")},
		"the wrong app id":            {"Hollow Game", "9999", filepath.Join(appdata, "MoonStudio", "HollowGame")},
		"a whole Documents folder":    {"Loose Game", "", filepath.Join(home, "Documents")},
		"application data itself":     {"Hollow Game", "", appdata},
		"no path":                     {"Hollow Game", "", ""},
	}
	for why, c := range refused {
		if sc.CatalogueSaveLocation(c.name, c.appID, c.path) {
			t.Errorf("%s: %q was vouched for as %q's save folder", why, c.path, c.name)
		}
	}
}

func TestSteamSaveLocation(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Steam", "userdata")
	if err := os.MkdirAll(filepath.Join(root, "123456"), 0o777); err != nil {
		t.Fatal(err)
	}
	sc := &Scanner{GOOS: "windows", SteamUserdataPaths: []string{root}}

	if !sc.SteamSaveLocation("588650", filepath.Join(root, "123456", "588650", "remote")) {
		t.Error("Steam's own save folder for the game was not vouched for")
	}
	for why, c := range map[string]struct{ appID, path string }{
		"another game's":       {"588650", filepath.Join(root, "123456", "570", "remote")},
		"Steam's own settings": {"7", filepath.Join(root, "123456", "7", "remote")},
		"the user folder":      {"588650", filepath.Join(root, "123456")},
		"outside Steam":        {"588650", filepath.Join(t.TempDir(), "123456", "588650")},
		"not an app id":        {"abc", filepath.Join(root, "123456", "abc")},
	} {
		if sc.SteamSaveLocation(c.appID, c.path) {
			t.Errorf("%s folder %q was vouched for", why, c.path)
		}
	}
}
