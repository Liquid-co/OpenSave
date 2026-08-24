package presets

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeEpicManifest(t *testing.T, dir, display, install string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(map[string]any{
		"DisplayName": display, "InstallLocation": install,
		"AppName": "x", "LaunchExecutable": "game.exe",
	})
	if err != nil {
		t.Fatal(err)
	}
	name := filepath.Join(dir, display+".item")
	if err := os.WriteFile(name, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

// Epic records the exact folder, on whatever drive the user chose, so a game
// installed outside every default location is still found.
func TestEpicInstallsAreFoundByTheirRecordedLocation(t *testing.T) {
	root := t.TempDir()
	manifests := filepath.Join(root, "Manifests")
	install := filepath.Join(root, "elsewhere", "BioshockRemastered")
	if err := os.MkdirAll(install, 0o755); err != nil {
		t.Fatal(err)
	}
	writeEpicManifest(t, manifests, "BioShock Remastered", install)

	sc := &Scanner{EpicManifestDirs: []string{manifests}, InstallParentDirs: []string{}}
	got := sc.epicInstalls()

	// Both spellings resolve: the manifest's installDir entries are folder
	// names, while a listing shows the display name.
	for _, key := range []string{"bioshockremastered", "bioshock remastered"} {
		if got[key] != install {
			t.Errorf("epicInstalls()[%q] = %q, want %q", key, got[key], install)
		}
	}
}

// Epic leaves the .item file behind after an uninstall. On the machine this
// was written against, all sixteen manifests pointed at folders that no longer
// existed — trusting them would hand every one to the scanner as a real
// install directory.
func TestEpicManifestsPointingNowhereAreIgnored(t *testing.T) {
	root := t.TempDir()
	manifests := filepath.Join(root, "Manifests")
	writeEpicManifest(t, manifests, "Uninstalled Game", filepath.Join(root, "gone", "UninstalledGame"))

	sc := &Scanner{EpicManifestDirs: []string{manifests}, InstallParentDirs: []string{}}
	if got := sc.epicInstalls(); len(got) != 0 {
		t.Errorf("a manifest naming a folder that does not exist was accepted: %v", got)
	}
}

// A launcher that records nothing is discovered by listing its default parent,
// which is how the Steam path already works.
func TestLauncherParentFoldersYieldTheirChildren(t *testing.T) {
	parent := t.TempDir()
	for _, name := range []string{"Hades", "Celeste"} {
		if err := os.MkdirAll(filepath.Join(parent, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// A file, not a directory: not an install.
	if err := os.WriteFile(filepath.Join(parent, "readme.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	sc := &Scanner{InstallParentDirs: []string{parent}, EpicManifestDirs: []string{}}
	got := sc.launcherInstallDirs()

	if got["hades"] != filepath.Join(parent, "Hades") {
		t.Errorf("hades = %q", got["hades"])
	}
	if got["celeste"] != filepath.Join(parent, "Celeste") {
		t.Errorf("celeste = %q", got["celeste"])
	}
	if _, ok := got["readme.txt"]; ok {
		t.Error("a plain file was offered as an install directory")
	}
}

// A recorded location names the drive the game is really on; a default parent
// can only guess. When both describe the same game, the record wins.
func TestARecordedLocationBeatsAGuessedOne(t *testing.T) {
	root := t.TempDir()
	guessed := filepath.Join(root, "Program Files", "Epic Games")
	real := filepath.Join(root, "OtherDrive", "Hades")
	for _, d := range []string{filepath.Join(guessed, "Hades"), real} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	manifests := filepath.Join(root, "Manifests")
	writeEpicManifest(t, manifests, "Hades", real)

	sc := &Scanner{InstallParentDirs: []string{guessed}, EpicManifestDirs: []string{manifests}}
	if got := sc.launcherInstallDirs()["hades"]; got != real {
		t.Errorf("launcherInstallDirs()[hades] = %q, want the recorded %q", got, real)
	}
}

// Nothing installed must not produce candidates: a path that does not exist
// handed to the scanner would be offered to the user as a save location.
func TestNoLaunchersMeansNoCandidates(t *testing.T) {
	sc := &Scanner{
		EpicManifestDirs:  []string{filepath.Join(t.TempDir(), "absent")},
		InstallParentDirs: []string{filepath.Join(t.TempDir(), "absent")},
	}
	if got := sc.launcherInstallDirs(); len(got) != 0 {
		t.Errorf("want no candidates, got %v", got)
	}
}

// End to end: a game installed by Epic, on a drive no default folder would
// suggest, has its <base>-rooted save found. Before launcher discovery the
// same manifest entry produced nothing, because <base> resolved only inside a
// Steam library.
func TestLudusaviResolvesBaseForAnEpicInstall(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))

	// The install sits under neither Steam nor any launcher's default folder.
	install := filepath.Join(t.TempDir(), "SomeOtherDrive", "Samurai Riot")
	saveDir := filepath.Join(install, "Save")
	if err := os.MkdirAll(saveDir, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(saveDir, "slot1.dat"), []byte("x"), 0o666); err != nil {
		t.Fatal(err)
	}

	manifests := filepath.Join(t.TempDir(), "Manifests")
	writeEpicManifest(t, manifests, "Samurai Riot", install)

	sc := manifestScanner(t, `
Samurai Riot:
  files:
    <base>/Save/*.dat:
      tags: [save]
      when:
        - os: windows
  installDir:
    Samurai Riot: {}
`)
	sc.SteamRoots = []string{t.TempDir()} // no Steam library at all
	sc.EpicManifestDirs = []string{manifests}
	sc.InstallParentDirs = []string{}

	found := sc.scanLudusavi(map[string]bool{})
	if len(found) == 0 {
		t.Fatal("an Epic-installed game's <base> save was not found — launcher " +
			"discovery is not reaching installBaseCandidates")
	}
	if found[0].SavePath != saveDir {
		t.Errorf("SavePath = %q, want %q", found[0].SavePath, saveDir)
	}
}

// The same, for a launcher that records nothing and is found by listing its
// default folder.
func TestLudusaviResolvesBaseUnderALauncherDefaultFolder(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))

	parent := filepath.Join(t.TempDir(), "XboxGames")
	saveDir := filepath.Join(parent, "Samurai Riot", "Save")
	if err := os.MkdirAll(saveDir, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(saveDir, "slot1.dat"), []byte("x"), 0o666); err != nil {
		t.Fatal(err)
	}

	sc := manifestScanner(t, `
Samurai Riot:
  files:
    <base>/Save/*.dat:
      tags: [save]
      when:
        - os: windows
  installDir:
    Samurai Riot: {}
`)
	sc.SteamRoots = []string{t.TempDir()}
	sc.EpicManifestDirs = []string{}
	sc.InstallParentDirs = []string{parent}

	found := sc.scanLudusavi(map[string]bool{})
	if len(found) == 0 {
		t.Fatal("a game under a launcher's default folder had no <base> resolved")
	}
	if found[0].SavePath != saveDir {
		t.Errorf("SavePath = %q, want %q", found[0].SavePath, saveDir)
	}
}
