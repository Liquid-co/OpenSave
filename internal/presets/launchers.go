package presets

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Install-directory discovery for the launchers that are not Steam.
//
// The Ludusavi manifest roots 77,272 of its save paths at <base> — the game's
// own install folder — which is more than every other placeholder put
// together. Resolving <base> therefore decides how much of the manifest is
// usable, and until now it resolved only for games installed through Steam:
// installBaseCandidates read steamapps across the Steam libraries and nothing
// else. A game installed by Epic, GOG, the Xbox app, EA, Ubisoft Connect,
// Battle.net, itch or Amazon had no <base> at all, so none of its
// manifest-declared paths could be tried.
//
// Two ways to find an install folder, in order of trustworthiness:
//
//   - A launcher that records its installs is asked. Epic writes one JSON
//     manifest per game naming the exact InstallLocation, so no guessing is
//     needed and a game installed on any drive is found.
//   - Otherwise the launcher's default parent folder is listed, and each
//     child is a candidate install directory. This is how the Steam path
//     already works (steamapps/common/<name>), and it costs one readdir per
//     launcher that is present.
//
// Neither invents a path: every candidate is a directory that exists on disk
// when it is returned.

// epicManifestDirs returns where the Epic launcher records its installs.
func (sc *Scanner) epicManifestDirs() []string {
	if sc.EpicManifestDirs != nil {
		return sc.EpicManifestDirs
	}
	programData := os.Getenv("PROGRAMDATA")
	if programData == "" {
		return nil
	}
	return []string{filepath.Join(programData, "Epic", "EpicGamesLauncher", "Data", "Manifests")}
}

// epicManifest is the subset of an Epic .item file that matters here.
type epicManifest struct {
	DisplayName     string `json:"DisplayName"`
	InstallLocation string `json:"InstallLocation"`
}

// epicInstalls maps a lowercased lookup name to an install folder, for every
// Epic game whose recorded folder is still on disk.
//
// Both the display name and the folder's own name are registered. Ludusavi's
// installDir entries are folder names ("BioshockRemastered"), while a user
// reading a listing recognises the display name ("BioShock Remastered"), and
// the two are rarely the same string.
//
// Stale manifests are normal — Epic leaves the .item file behind after an
// uninstall, and on the machine this was written against all sixteen pointed
// at folders that no longer existed. Every one is checked rather than trusted.
func (sc *Scanner) epicInstalls() map[string]string {
	out := map[string]string{}
	for _, dir := range sc.epicManifestDirs() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".item") {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				continue
			}
			var m epicManifest
			if json.Unmarshal(raw, &m) != nil || m.InstallLocation == "" {
				continue
			}
			loc := filepath.Clean(m.InstallLocation)
			if !dirExists(loc) {
				continue
			}
			if base := filepath.Base(loc); base != "" {
				out[strings.ToLower(base)] = loc
			}
			if m.DisplayName != "" {
				out[strings.ToLower(m.DisplayName)] = loc
			}
		}
	}
	return out
}

// installParentDirs returns folders whose immediate children are game install
// directories, for the launchers that do not record their installs anywhere
// readable.
//
// Only folders that exist are returned, so a machine without a given launcher
// costs one stat per candidate and nothing more.
func (sc *Scanner) installParentDirs() []string {
	if sc.InstallParentDirs != nil {
		return sc.InstallParentDirs
	}
	var out []string
	add := func(parts ...string) {
		if parts[0] == "" {
			return
		}
		if p := filepath.Join(parts...); dirExists(p) {
			out = append(out, p)
		}
	}

	programFiles := os.Getenv("PROGRAMFILES")
	programFilesX86 := os.Getenv("PROGRAMFILES(X86)")
	home, _ := os.UserHomeDir()
	systemDrive := os.Getenv("SYSTEMDRIVE")
	if systemDrive == "" {
		systemDrive = "C:"
	}

	// Epic's default location, for installs whose manifest has gone.
	add(programFiles, "Epic Games")
	// GOG Galaxy.
	add(programFilesX86, "GOG Galaxy", "Games")
	add(programFiles, "GOG Galaxy", "Games")
	// The Xbox app installs to a drive-root folder by default.
	add(systemDrive + string(filepath.Separator) + "XboxGames")
	// EA — the current app and the Origin layout it replaced.
	add(programFiles, "EA Games")
	add(programFilesX86, "Origin Games")
	// Ubisoft Connect.
	add(programFilesX86, "Ubisoft", "Ubisoft Game Launcher", "games")
	add(programFiles, "Ubisoft", "Ubisoft Game Launcher", "games")
	// Battle.net puts each game directly under Program Files, so the parent
	// is the whole folder — the children are filtered by name against the
	// manifest, so a non-game folder simply never matches.
	add(programFilesX86, "Battle.net")
	// itch.io keeps its installs per user.
	add(home, "AppData", "Roaming", "itch", "apps")
	// Amazon Games.
	add(home, "Games", "Amazon Games", "Library")
	// Riot.
	add(systemDrive + string(filepath.Separator) + "Riot Games")

	return dedupePaths(out)
}

// launcherInstallDirs maps a lowercased folder name to an install directory,
// across every non-Steam launcher found on this machine.
//
// Epic's recorded locations win over a guess from a default folder: they name
// the drive the game is actually on, which a default parent cannot.
func (sc *Scanner) launcherInstallDirs() map[string]string {
	out := map[string]string{}
	for _, parent := range sc.installParentDirs() {
		entries, err := os.ReadDir(parent)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			key := strings.ToLower(e.Name())
			if _, seen := out[key]; !seen {
				out[key] = filepath.Join(parent, e.Name())
			}
		}
	}
	for name, loc := range sc.epicInstalls() {
		out[name] = loc // recorded beats guessed
	}
	return out
}

// knownGameNames returns the normalised names of every game the manifest knows,
// mapped to its Steam AppID where the manifest has one.
//
// Used to tell a publisher folder from a game folder. Both %USERPROFILE%\Saved
// Games and Documents\My Games hold a mix: most children are games, but some
// are the studio ("CD Projekt Red" holding Cyberpunk 2077, "Arkane Studios"
// holding Deathloop, "MachineGames" holding Wolfenstein II). Offering the
// studio as the game names it after the publisher, finds no cover art for it,
// and — where a studio ships more than one title — puts several games in one
// synced unit, so a rollback of any of them rolls back all of them.
//
// The manifest is the arbiter rather than a hand-written list of studios: it
// already knows twenty thousand game names, and a name it does not know is
// exactly the case where descending would be a guess.
func (sc *Scanner) knownGameNames() map[string]string {
	idx := sc.loadManifestIndex()
	out := make(map[string]string, len(idx))
	for _, g := range idx {
		if k := normalizeGameName(g.Name); k != "" {
			if _, seen := out[k]; !seen {
				out[k] = g.SteamID
			}
		}
	}
	return out
}

// resolveWrapperChild decides which folder under a "Saved Games"-style wrapper
// is the game.
//
// It returns the child's name and true only when the folder itself is not a
// game the manifest knows AND exactly one of its subfolders is. Both halves
// matter:
//
//   - A known name is never descended into. "God of War" and "The Last of Us
//     Part I" each hold a single subfolder here, and descending would offer a
//     profile id as the game.
//   - An unknown folder with no known child is left exactly as it was.
//     "ThomasAndFriends" and "TrainSimWorld2EGS" are real saves the manifest
//     has never heard of, and guessing at them would lose them.
//
// Requiring exactly one known child keeps a studio folder holding two games
// from silently resolving to whichever came first — that case wants both rows,
// which is what returning false leaves the caller free to do.
func (sc *Scanner) resolveWrapperChild(dir, name string, known map[string]string) (string, bool) {
	if known == nil {
		return "", false
	}
	if _, isGame := known[normalizeGameName(name)]; isGame {
		return "", false
	}
	var hits []string
	for _, sub := range listSubdirs(filepath.Join(dir, name)) {
		if _, isGame := known[normalizeGameName(sub)]; isGame {
			hits = append(hits, sub)
		}
	}
	if len(hits) == 1 {
		return hits[0], true
	}
	return "", false
}
