package presets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Custom scan paths — the locations a user adds by hand in Settings.
//
// Reported: "adding new file locations to scan and then hitting auto scan
// wouldn't check the new locations or sublocations." Three separate causes,
// all in one branch of Scan:
//
//   - the location added was never itself a candidate, only its children, so
//     pointing at the folder the saves are actually in found nothing;
//   - children were listed one level deep and offered raw, so a save folder
//     inside a game folder was never reached;
//   - and unlike every other branch of the scanner, the result was not
//     narrowed, so adding a games library proposed whole installs.
//
// Measured on a real library before the fix: adding the Steam common folder
// offered 17 game installs, including two over 100 GB, and neither of the two
// real save folders inside them.

func customMkdirs(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(path, 0o777); err != nil {
		t.Fatal(err)
	}
	return path
}

func customWriteFile(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("save"), 0o666); err != nil {
		t.Fatal(err)
	}
}

// scanCustom runs a real scan and keeps only what was found under path.
//
// The scan also walks this machine's own presets, so the filter has to be
// exact: an earlier version compared a two-character slice against a
// three-character separator string and let every real result through.
func scanCustom(path string) []DiscoveredSave {
	sc := &Scanner{ResolveAppName: func(string) string { return "" }}
	prefix := path + string(filepath.Separator)
	var mine []DiscoveredSave
	for _, d := range sc.Scan([]string{path}) {
		if d.SavePath == path || strings.HasPrefix(d.SavePath, prefix) {
			mine = append(mine, d)
		}
	}
	return mine
}

// Pointing at the folder the saves are in must find it. This found nothing.
func TestCustomScan_ThePathItselfCanBeTheSaveFolder(t *testing.T) {
	root := t.TempDir()
	saves := customMkdirs(t, filepath.Join(root, "MySaves"))
	customWriteFile(t, saves, "player.sav")

	found := scanCustom(saves)
	if len(found) != 1 || found[0].SavePath != saves {
		t.Fatalf("adding the save folder itself discovered %+v, want exactly that folder", found)
	}
}

// A save folder nested inside a game folder must be reached and narrowed to,
// not left pointing at the whole install.
func TestCustomScan_NarrowsToTheSaveFolderInsideAGameFolder(t *testing.T) {
	root := t.TempDir()
	game := customMkdirs(t, filepath.Join(root, "SomeGame"))
	saves := customMkdirs(t, filepath.Join(game, "SaveData"))
	customWriteFile(t, saves, "slot1.sgd")
	// Install noise that must not be offered instead.
	customWriteFile(t, customMkdirs(t, filepath.Join(game, "Binaries")), "game.exe")

	found := scanCustom(root)
	if len(found) != 1 {
		t.Fatalf("want one candidate, got %+v", found)
	}
	if found[0].SavePath != saves {
		t.Errorf("SavePath = %q, want the SaveData folder %q — offering the install "+
			"means syncing the whole game", found[0].SavePath, saves)
	}
}

// Two levels down, which is where a real one lives (GarrysMod: garrysmod/saves).
func TestCustomScan_ReachesASaveFolderTwoLevelsDown(t *testing.T) {
	root := t.TempDir()
	saves := customMkdirs(t, filepath.Join(root, "GarrysModLike", "garrysmod", "saves"))
	customWriteFile(t, saves, "world.dat")

	found := scanCustom(root)
	if len(found) != 1 || found[0].SavePath != saves {
		t.Fatalf("got %+v, want the nested saves folder %q", found, saves)
	}
}

// A folder of game folders must not be offered as a save itself, or adding
// "D:\Games" would propose syncing the games directory.
func TestCustomScan_AFolderOfFoldersIsNotOfferedItself(t *testing.T) {
	root := t.TempDir()
	for _, g := range []string{"GameA", "GameB"} {
		customWriteFile(t, customMkdirs(t, filepath.Join(root, g)), "data.bin")
	}
	for _, d := range scanCustom(root) {
		if d.SavePath == root {
			t.Fatalf("the container itself was offered: %+v", d)
		}
	}
}

// An empty folder is not a save folder.
func TestCustomScan_EmptyPathOffersNothing(t *testing.T) {
	root := customMkdirs(t, filepath.Join(t.TempDir(), "Empty"))
	if found := scanCustom(root); len(found) != 0 {
		t.Errorf("an empty folder produced %+v", found)
	}
}

// When nothing inside looks like a save folder, the game folder is still
// offered — better a container the user can correct than nothing at all.
func TestCustomScan_FallsBackToTheGameFolderWhenNothingLooksLikeASave(t *testing.T) {
	root := t.TempDir()
	game := customMkdirs(t, filepath.Join(root, "OpaqueGame"))
	customWriteFile(t, game, "loose.dat")

	found := scanCustom(root)
	if len(found) != 1 || found[0].SavePath != game {
		t.Fatalf("got %+v, want the game folder %q offered as-is", found, game)
	}
}
