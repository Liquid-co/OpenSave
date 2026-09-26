package presets

import (
	"encoding/base64"
	"path/filepath"
	"strings"
	"testing"
)

const zelda = "0100F2C0115B6000"

func scanRow(t *testing.T, sc *Scanner, idPrefix string) DiscoveredSave {
	t.Helper()
	for _, d := range sc.Scan(nil) {
		if strings.HasPrefix(d.ID, idPrefix) {
			return d
		}
	}
	t.Fatalf("no scan result starting %q", idPrefix)
	return DiscoveredSave{}
}

// A game installed to the emulator's NAND: its name is in
// game_list/<TITLEID>.appname.txt, in the emulator's cache folder — on Linux
// $XDG_CACHE_HOME/<emu>, not beside the saves.
func TestSwitchTitleNamedFromTheEmulatorsListCache(t *testing.T) {
	home := t.TempDir()
	buildNAND(t, filepath.Join(home, ".local", "share", "eden", "nand", "user", "save"), "8f3a1b2c4d5e6f708192a3b4c5d6e7f8", zelda)
	writeFile(t, filepath.Join(home, ".cache", "eden", "game_list", zelda+".appname.txt"), "The Legend of Zelda: Tears of the Kingdom\n")

	d := scanRow(t, linuxScanner(t, home), "eden-")
	if d.Name != "The Legend of Zelda: Tears of the Kingdom" {
		t.Errorf("name = %q", d.Name)
	}
	if d.TitleID != zelda {
		t.Errorf("title id = %q", d.TitleID)
	}
}

// A game in a ROM folder: Citron and Eden keep it in game_metadata_cache.json,
// with the program id written without its leading zero and the icon inside.
// A Flatpak keeps its cache in its own sandbox.
func TestSwitchTitleNamedFromCitronsMetadataCache(t *testing.T) {
	home := t.TempDir()
	app := filepath.Join(home, ".var", "app", "org.citron_emu.citron")
	buildNAND(t, filepath.Join(app, "data", "citron", "nand", "user", "save"), "8f3a1b2c4d5e6f708192a3b4c5d6e7f8", zelda)
	icon := base64.StdEncoding.EncodeToString([]byte("jpeg bytes"))
	writeFile(t, filepath.Join(app, "cache", "citron", "game_list", metadataCacheName),
		`{"entries":[{"key":"k","program_id":"100f2c0115b6000","file_type":3,"title":"Tears of the Kingdom","icon":"`+icon+`"}]}`)

	sc := linuxScanner(t, home)
	d := scanRow(t, sc, "citron-")
	if d.Name != "Tears of the Kingdom" {
		t.Errorf("name = %q", d.Name)
	}
	if got := string(sc.SwitchTitleIcon(zelda, d.SavePath)); got != "jpeg bytes" {
		t.Errorf("icon = %q", got)
	}
}

// A portable copy keeps its cache beside its saves, which only the save
// folder's own path leads to.
func TestSwitchTitleFoundThroughAPortableCopysFolder(t *testing.T) {
	dir := t.TempDir()
	save := filepath.Join(dir, "citron", "user", "nand", "user", "save", "0000000000000000", "8f3a1b2c4d5e6f708192a3b4c5d6e7f8", zelda)
	writeFile(t, filepath.Join(save, "progress.sav"), "x")
	writeFile(t, filepath.Join(dir, "citron", "user", "cache", "game_list", zelda+".appname.txt"), "Tears of the Kingdom")
	writeFile(t, filepath.Join(dir, "citron", "user", "cache", "game_list", zelda+".jpeg"), "icon")

	sc := linuxScanner(t, t.TempDir())
	if got := sc.SwitchTitleName(zelda, save); got != "Tears of the Kingdom" {
		t.Errorf("name = %q", got)
	}
	if got := string(sc.SwitchTitleIcon(zelda, save)); got != "icon" {
		t.Errorf("icon = %q", got)
	}
	if sc.SwitchTitleName(zelda, "") != "" {
		t.Error("found the portable copy's cache without being led to it")
	}
}

// Ryujinx names every game it has run, whichever emulator holds the save.
func TestSwitchTitleNamedFromRyujinx(t *testing.T) {
	home := t.TempDir()
	buildNAND(t, filepath.Join(home, ".local", "share", "eden", "nand", "user", "save"), "8f3a1b2c4d5e6f708192a3b4c5d6e7f8", zelda)
	writeFile(t, filepath.Join(home, ".config", "Ryujinx", "games", strings.ToLower(zelda), "gui", "metadata.json"),
		`{"title":"Tears of the Kingdom","favorite":false,"timespan_played":"00:00:00"}`)

	if d := scanRow(t, linuxScanner(t, home), "eden-"); d.Name != "Tears of the Kingdom" {
		t.Errorf("name = %q", d.Name)
	}
}

// What no emulator here knows keeps the name it had, and a name that says
// nothing (" ", which the emulators write when they could not read one) is
// not taken.
func TestSwitchTitleUnknownKeepsItsName(t *testing.T) {
	home := t.TempDir()
	buildNAND(t, filepath.Join(home, ".local", "share", "eden", "nand", "user", "save"), "8f3a1b2c4d5e6f708192a3b4c5d6e7f8", zelda)
	writeFile(t, filepath.Join(home, ".cache", "eden", "game_list", zelda+".appname.txt"), " ")

	if d := scanRow(t, linuxScanner(t, home), "eden-"); d.Name != "Eden Switch Emulator - Title ID: "+zelda {
		t.Errorf("name = %q", d.Name)
	}
}

// The same game in two emulators is one game — and a Switch game is never
// given the App ID of the Steam game with its name, whose saves are another
// format.
func TestSwitchTitleGroupsByTitleNotByName(t *testing.T) {
	const hollowKnight = "0100633007D48000"
	home := t.TempDir()
	share := filepath.Join(home, ".local", "share")
	buildNAND(t, filepath.Join(share, "eden", "nand", "user", "save"), "8f3a1b2c4d5e6f708192a3b4c5d6e7f8", hollowKnight)
	buildNAND(t, filepath.Join(share, "citron", "nand", "user", "save"), "11111111111111111111111111111111", hollowKnight)
	writeFile(t, filepath.Join(home, ".cache", "eden", "game_list", hollowKnight+".appname.txt"), "Hollow Knight")
	// Each emulator's own name comes first, and they need not agree.
	writeFile(t, filepath.Join(home, ".cache", "citron", "game_list", hollowKnight+".appname.txt"), "Hollow Knight Voidheart Edition")

	found := linuxScanner(t, home).Scan(nil)
	Group(found)
	var rows []DiscoveredSave
	for _, d := range found {
		if d.TitleID == hollowKnight {
			rows = append(rows, d)
		}
	}
	if len(rows) != 2 {
		t.Fatalf("found %d rows for the title, want 2", len(rows))
	}
	if rows[0].GroupID != rows[1].GroupID {
		t.Errorf("the two emulators' copies are in groups %q and %q", rows[0].GroupID, rows[1].GroupID)
	}
	for _, d := range rows {
		if d.AppID != "" {
			t.Errorf("%s was given App ID %s", d.ID, d.AppID)
		}
	}
}

// On Windows an installed emulator keeps its cache inside its own folder in
// %APPDATA% — the setup in the report this was built for, Citron's saves
// found as "Citron Switch Emulator - Title ID: …".
func TestSwitchTitleNamedOnWindows(t *testing.T) {
	appData := t.TempDir()
	t.Setenv("APPDATA", appData)
	writeFile(t, filepath.Join(appData, "citron", "cache", "game_list", zelda+".appname.txt"), "The Legend of Zelda: Tears of the Kingdom")
	writeFile(t, filepath.Join(appData, "citron", "cache", "game_list", zelda+".jpeg"), "icon")

	sc := &Scanner{GOOS: "windows"}
	if got := sc.SwitchTitleName(zelda, ""); got != "The Legend of Zelda: Tears of the Kingdom" {
		t.Errorf("name = %q", got)
	}
	if got := string(sc.SwitchTitleIcon(zelda, "")); got != "icon" {
		t.Errorf("icon = %q", got)
	}
}
