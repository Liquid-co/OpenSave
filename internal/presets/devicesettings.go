package presets

import (
	"path"
	"sort"
	"strings"
)

// DeviceSettings answers, for a tracked game, which of the files in its save
// folders are its settings rather than its save: what the Ludusavi manifest
// tags "config" and not "save" (indexedGame.Config). A device's graphics
// settings are the usual case — a game that keeps them beside its saves had
// one PC's resolution and quality handed to every other.
//
// The answer is a list of exclusion patterns, in the syntax of a game's own
// exclusion rules and relative to the save folder they fall in, so it is
// applied by exactly the machinery that applies those: never synced in either
// direction, never deleted on another device, still in every snapshot.
type DeviceSettings struct {
	sc       *Scanner
	byID     map[string]indexedGame
	byName   map[string]indexedGame
	protonIx map[string][]string
	bases    func([]string) []string
}

// DeviceSettings loads the manifest index once for any number of lookups.
// Nil when there is no index.
func (sc *Scanner) DeviceSettings() *DeviceSettings {
	if sc == nil || sc.CacheFile == "" {
		return nil
	}
	idx := sc.loadManifestIndex()
	if len(idx) == 0 {
		return nil
	}
	ds := &DeviceSettings{sc: sc, byID: map[string]indexedGame{}, byName: map[string]indexedGame{}}
	for _, g := range idx {
		if len(g.Config) == 0 && len(g.Paths) == 0 {
			continue
		}
		if g.SteamID != "" {
			ds.byID[g.SteamID] = g
		}
		if key := normalizeGameName(g.Name); key != "" {
			ds.byName[key] = g
		}
	}
	if sc.goos() == "linux" {
		ds.protonIx = sc.protonPrefixIndex()
	}
	ds.bases = sc.installBaseCandidates()
	return ds
}

// Patterns returns the exclusion patterns for a game's settings that lie
// inside one of roots (its save folders; the first is the main one), each
// relative to the root it is in. Patterns for any other root than the first
// are not returned: a game's rules are written against its main folder.
func (ds *DeviceSettings) Patterns(name, appID string, roots []string) []string {
	if ds == nil || len(roots) == 0 {
		return nil
	}
	g, ok := ds.byID[appID]
	if !ok || appID == "" {
		g, ok = ds.byName[normalizeGameName(stripNameSuffixes(name))]
	}
	if !ok {
		return nil
	}
	installBases := ds.bases(g.Installs)
	seen := map[string]bool{}
	var out []string
	for _, vars := range ds.sc.ludusaviVarSets(g, ds.protonIx) {
		for _, tpl := range g.Config {
			for _, expanded := range expandTemplate(tpl, vars, installBases, g.SteamID) {
				rel, ok := relativeUnder(roots[0], expanded)
				if !ok || coversEverything(rel) || seen[rel] {
					continue
				}
				seen[rel] = true
				out = append(out, "/"+rel)
			}
		}
	}
	sort.Strings(out)
	return out
}

// relativeUnder returns pattern relative to root when it names something
// strictly inside it, comparing component by component so that a wildcard the
// template had (a store user id) matches the folder actually tracked.
// Case-insensitive, as exclusion rules are.
func relativeUnder(root, pattern string) (string, bool) {
	split := func(p string) []string {
		// Either separator on every system: a pattern written for Windows
		// (Ludusavi's data, a Proton prefix) is matched on Linux too, and
		// path.Match would read a backslash as an escape.
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

// coversEverything reports a relative pattern that would exclude the whole
// folder — a settings entry naming the folder the save is tracked at. Never
// applied: a wrong exclusion loses saves, a missed one only syncs a setting.
func coversEverything(rel string) bool {
	first := strings.SplitN(rel, "/", 2)[0]
	return strings.Trim(first, "*") == "" || first == "*.*"
}
