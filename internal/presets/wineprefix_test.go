package presets

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// mkPrefixSave builds a Wine prefix containing one save folder.
func mkPrefixSave(t *testing.T, prefix, user, root, game string) string {
	t.Helper()
	dir := filepath.Join(prefix, "drive_c", "users", user, root, game)
	mustMkFile(t, filepath.Join(dir, "save.dat"))
	return dir
}

// TestScanWinePrefixes_HeroicLutrisBottles is the reported Steam Deck gap:
// a game installed outside Steam keeps its saves in that launcher's own Wine
// prefix, which the Steam-only compatdata scan never looked inside.
func TestScanWinePrefixes_HeroicLutrisBottles(t *testing.T) {
	home := t.TempDir()

	// Heroic (native layout), prefix named after the game.
	heroic := filepath.Join(home, "Games", "Heroic", "Prefixes", "default", "Cracked Adventure")
	mkPrefixSave(t, heroic, "deck", filepath.Join("AppData", "Roaming"), "CrackedAdventure")

	// Bottles (Flatpak layout), and a prefix user that is NOT "steamuser" —
	// the hardcoded name is why these yielded nothing.
	bottles := filepath.Join(home, ".var", "app", "com.usebottles.bottles",
		"data", "bottles", "bottles", "GameBottle")
	mkPrefixSave(t, bottles, "myuser", filepath.Join("Documents", "My Games"), "BottledGame")

	// Lutris.
	lutris := filepath.Join(home, ".local", "share", "lutris", "prefixes", "SomeGame")
	mkPrefixSave(t, lutris, "deck", "Saved Games", "LutrisGame")

	// A bare ~/.wine is itself a prefix, not a parent of prefixes.
	wine := filepath.Join(home, ".wine")
	mkPrefixSave(t, wine, "deck", filepath.Join("AppData", "Local"), "PlainWineGame")

	sc := &Scanner{GOOS: "linux", HomeDir: home}
	found := sc.scanWinePrefixes(map[string]bool{})

	wanted := map[string]string{
		"CrackedAdventure": "Heroic",
		"BottledGame":      "Bottles",
		"LutrisGame":       "Lutris",
		"PlainWineGame":    "Wine",
	}
	for game, launcher := range wanted {
		var hit *DiscoveredSave
		for i := range found {
			if strings.Contains(found[i].SavePath, game) {
				hit = &found[i]
				break
			}
		}
		if hit == nil {
			t.Errorf("%s (%s) not discovered; got %d results", game, launcher, len(found))
			continue
		}
		if !strings.Contains(hit.Name, launcher) {
			t.Errorf("%s labelled %q, want it to mention %s", game, hit.Name, launcher)
		}
	}
}

// TestScanWinePrefixes_SkipsNoise keeps the new pass from filling the grid
// with Wine plumbing and shader caches.
func TestScanWinePrefixes_SkipsNoise(t *testing.T) {
	home := t.TempDir()
	prefix := filepath.Join(home, ".local", "share", "wineprefixes", "test")

	mkPrefixSave(t, prefix, "deck", filepath.Join("AppData", "Roaming"), "RealGame")
	// Vendor plumbing, a cache, and a hex-hash cache dir — none are saves.
	mkPrefixSave(t, prefix, "deck", filepath.Join("AppData", "Roaming"), "Microsoft")
	mkPrefixSave(t, prefix, "deck", filepath.Join("AppData", "Roaming"), "ShaderCache")
	mkPrefixSave(t, prefix, "deck", filepath.Join("AppData", "Roaming"),
		"00767f4da4e990265f6f7ce9e2273256043161ab200bb1c35d2f2393a05e4c2f")

	found := (&Scanner{GOOS: "linux", HomeDir: home}).scanWinePrefixes(map[string]bool{})

	for _, d := range found {
		for _, bad := range []string{"Microsoft", "ShaderCache", "00767f4d"} {
			if strings.Contains(d.SavePath, bad) {
				t.Errorf("offered noise: %s", d.SavePath)
			}
		}
	}
	if len(found) != 1 {
		t.Errorf("expected only RealGame, got %d: %+v", len(found), found)
	}
}

// TestScanWinePrefixes_RespectsSeen pins that a precise hit from an earlier
// pass isn't shadowed by this broader one.
func TestScanWinePrefixes_RespectsSeen(t *testing.T) {
	home := t.TempDir()
	prefix := filepath.Join(home, ".wine")
	save := mkPrefixSave(t, prefix, "deck", filepath.Join("AppData", "Roaming"), "AlreadyFound")

	abs, err := filepath.Abs(save)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{abs: true}

	if found := (&Scanner{GOOS: "linux", HomeDir: home}).scanWinePrefixes(seen); len(found) != 0 {
		t.Errorf("re-offered an already-discovered path: %+v", found)
	}
}

// TestScanWinePrefixes_NonLinuxNoop keeps Windows scans from paying for this.
func TestScanWinePrefixes_NonLinuxNoop(t *testing.T) {
	home := t.TempDir()
	mkPrefixSave(t, filepath.Join(home, ".wine"), "u", filepath.Join("AppData", "Roaming"), "G")

	if found := (&Scanner{GOOS: "windows", HomeDir: home}).scanWinePrefixes(map[string]bool{}); len(found) != 0 {
		t.Errorf("wine prefix scan should be Linux-only, got %+v", found)
	}
}

// A Steam Deck's games are as often on its SD card as in the home folder, and
// Heroic asks where to install. A prefix there was never looked at: every
// place searched was under home.
func TestScanWinePrefixes_HeroicOnAnSDCard(t *testing.T) {
	home := t.TempDir()
	drives := t.TempDir()
	sd := filepath.Join(drives, "run", "media", "deck", "SDCARD")
	mkPrefixSave(t, filepath.Join(sd, "Heroic", "Prefixes", "default", "Card Game"),
		"deck", filepath.Join("AppData", "Roaming"), "CardGameSaves")

	found := (&Scanner{GOOS: "linux", HomeDir: home, MountRoots: []string{sd}}).scanWinePrefixes(map[string]bool{})
	if len(found) != 1 || !strings.Contains(found[0].SavePath, "CardGameSaves") {
		t.Fatalf("the save in a Heroic prefix on the SD card was not found: %+v", found)
	}
	if !strings.Contains(found[0].Name, "Heroic") {
		t.Errorf("labelled %q, want it to mention Heroic", found[0].Name)
	}
}

// Heroic writes down where each game's prefix is. Reading that finds a prefix
// wherever it was put, where guessing the usual folders cannot.
func TestScanWinePrefixes_HeroicConfiguredPrefixAnywhere(t *testing.T) {
	home := t.TempDir()
	elsewhere := filepath.Join(t.TempDir(), "my stuff", "Odd Place")
	mkPrefixSave(t, elsewhere, "deck", filepath.Join("Documents", "My Games"), "OddGame")

	cfgDir := filepath.Join(home, ".var", "app", "com.heroicgameslauncher.hgl", "config", "heroic", "GamesConfig")
	body := `{"a1b2c3": {"winePrefix": ` + strconv.Quote(elsewhere) + `, "wineVersion": {"name": "GE-Proton"}}, "version": "v0", "explicit": true}`
	writeFile(t, filepath.Join(cfgDir, "a1b2c3.json"), body)

	found := (&Scanner{GOOS: "linux", HomeDir: home}).scanWinePrefixes(map[string]bool{})
	if len(found) != 1 || !strings.Contains(found[0].SavePath, "OddGame") {
		t.Fatalf("the prefix Heroic's config points at was not searched: %+v", found)
	}
}

// config.json names the folder new prefixes go in, often with a ~.
func TestScanWinePrefixes_HeroicDefaultPrefixFolder(t *testing.T) {
	home := t.TempDir()
	mkPrefixSave(t, filepath.Join(home, "Custom", "Prefixes", "Some Game"),
		"deck", filepath.Join("AppData", "Local"), "CustomFolderGame")
	writeFile(t, filepath.Join(home, ".config", "heroic", "config.json"),
		`{"defaultSettings": {"defaultWinePrefix": "~/Custom/Prefixes", "winePrefix": "~/Custom/Prefixes/default"}, "version": "v0"}`)

	found := (&Scanner{GOOS: "linux", HomeDir: home}).scanWinePrefixes(map[string]bool{})
	if len(found) != 1 || !strings.Contains(found[0].SavePath, "CustomFolderGame") {
		t.Fatalf("a prefix in Heroic's configured prefix folder was not found: %+v", found)
	}
}

// The same prefix reached two ways — Heroic's config and the usual folder —
// is one prefix, offered once.
func TestScanWinePrefixes_OnePrefixFoundTwiceIsOfferedOnce(t *testing.T) {
	home := t.TempDir()
	prefix := filepath.Join(home, "Games", "Heroic", "Prefixes", "default", "Twice")
	mkPrefixSave(t, prefix, "deck", filepath.Join("AppData", "Roaming"), "TwiceGame")
	writeFile(t, filepath.Join(home, ".config", "heroic", "GamesConfig", "x.json"),
		`{"x": {"winePrefix": `+strconv.Quote(prefix)+`}}`)

	found := (&Scanner{GOOS: "linux", HomeDir: home}).scanWinePrefixes(map[string]bool{})
	if len(found) != 1 {
		t.Errorf("one prefix reached two ways was offered %d times: %+v", len(found), found)
	}
}

// Two installs of one game — at home and on the SD card — have prefixes with
// the same name. They are two saves, and the app keys its grid on the id, so
// the ids must differ.
func TestScanWinePrefixes_SameGameOnTwoDrivesGetsTwoIDs(t *testing.T) {
	home := t.TempDir()
	sd := filepath.Join(t.TempDir(), "mmcblk0p1")
	mkPrefixSave(t, filepath.Join(home, "Games", "Heroic", "Prefixes", "default", "Same Game"),
		"deck", filepath.Join("AppData", "Roaming"), "SameSaves")
	mkPrefixSave(t, filepath.Join(sd, "Heroic", "Prefixes", "default", "Same Game"),
		"deck", filepath.Join("AppData", "Roaming"), "SameSaves")

	found := (&Scanner{GOOS: "linux", HomeDir: home, MountRoots: []string{sd}}).scanWinePrefixes(map[string]bool{})
	if len(found) != 2 {
		t.Fatalf("want both installs, got %+v", found)
	}
	if found[0].ID == found[1].ID {
		t.Errorf("both installs have the id %q", found[0].ID)
	}
}

// The drives to search: both SD card layouts SteamOS has used, a desktop's
// removable drive, and fixed mounts under /mnt.
func TestMountRootsUnder(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{
		filepath.Join("run", "media", "mmcblk0p1"),
		filepath.Join("run", "media", "deck", "SDCARD"),
		filepath.Join("media", "sam", "USB"),
		filepath.Join("mnt", "games"),
		filepath.Join("mnt", "games", "SteamLibrary"),
	} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o777); err != nil {
			t.Fatal(err)
		}
	}
	got := map[string]bool{}
	for _, r := range mountRootsUnder([]string{
		filepath.Join(root, "run", "media"), filepath.Join(root, "media"), filepath.Join(root, "mnt"),
	}) {
		rel, _ := filepath.Rel(root, r)
		got[filepath.ToSlash(rel)] = true
	}
	for _, want := range []string{"run/media/mmcblk0p1", "run/media/deck/SDCARD", "media/sam/USB", "mnt/games"} {
		if !got[want] {
			t.Errorf("%s is not searched; roots were %v", want, got)
		}
	}
	if got["mnt/games/SteamLibrary"] {
		t.Error("a folder inside a drive under /mnt was treated as a drive of its own")
	}
}
