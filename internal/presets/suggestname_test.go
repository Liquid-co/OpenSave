package presets

import (
	"path/filepath"
	"testing"
)

func TestSuggestName(t *testing.T) {
	sc := &Scanner{GOOS: "linux", HomeDir: t.TempDir()}
	for path, want := range map[string]string{
		`C:\Users\a\AppData\LocalLow\Team Cherry\Hollow Knight`:                                                          "Hollow Knight",
		`C:\Users\a\Documents\My Games\Skyrim Special Edition\Saves`:                                                     "Skyrim Special Edition",
		`D:\Games\Black Myth - Wukong\b1\Saved\SaveGames`:                                                                "Black Myth - Wukong",
		`C:\Users\a\AppData\Roaming\Balatro\1\save.jkr`:                                                                  "Balatro",
		"/home/deck/.local/share/Celeste/Saves":                                                                          "Celeste",
		`C:\Program Files (x86)\Steam\userdata\12345678\1145360\remote`:                                                  "Hades",
		"/home/deck/.local/share/eden/nand/user/save/0000000000000000/8f3a1b2c4d5e6f708192a3b4c5d6e7f8/0100ABCDEF012000": "Switch game - Title ID: 0100ABCDEF012000",
		`C:\`: "",
	} {
		if got := sc.SuggestName(path); got != want {
			t.Errorf("SuggestName(%q) = %q, want %q", path, got, want)
		}
	}
}

// A Switch save is named as its emulator knows it.
func TestSuggestNameForASwitchSave(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".local", "share", "eden", "nand", "user", "save")
	buildNAND(t, root, "8f3a1b2c4d5e6f708192a3b4c5d6e7f8", zelda)
	writeFile(t, filepath.Join(home, ".cache", "eden", "game_list", zelda+".appname.txt"), "Tears of the Kingdom")

	save := filepath.Join(root, "0000000000000000", "8f3a1b2c4d5e6f708192a3b4c5d6e7f8", zelda)
	if got := linuxScanner(t, home).SuggestName(save); got != "Tears of the Kingdom" {
		t.Errorf("got %q", got)
	}
}
