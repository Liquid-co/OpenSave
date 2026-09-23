package presets

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"strings"
)

// Wine/Proton prefixes that don't belong to Steam.
//
// Steam's own prefixes live under <library>/steamapps/compatdata and are
// handled by scanProtonCompat. Everything else — Heroic, Lutris, Bottles, a
// bare ~/.wine — is where non-Steam and cracked games end up on Linux and on
// a Steam Deck, and none of it was being scanned. A game launched through any
// of those had its saves sitting in a prefix OpenSave never looked inside.
//
// Kept bounded for the same reason as the rest of the scanner: only known
// launcher locations are visited, never a blind walk of $HOME.

// maxPrefixesPerLauncher stops a pathological library (hundreds of bottles)
// from dominating a scan.
const maxPrefixesPerLauncher = 60

// winePrefixCandidate is a prefix directory plus the launcher it came from,
// used to label results so the user can tell two copies apart.
type winePrefixCandidate struct {
	path     string
	launcher string
}

// prefixParent is a folder whose children are prefixes, and the launcher
// that keeps them there.
type prefixParent struct {
	dir      string
	launcher string
}

// winePrefixDirs returns the prefix directories for every non-Steam launcher
// present on this machine. Both native and Flatpak install locations are
// checked, since on a Deck these are almost always Flatpaks.
func (sc *Scanner) winePrefixDirs() []winePrefixCandidate {
	home := sc.linuxHome()
	if home == "" {
		return nil
	}

	// Each entry is a directory whose *children* are prefixes.
	prefixParents := []prefixParent{
		// Heroic (Epic/GOG/Amazon, and any manually added game).
		{filepath.Join(home, "Games", "Heroic", "Prefixes"), "Heroic"},
		{filepath.Join(home, "Games", "Heroic", "Prefixes", "default"), "Heroic"},
		{filepath.Join(home, ".var", "app", "com.heroicgameslauncher.hgl", "config", "heroic", "Prefixes"), "Heroic"},
		{filepath.Join(home, ".var", "app", "com.heroicgameslauncher.hgl", "config", "heroic", "Prefixes", "default"), "Heroic"},

		// Bottles.
		{filepath.Join(home, ".var", "app", "com.usebottles.bottles", "data", "bottles", "bottles"), "Bottles"},
		{filepath.Join(home, ".local", "share", "bottles", "bottles"), "Bottles"},

		// Lutris keeps prefixes wherever the install script put them; these
		// are its defaults.
		{filepath.Join(home, "Games"), "Lutris"},
		{filepath.Join(home, ".var", "app", "net.lutris.Lutris", "data", "lutris", "prefixes"), "Lutris"},
		{filepath.Join(home, ".local", "share", "lutris", "prefixes"), "Lutris"},

		// Generic Wine.
		{filepath.Join(home, ".local", "share", "wineprefixes"), "Wine"},
	}

	// The same launcher folders on every other drive. Everything above is
	// under the home folder, and a Steam Deck's games are as often as not on
	// its SD card; a desktop's on a second disk. Heroic in particular asks
	// where to install, and a prefix on /run/media or /var/mnt was never
	// looked at.
	for _, root := range sc.linuxMountRoots() {
		prefixParents = append(prefixParents,
			prefixParent{filepath.Join(root, "Heroic", "Prefixes"), "Heroic"},
			prefixParent{filepath.Join(root, "Heroic", "Prefixes", "default"), "Heroic"},
			prefixParent{filepath.Join(root, "Games", "Heroic", "Prefixes"), "Heroic"},
			prefixParent{filepath.Join(root, "Games", "Heroic", "Prefixes", "default"), "Heroic"},
			prefixParent{filepath.Join(root, "Games"), "Wine"},
		)
	}

	// And wherever Heroic was told to put them, which it writes down. This is
	// the one source that is right by construction rather than by guessing
	// the usual places.
	configured, configuredParents := heroicConfiguredPrefixes(home)
	for _, dir := range configuredParents {
		prefixParents = append(prefixParents, prefixParent{dir, "Heroic"})
	}

	var out []winePrefixCandidate
	listed := map[string]bool{}
	add := func(path, launcher string) bool {
		key := filepath.Clean(path)
		if listed[key] || !isWinePrefix(path) {
			return false
		}
		listed[key] = true
		out = append(out, winePrefixCandidate{path: path, launcher: launcher})
		return true
	}
	for _, parent := range prefixParents {
		if !dirExists(parent.dir) {
			continue
		}
		n := 0
		for _, sub := range listSubdirs(parent.dir) {
			if !add(filepath.Join(parent.dir, sub), parent.launcher) {
				continue
			}
			if n++; n >= maxPrefixesPerLauncher {
				break
			}
		}
	}
	for _, prefix := range configured {
		add(prefix, "Heroic")
	}

	// A bare ~/.wine is itself a prefix, not a parent of prefixes.
	add(filepath.Join(home, ".wine"), "Wine")
	return out
}

// mountRootBases are where Linux desktops and SteamOS mount other drives.
var mountRootBases = []string{"/run/media", "/media", "/mnt", "/var/mnt"}

// maxMountRoots bounds the drive roots one scan visits.
const maxMountRoots = 48

// linuxMountRoots lists the roots of the drives mounted beside the system
// one.
func (sc *Scanner) linuxMountRoots() []string {
	if sc.MountRoots != nil {
		return sc.MountRoots
	}
	if sc.HomeDir != "" {
		// A scanner pointed at a made-up home is a test describing a whole
		// made-up machine. The real machine's drives are not part of it.
		return nil
	}
	return mountRootsUnder(mountRootBases)
}

// mountRootsUnder lists the folders under each base that may be a drive.
//
// /run/media and /media hold a drive either directly — /run/media/mmcblk0p1,
// how older SteamOS mounts the SD card — or under a user's folder —
// /run/media/deck/<label>, how newer SteamOS and most desktops do it — so both
// levels are roots there. /mnt and /var/mnt hold drives directly, and going a
// level deeper would only be walking the drives' own top folders.
func mountRootsUnder(bases []string) []string {
	var out []string
	for _, base := range bases {
		twoLevels := filepath.Base(base) == "media"
		for _, a := range listSubdirs(base) {
			first := filepath.Join(base, a)
			out = append(out, first)
			if len(out) >= maxMountRoots {
				return out
			}
			if !twoLevels {
				continue
			}
			for _, b := range listSubdirs(first) {
				out = append(out, filepath.Join(first, b))
				if len(out) >= maxMountRoots {
					return out
				}
			}
		}
	}
	return out
}

// heroicConfigDirs are where Heroic keeps its settings, native and Flatpak.
func heroicConfigDirs(home string) []string {
	return []string{
		filepath.Join(home, ".config", "heroic"),
		filepath.Join(home, ".var", "app", "com.heroicgameslauncher.hgl", "config", "heroic"),
	}
}

// maxHeroicGameConfigs bounds how many per-game settings files are read.
const maxHeroicGameConfigs = 500

// heroicConfiguredPrefixes reads where Heroic keeps prefixes. prefixes are
// prefixes themselves: each game's own, from GamesConfig/<game>.json. parents
// are folders whose children are prefixes: the defaults from config.json,
// where Heroic makes a new game's prefix.
func heroicConfiguredPrefixes(home string) (prefixes, parents []string) {
	for _, dir := range heroicConfigDirs(home) {
		var cfg struct {
			DefaultSettings struct {
				DefaultWinePrefix string `json:"defaultWinePrefix"`
				WinePrefix        string `json:"winePrefix"`
			} `json:"defaultSettings"`
		}
		if raw, err := os.ReadFile(filepath.Join(dir, "config.json")); err == nil && json.Unmarshal(raw, &cfg) == nil {
			if p := expandHomePath(cfg.DefaultSettings.DefaultWinePrefix, home); p != "" {
				parents = append(parents, p)
			}
			// The shared default prefix is a prefix, and newer Heroic makes
			// per-game prefixes inside it as well.
			if p := expandHomePath(cfg.DefaultSettings.WinePrefix, home); p != "" {
				prefixes = append(prefixes, p)
				parents = append(parents, p)
			}
		}

		entries, err := os.ReadDir(filepath.Join(dir, "GamesConfig"))
		if err != nil {
			continue
		}
		read := 0
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			if read++; read > maxHeroicGameConfigs {
				break
			}
			raw, err := os.ReadFile(filepath.Join(dir, "GamesConfig", e.Name()))
			if err != nil {
				continue
			}
			// { "<appName>": { "winePrefix": "...", ... }, "version": "v0" }
			var byGame map[string]json.RawMessage
			if json.Unmarshal(raw, &byGame) != nil {
				continue
			}
			for _, settings := range byGame {
				var g struct {
					WinePrefix string `json:"winePrefix"`
				}
				if json.Unmarshal(settings, &g) != nil {
					continue
				}
				if p := expandHomePath(g.WinePrefix, home); p != "" {
					prefixes = append(prefixes, p)
				}
			}
		}
	}
	return prefixes, parents
}

// expandHomePath resolves a leading ~ against home, and returns "" for
// anything that is still not an absolute path — a relative path in someone
// else's config has no meaning here.
func expandHomePath(p, home string) string {
	p = strings.TrimSpace(p)
	if p == "~" {
		p = home
	} else if strings.HasPrefix(p, "~/") {
		p = filepath.Join(home, p[2:])
	}
	if !strings.HasPrefix(p, "/") && !filepath.IsAbs(p) {
		return ""
	}
	return filepath.Clean(p)
}

// isWinePrefix reports whether a directory looks like a Wine prefix.
func isWinePrefix(dir string) bool {
	return dirExists(filepath.Join(dir, "drive_c"))
}

// prefixUserDirs returns the per-user home directories inside a prefix.
// Steam's prefixes always use "steamuser", but Heroic, Lutris and Bottles
// name it after the actual account — hardcoding steamuser is exactly why
// those prefixes yielded nothing.
func prefixUserDirs(prefix string) []string {
	usersDir := filepath.Join(prefix, "drive_c", "users")
	var out []string
	for _, user := range listSubdirs(usersDir) {
		if user == "Public" || user == "Default" || user == "Default User" || user == "All Users" {
			continue
		}
		out = append(out, filepath.Join(usersDir, user))
	}
	return out
}

// scanWinePrefixes walks non-Steam launcher prefixes and offers the saves
// inside, using the same conventions and noise filters as the Proton pass.
func (sc *Scanner) scanWinePrefixes(seen map[string]bool) []DiscoveredSave {
	if sc.goos() != "linux" {
		return nil
	}

	var found []DiscoveredSave
	// The same game can have a prefix in two places now that other drives are
	// searched — an old install at home and a new one on the SD card — and the
	// id is built from names alone. The app keys its grid and its selection on
	// the id, so a repeat is told apart by where it lives; a first sighting
	// keeps the id it has always had.
	usedIDs := map[string]bool{}
	for _, candidate := range sc.winePrefixDirs() {
		// The prefix folder name is usually the game name for Heroic and
		// Bottles, which is the best label available here.
		prefixName := filepath.Base(candidate.path)

		perPrefix := 0
		for _, userHome := range prefixUserDirs(candidate.path) {
			for _, root := range protonSaveRoots {
				rootPath := filepath.Join(userHome, root)
				for _, sub := range listSubdirs(rootPath) {
					if protonVendorSkip[toLowerASCII(sub)] || looksLikeHexHash(sub) || isCacheDirName(sub) {
						continue
					}
					savePath := filepath.Join(rootPath, sub)
					abs, err := filepath.Abs(savePath)
					if err != nil || seen[abs] || !dirNonEmpty(abs) {
						continue
					}
					// A precise Ludusavi hit inside this folder wins; the
					// broad parent would just be noise on top of it.
					if seenInside(seen, abs) {
						continue
					}
					seen[abs] = true

					id := "wine-" + sanitizeID(candidate.launcher) + "-" +
						sanitizeID(prefixName) + "-" + sanitizeID(sub)
					if usedIDs[id] {
						id += "-" + shortPathHash(abs)
					}
					usedIDs[id] = true

					found = append(found, DiscoveredSave{
						ID:       id,
						Name:     fmt.Sprintf("%s (%s)", sub, candidate.launcher),
						Type:     "game",
						SavePath: savePath,
					})
					if perPrefix++; perPrefix >= 12 {
						break
					}
				}
				if perPrefix >= 12 {
					break
				}
			}
			if perPrefix >= 12 {
				break
			}
		}
	}
	return found
}

// ensure os is referenced even if the helpers above change shape.
var _ = os.ReadDir

// shortPathHash is a few hex digits that differ between two paths, for telling
// apart ids that would otherwise be the same.
func shortPathHash(p string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(p))
	return fmt.Sprintf("%08x", h.Sum32())
}
