package presets

import (
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
	return sc.deviceSettings(sc.loadManifestIndex())
}

// DeviceSettingsNoWait is DeviceSettings for the daemon while it starts, which
// must not wait for an index that is due a rebuild (loadManifestIndexNoWait):
// until the rebuild is done, settings files are left out as the index on disk
// names them, and the next start goes by the new one.
func (sc *Scanner) DeviceSettingsNoWait() *DeviceSettings {
	if sc == nil || sc.CacheFile == "" {
		return nil
	}
	return sc.deviceSettings(sc.loadManifestIndexNoWait())
}

func (sc *Scanner) deviceSettings(idx []indexedGame) *DeviceSettings {
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

// coversEverything reports a relative pattern that would exclude the whole
// folder — a settings entry naming the folder the save is tracked at. Never
// applied: a wrong exclusion loses saves, a missed one only syncs a setting.
func coversEverything(rel string) bool {
	first := strings.SplitN(rel, "/", 2)[0]
	return strings.Trim(first, "*") == "" || first == "*.*"
}
