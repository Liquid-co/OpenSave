package presets

import (
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// Finding a game in the game database by what is on this device: a program
// or a folder that was picked, or a name typed.
// What it answers is everything tracking needs and a folder picked by hand
// lacked: the game's name, its Steam App ID (cover art, launching, sessions),
// and where its saves are on this device.

// GameDB is the game database loaded once for any number of lookups. Nil
// when there is no index; every method answers nothing on a nil GameDB.
type GameDB struct {
	sc       *Scanner
	all      []indexedGame // every game, for looking one up by what is on disk
	byID     map[string]indexedGame
	protonIx map[string][]string
	bases    func([]string) []string
	blocked  map[string]bool // folders no save location may be
	// byInstall: games by install folder name, built when first asked.
	byInstall map[string][]indexedGame
}

// GameDB loads the game database (the manifest index) for lookups.
func (sc *Scanner) GameDB() *GameDB {
	if sc == nil || sc.CacheFile == "" {
		return nil
	}
	idx := sc.loadManifestIndex()
	if len(idx) == 0 {
		return nil
	}
	ds := &GameDB{sc: sc, all: idx, byID: map[string]indexedGame{}}
	for _, g := range idx {
		if g.SteamID != "" && len(g.Paths) > 0 {
			ds.byID[g.SteamID] = g
		}
	}
	if sc.goos() == "linux" {
		ds.protonIx = sc.protonPrefixIndex()
	}
	if sc.goos() == "windows" {
		ds.blocked = blockedRoots(windowsPathVars())
	} else {
		ds.blocked = linuxBlockedRoots(sc.linuxHome())
	}
	ds.bases = sc.installBaseCandidates()
	return ds
}

// GameMatch is a game from the game database, as found on this device.
type GameMatch struct {
	Name  string `json:"name"`
	AppID string `json:"appId,omitempty"`
	// SavePaths are the game's save locations that exist on this device,
	// most likely first.
	SavePaths []string `json:"savePaths"`
	// InstallDir is where it is installed, when that is known.
	InstallDir string `json:"installDir,omitempty"`
	// TrackedID and TrackedName name the game already tracked here that
	// this one is — its folder is, holds or is held by one of SavePaths, or it
	// has the App ID. Filled in by the daemon.
	TrackedID   string `json:"trackedId,omitempty"`
	TrackedName string `json:"trackedName,omitempty"`
}

// savePathsHere lists a game's save locations that exist on this device.
func (ds *GameDB) savePathsHere(g indexedGame) []string {
	if ds == nil || len(g.Paths) == 0 {
		return nil
	}
	return expandGamePaths(g, ds.sc.ludusaviVarSets(g, ds.protonIx), ds.bases, ds.blocked)
}

func (ds *GameDB) matchOf(g indexedGame, installDir string) GameMatch {
	return GameMatch{Name: g.Name, AppID: g.SteamID, SavePaths: ds.savePathsHere(g), InstallDir: installDir}
}

// Search finds games by name: the whole name first, then names that begin
// with what was typed, then names that contain it.
func (ds *GameDB) Search(query string, limit int) []GameMatch {
	q := normalizeGameName(query)
	if ds == nil || len(q) < 2 {
		return nil
	}
	type hit struct {
		g    indexedGame
		rank int
	}
	var hits []hit
	for _, g := range ds.all {
		n := normalizeGameName(g.Name)
		switch {
		case n == q:
			hits = append(hits, hit{g, 0})
		case strings.HasPrefix(n, q):
			hits = append(hits, hit{g, 1})
		case strings.Contains(n, q):
			hits = append(hits, hit{g, 2})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].rank != hits[j].rank {
			return hits[i].rank < hits[j].rank
		}
		return len(hits[i].g.Name) < len(hits[j].g.Name)
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	out := make([]GameMatch, 0, len(hits))
	for _, h := range hits {
		out = append(out, ds.matchOf(h.g, ""))
	}
	return out
}

// IdentifyProgram finds the game a program belongs to: by the Steam install
// folder it is in, or by the name of a folder above it that is the game's
// install folder in the game database ("G:\Games\Crimson Desert").
func (ds *GameDB) IdentifyProgram(exe string) (GameMatch, bool) {
	if ds == nil || exe == "" {
		return GameMatch{}, false
	}
	exe = filepath.Clean(exe)
	byAppID, _ := ds.sc.InstallDirs()
	for appID, dir := range byAppID {
		if dir != "" && isSubPath(dir, exe) {
			if g, ok := ds.gameByID(appID); ok {
				return ds.matchOf(g, dir), true
			}
		}
	}
	byFolder := ds.installNameIndex()
	for dir := filepath.Dir(exe); dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		if gs := byFolder[installKey(filepath.Base(dir))]; len(gs) > 0 {
			return ds.matchOf(pickGame(gs), dir), true
		}
	}
	return GameMatch{}, false
}

// IdentifyFolder finds the game a save folder belongs to: the game whose save
// location in the game database is this folder, or holds it, or is held by it
// (a folder picked one level too high).
func (ds *GameDB) IdentifyFolder(folder string) (GameMatch, bool) {
	return ds.identifyFolder(folder, 0)
}

// IdentifyFolderStrictly is IdentifyFolder for naming a game nobody asked
// about: a folder holding the save location only counts when that is at most
// one folder further down ("Duckov" holding "DuckovSaves"), never a folder
// like Documents that holds many games'.
func (ds *GameDB) IdentifyFolderStrictly(folder string) (GameMatch, bool) {
	return ds.identifyFolder(folder, 1)
}

// identifyFolder: maxDepth limits how far below the folder a save location
// held by it may be, 0 for no limit.
func (ds *GameDB) identifyFolder(folder string, maxDepth int) (GameMatch, bool) {
	if ds == nil || folder == "" {
		return GameMatch{}, false
	}
	folder = filepath.Clean(folder)
	low := strings.ToLower(folder)
	best, bestScore := indexedGame{}, 0
	for _, g := range ds.all {
		if len(g.Paths) == 0 {
			continue
		}
		// An install folder is only worked out for a game whose install
		// folder name is in the path at all: doing it for every game in the
		// database would look on disk tens of thousands of times.
		bases := func([]string) []string { return nil }
		for _, inst := range g.Installs {
			if inst != "" && strings.Contains(low, strings.ToLower(inst)) {
				bases = ds.bases
				break
			}
		}
		for _, vars := range ds.sc.ludusaviVarSets(g, ds.protonIx) {
			for _, tpl := range g.Paths {
				for _, expanded := range expandTemplate(tpl, vars, bases(g.Installs), g.SteamID) {
					score := 0
					switch {
					case sameUnder(folder, expanded):
						score = 3
					case under(expanded, folder):
						score = 2 // a folder inside the save location
					case under(folder, expanded):
						// The save location inside the folder picked, counted
						// to the folder a file location is in.
						rel, _ := relativeUnder(folder, expanded)
						if path.Ext(path.Base(rel)) != "" {
							rel = path.Dir(rel)
						}
						if maxDepth == 0 || strings.Count(rel, "/") < maxDepth {
							score = 1
						}
					}
					if score > bestScore || (score == bestScore && score > 0 && better(g, best)) {
						best, bestScore = g, score
					}
				}
			}
		}
	}
	if bestScore == 0 {
		return GameMatch{}, false
	}
	return ds.matchOf(best, ""), true
}

// sameUnder reports whether path is pattern (a template expanded, possibly
// with wildcards), compared component by component.
func sameUnder(path, pattern string) bool {
	rel, ok := relativeUnder(path, pattern+string(filepath.Separator)+"x")
	return ok && rel == "x"
}

// relativeUnder reports whether pattern (a path, possibly with wildcards)
// lies strictly inside root, and the rest of it below root. Compared
// component by component, case-insensitively, with either separator: the
// game database writes Windows paths, which a Proton prefix holds on Linux.
func relativeUnder(root, pattern string) (string, bool) {
	split := func(p string) []string {
		p = strings.ToLower(path.Clean(strings.ReplaceAll(p, `\`, "/")))
		var parts []string
		for _, c := range strings.Split(p, "/") {
			if c != "" {
				parts = append(parts, c)
			}
		}
		return parts
	}
	r, p := split(root), split(pattern)
	if len(r) == 0 || len(p) <= len(r) {
		return "", false
	}
	for i, rc := range r {
		if ok, err := path.Match(p[i], rc); err != nil || !ok {
			return "", false
		}
	}
	return strings.Join(p[len(r):], "/"), true
}

// under reports whether path lies strictly inside pattern.
func under(pattern, path string) bool {
	_, ok := relativeUnder(pattern, path)
	return ok
}

// better prefers a game with a Steam App ID — cover art, launching — when two
// match equally.
func better(g, than indexedGame) bool {
	return g.SteamID != "" && than.SteamID == ""
}

func pickGame(gs []indexedGame) indexedGame {
	best := gs[0]
	for _, g := range gs[1:] {
		if better(g, best) {
			best = g
		}
	}
	return best
}

func (ds *GameDB) gameByID(appID string) (indexedGame, bool) {
	if g, ok := ds.byID[appID]; ok {
		return g, true
	}
	for _, g := range ds.all {
		if g.SteamID == appID {
			return g, true
		}
	}
	return indexedGame{}, false
}

// installNameIndex maps install folder names, normalised, to the games that
// use them, for names specific enough to say which game a folder is.
func (ds *GameDB) installNameIndex() map[string][]indexedGame {
	if ds.byInstall != nil {
		return ds.byInstall
	}
	ds.byInstall = map[string][]indexedGame{}
	for _, g := range ds.all {
		for _, inst := range g.Installs {
			n := installKey(inst)
			if len(n) < 4 || genericInstallNames[n] {
				continue
			}
			ds.byInstall[n] = append(ds.byInstall[n], g)
		}
	}
	return ds.byInstall
}

var genericInstallNames = map[string]bool{
	"game": true, "games": true, "data": true, "bin": true, "binaries": true, "launcher": true,
	"client": true, "common": true, "steam": true, "app": true,
}

// installKey is a folder name as compared with the game database's install
// folder names: normalised, and without spaces, so "Project Zomboid" is the
// "ProjectZomboid" the database knows.
func installKey(name string) string {
	return strings.ReplaceAll(normalizeGameName(name), " ", "")
}
