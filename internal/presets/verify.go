package presets

import (
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Whether a folder is a known game's save location on this device, without
// the folder having to exist yet.
//
// A game arriving from a peer may be tracked by itself only at a folder this
// device can vouch for as a save (CVE-2026-103398). The background scan vouches
// for folders it has seen; these vouch for the ones it can foresee: where the
// save catalogue says a game keeps its saves here, and Steam's own folder for a
// Steam game. So a new device still fills itself in, saves arriving before the
// game is first played there, while the peer still chooses no folder.

// catalogueWait is how long a check waits for a newer catalogue to be indexed
// before answering from the one it had. It is asked while a peer waits.
const catalogueWait = 2 * time.Second

// CatalogueSaveLocation reports whether path is a save location the catalogue
// gives the game called name, or with Steam app ID appID, on this device:
// exactly the folder a scan would offer for that game once it had saved there.
func (sc *Scanner) CatalogueSaveLocation(name, appID, path string) bool {
	if sc.CacheFile == "" || strings.TrimSpace(path) == "" {
		return false
	}
	games := sc.manifestIndex(catalogueWait)
	if len(games) == 0 {
		return false
	}

	var blocked map[string]bool
	var protonIdx map[string][]string
	if sc.goos() == "windows" {
		wv := windowsPathVars()
		if wv == nil {
			return false
		}
		blocked = blockedRoots(wv)
	} else {
		blocked = linuxBlockedRoots(sc.linuxHome())
		protonIdx = sc.protonPrefixIndex()
	}
	baseDirs := sc.installBaseCandidates()
	target := foldCase(filepath.Clean(path))

	for _, g := range games {
		if !catalogueNames(g, name, appID) {
			continue
		}
		installBases := baseDirs(g.Installs)
		// Lower case on every system, as the blocked roots are keyed and as a
		// scan compares them (expandGamePaths): folded only where the
		// filesystem folds, ~/Documents slipped past on Linux and macOS.
		tooBroad := func(dir string) bool {
			d := strings.ToLower(filepath.Clean(dir))
			if blocked[d] {
				return true
			}
			for _, b := range installBases {
				if d == strings.ToLower(filepath.Clean(b)) {
					return true
				}
			}
			return false
		}
		if tooBroad(target) {
			continue
		}
		// Inside the game's own install folder the program sits beside its
		// saves, so a folder there is vouched for only as the catalogue names
		// it, never through a wildcard: "<base>/**/*.sav" stands for save
		// files, and read as a folder it took in Binaries, Engine, any
		// subfolder at all, for the peer to write into. And a folder holding
		// programs is never a save, whatever the catalogue says.
		if insideAny(installBases, path) {
			if holdsPrograms(path) {
				continue
			}
			for _, vars := range sc.ludusaviVarSets(g, protonIdx) {
				for _, tpl := range g.Paths {
					for _, pattern := range expandTemplate(tpl, vars, installBases, g.SteamID) {
						if !hasWildcard(pattern) && locationMatches(pattern, target) {
							return true
						}
						if looksLikeFile(pattern) && !hasWildcard(filepath.Dir(pattern)) &&
							locationMatches(filepath.Dir(pattern), target) {
							return true
						}
					}
				}
			}
			continue
		}
		for _, vars := range sc.ludusaviVarSets(g, protonIdx) {
			for _, tpl := range g.Paths {
				for _, pattern := range expandTemplate(tpl, vars, installBases, g.SteamID) {
					if !wholeFolderWildcard(pattern) && locationMatches(pattern, target) {
						return true
					}
					// A template naming the save file: its folder is the
					// location, as a scan takes it (statOrGlob), unless that
					// folder is too broad to track.
					if looksLikeFile(pattern) && !wholeFolderWildcard(filepath.Dir(pattern)) &&
						locationMatches(filepath.Dir(pattern), target) {
						return true
					}
				}
			}
		}
	}
	return false
}

// SteamSaveLocation reports whether path is Steam's own save folder for the
// game with appID on this device: userdata/<user>/<appID>, or inside it.
func (sc *Scanner) SteamSaveLocation(appID, path string) bool {
	if !isAppID(appID) || steamUserdataSystemIDs[appID] || strings.TrimSpace(path) == "" {
		return false
	}
	roots := sc.SteamUserdataPaths
	if roots == nil {
		for _, root := range sc.steamRootDirs() {
			roots = append(roots, filepath.Join(root, "userdata"))
		}
	}
	for _, root := range roots {
		if !dirExists(root) || !within(root, path) {
			continue
		}
		rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
		if err != nil {
			continue
		}
		parts := strings.Split(rel, string(filepath.Separator))
		if len(parts) >= 2 && isAppID(parts[0]) && parts[1] == appID {
			return true
		}
	}
	return false
}

// catalogueNames reports whether a catalogue entry is the game a peer named:
// by Steam app ID when it gave one, else by name. A scan names a game with
// several save folders "Name (folder)", so that suffix is allowed for.
func catalogueNames(g indexedGame, name, appID string) bool {
	if appID != "" && g.SteamID != "" {
		return g.SteamID == appID
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	if strings.EqualFold(g.Name, name) {
		return true
	}
	if i := strings.LastIndex(name, " ("); i > 0 && strings.HasSuffix(name, ")") {
		return strings.EqualFold(g.Name, name[:i])
	}
	return false
}

// locationMatches compares a catalogue location (which may hold wildcards)
// with a folder, as this device's filesystem compares names.
func locationMatches(pattern, target string) bool {
	p := foldCase(filepath.Clean(pattern))
	if strings.ContainsAny(p, "*?[") {
		ok, err := filepath.Match(p, target)
		return err == nil && ok
	}
	return p == target
}

// looksLikeFile reports whether a catalogue location names files rather than
// a folder: a wildcard or an extension in its last element.
func looksLikeFile(pattern string) bool {
	base := filepath.Base(pattern)
	return strings.ContainsAny(base, "*?[") || filepath.Ext(base) != ""
}

func foldCase(p string) string {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return strings.ToLower(p)
	}
	return p
}

// wholeFolderWildcard reports whether a catalogue location has a folder named
// by wildcards alone ("*", "**", "?*"): one that stands for any folder at all.
// Such a location vouches for nothing. The catalogue is downloaded, and one
// entry of "<winAppData>/*" would vouch for every folder in application data,
// Microsoft's included, for a peer to write into (CVE-2026-103398). A wildcard
// with a name around it, "JustADream*", still names the game's own folder.
func wholeFolderWildcard(pattern string) bool {
	for _, el := range strings.FieldsFunc(pattern, func(r rune) bool { return r == '/' || r == filepath.Separator }) {
		if strings.Trim(el, "*?") == "" {
			return true
		}
	}
	return false
}

// hasWildcard reports whether a catalogue location holds a wildcard anywhere.
func hasWildcard(pattern string) bool {
	return strings.ContainsAny(pattern, "*?[")
}

// insideAny reports whether p is inside one of the install folders (not the
// folder itself, which tooBroad already refuses).
func insideAny(bases []string, p string) bool {
	for _, b := range bases {
		if within(b, p) {
			return true
		}
	}
	return false
}

// programExts are the files a save folder never needs, and the ones a peer
// writing into a game's folder could make run.
var programExts = map[string]bool{
	".exe": true, ".dll": true, ".so": true, ".dylib": true, ".sys": true,
	".com": true, ".bat": true, ".cmd": true, ".ps1": true, ".msi": true,
	".scr": true, ".asi": true,
}

// holdsPrograms reports whether a folder, or one below it, holds a program or
// library. Only the first few thousand entries are looked at: a save folder is
// small, and one too large to look through is not taken on trust either.
func holdsPrograms(dir string) bool {
	const limit = 5000
	seen, found := 0, false
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if seen++; seen > limit {
			found = true
			return fs.SkipAll
		}
		if !d.IsDir() && programExts[strings.ToLower(filepath.Ext(p))] {
			found = true
			return fs.SkipAll
		}
		return nil
	})
	return found
}
