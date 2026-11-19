package presets

import (
	"compress/gzip"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gopkg.in/yaml.v3"
)

// Ludusavi manifest integration.
//
// The Ludusavi manifest (github.com/mtkennerly/ludusavi-manifest) is a
// community-maintained database of save locations for tens of thousands of
// games, sourced from PCGamingWiki. Detection is purely by path pattern —
// no store validation — so it finds saves for Steam, GOG, Epic, itch, and
// cracked/repack installs alike, as long as the game writes to its usual
// location.
//
// The raw manifest is a large YAML file. We download it at most once per
// week, distill the Windows-relevant entries into a compact JSON index
// next to it, and expand + stat the patterns during a scan.

const ludusaviManifestURL = "https://raw.githubusercontent.com/mtkennerly/ludusavi-manifest/master/data/manifest.yaml"

const manifestMaxAge = 7 * 24 * time.Hour

// manifestFileEntry / manifestGame model just the YAML we care about.
type manifestFileEntry struct {
	Tags []string `yaml:"tags"`
	When []struct {
		OS    string `yaml:"os"`
		Store string `yaml:"store"`
	} `yaml:"when"`
}

type manifestGame struct {
	Files map[string]manifestFileEntry `yaml:"files"`
	// Registry keys holding save data, in the same shape as Files. 2,810 games
	// declare one, and for 619 of them it is the only place a save exists.
	Registry   map[string]manifestFileEntry `yaml:"registry"`
	InstallDir map[string]struct{}          `yaml:"installDir"`
	Steam      struct {
		ID int64 `yaml:"id"`
	} `yaml:"steam"`
}

// indexedGame is the compact, pre-filtered form persisted as JSON so scans
// don't re-parse megabytes of YAML.
type indexedGame struct {
	Name     string   `json:"n"`
	SteamID  string   `json:"s,omitempty"`
	Installs []string `json:"i,omitempty"` // installDir folder names
	Paths    []string `json:"p"`           // Windows-relevant save path templates
	// Registry keys holding this game's save data, full hive names with
	// forward slashes exactly as the manifest writes them. Absent from an
	// index built by an older release, which reads as "none" — the field is
	// additive, so an index on disk from a previous version still loads.
	Registry []string `json:"r,omitempty"`
	// Config is where the game keeps its settings and nothing else: entries
	// the manifest tags "config" and not "save". A device's own graphics
	// settings among them; inside a tracked save folder they are left out of
	// syncing (DeviceSettingsPatterns).
	Config []string `json:"c,omitempty"`
	// When holds the manifest's conditions on the templates in Paths that
	// have any, by template: the OS a save path is for, and the store. See
	// pathsFor.
	When map[string][]pathWhen `json:"w,omitempty"`
}

// pathWhen is one of a template's conditions; it applies when any one of
// them holds.
type pathWhen struct {
	OS    string `json:"o,omitempty"`
	Store string `json:"t,omitempty"`
}

// gamePath is one of a game's templates with the placeholder values to
// expand it with.
type gamePath struct {
	tpl  string
	vars map[string]string
}

// pathsFor returns the game's templates that apply under one var set, each
// with the vars to expand it with.
//
// The manifest gives many templates an OS (when: os). A var set stands for one
// OS's folders, and a template for another OS is not tried there: a
// Windows-only "<home>/Saved Games/X" is not looked for in a Linux home, nor a
// Linux-only "<home>/.local/share/X" inside a Proton prefix. Templates in the
// game's install folder are tried under every var set all the same, since a
// Windows build run through Wine writes there just as it does on Windows.
//
// The store (when: store) does not decide whether a template is tried — a
// copy of a game from anywhere keeps its saves where the manifest says, which
// is how Ludusavi treats it too — with one exception taken from Ludusavi
// (src/scan.rs): for Ubisoft Connect, <root> is that launcher's own folder.
func (g indexedGame) pathsFor(vars map[string]string) []gamePath {
	target := varSetOS(vars)
	var uplay map[string]string
	out := make([]gamePath, 0, len(g.Paths))
	for _, tpl := range g.Paths {
		when := g.When[tpl]
		if !installRelative(tpl) && !whenAllowsOS(when, target) {
			continue
		}
		v := vars
		if whenHasStore(when, "uplay") && strings.Contains(tpl, "<root>") {
			if uplay == nil {
				uplay = withUplayRoot(vars)
			}
			if uplay != nil {
				v = uplay
			}
		}
		out = append(out, gamePath{tpl, v})
	}
	return out
}

// varSetOS names the OS whose folders a var set holds, in the manifest's
// terms: the Windows placeholders (Windows itself, or a Wine prefix), the XDG
// ones (Linux), or <home> alone (macOS).
func varSetOS(vars map[string]string) string {
	switch {
	case vars["<winAppData>"] != "":
		return "windows"
	case vars["<xdgData>"] != "":
		return "linux"
	}
	return "mac"
}

// installRelative reports whether a template is in the game's install folder.
func installRelative(tpl string) bool {
	return strings.Contains(tpl, "<base>") || strings.Contains(tpl, "<root>") || strings.Contains(tpl, "<game>")
}

func whenAllowsOS(when []pathWhen, target string) bool {
	if len(when) == 0 {
		return true
	}
	for _, w := range when {
		if w.OS == "" || w.OS == target {
			return true
		}
	}
	return false
}

func whenHasStore(when []pathWhen, store string) bool {
	for _, w := range when {
		if w.Store == store {
			return true
		}
	}
	return false
}

// withUplayRoot returns vars with <root> at Ubisoft Connect's install folder on
// the same drive as ProgramData — C:\ on Windows, drive_c in a prefix — or nil
// when vars have no Windows drive.
func withUplayRoot(vars map[string]string) map[string]string {
	pd := vars["<winProgramData>"]
	if pd == "" {
		return nil
	}
	out := make(map[string]string, len(vars)+1)
	for k, v := range vars {
		out[k] = v
	}
	out["<root>"] = filepath.Join(filepath.Dir(pd), "Program Files (x86)", "Ubisoft", "Ubisoft Game Launcher")
	return out
}

// manifestPaths derives the manifest + index locations from the scanner's
// cache file directory (~/.opensave).
func (sc *Scanner) manifestPaths() (yamlPath, indexPath string) {
	dir := filepath.Dir(sc.CacheFile)
	return filepath.Join(dir, "ludusavi-manifest.yaml"), filepath.Join(dir, indexFileName)
}

// indexFileName carries the index's format in its name.
//
// The index is rebuilt only when it is older than the manifest it came from,
// which is the right rule for content and the wrong one for shape: adding
// registry keys to the format left every existing install reading a file that
// had none, and no manifest update was due to trigger a rebuild. Games whose
// save is in the registry would simply never have been found.
//
// A new name rather than a version field inside the file. An older build keeps
// reading the file it knows and a newer one builds its own, so a user moving
// between the two is never handed an index whose shape their code predates.
// Bump this whenever indexedGame gains or loses a field that a scan depends on,
// or the rule for which templates it keeps changes: v4 keeps <winDir>
// templates, and adds each template's conditions (When).
const indexFileName = "ludusavi-index-v4.json"

// scanLudusavi expands the manifest's save-path templates and returns the
// locations that actually exist on this machine.
func (sc *Scanner) scanLudusavi(seen map[string]bool) []DiscoveredSave {
	if sc.CacheFile == "" {
		return nil
	}
	games := sc.loadManifestIndex()
	// Naming and cover art match against whatever manifest this scan loaded,
	// rather than only the copy compiled into the binary. See
	// adoptManifestForNaming.
	adoptManifestForNaming(games)
	if len(games) == 0 {
		return nil
	}

	// Per-OS placeholder resolution. Windows: the %VAR% roots. Linux:
	// native XDG/home roots plus, for Steam games, a var set per Proton
	// prefix so Windows-path templates resolve inside the Wine prefix.
	var blocked map[string]bool
	var protonIdx map[string][]string
	if sc.goos() == "windows" {
		wv := windowsPathVars()
		if wv == nil {
			return nil
		}
		blocked = blockedRoots(wv)
	} else {
		blocked = linuxBlockedRoots(sc.linuxHome())
		protonIdx = sc.prefixIndex()
	}
	baseDirs := sc.installBaseCandidates()

	var mu sync.Mutex
	var found []DiscoveredSave
	markSeen := func(p string) bool { // true if fresh
		mu.Lock()
		defer mu.Unlock()
		if seen[p] {
			return false
		}
		seen[p] = true
		return true
	}

	jobs := make(chan indexedGame)
	var wg sync.WaitGroup
	for w := 0; w < 16; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for g := range jobs {
				varSets := sc.ludusaviVarSets(g, protonIdx)
				dirs := expandGamePaths(g, varSets, baseDirs, blocked)
				for i, dir := range dirs {
					abs, err := filepath.Abs(dir)
					if err != nil || !markSeen(abs) {
						continue
					}
					id := "ludusavi-" + sanitizeID(g.Name)
					name := g.Name
					if len(dirs) > 1 {
						id += "-" + strconv.Itoa(i)
						name += " (" + filepath.Base(dir) + ")"
					}
					mu.Lock()
					found = append(found, DiscoveredSave{
						ID: id, Name: name, Type: "game", SavePath: abs, AppID: g.SteamID,
					})
					mu.Unlock()
				}
			}
		}()
	}
	for _, g := range games {
		jobs <- g
	}
	close(jobs)
	wg.Wait()
	return found
}

// expandGamePaths resolves one game's templates to existing directories,
// trying every placeholder var set (Windows has one; Linux has the native
// XDG set plus one per Proton prefix).
func expandGamePaths(g indexedGame, varSets []map[string]string, baseDirs func([]string) []string, blocked map[string]bool) []string {
	installBases := baseDirs(g.Installs)

	// Install roots are as off-limits as the blocked profile roots: many
	// games (Sonic & Sega All-Stars Racing, most repack-era titles) keep
	// their save file directly in the install dir, and offering that dir
	// would snapshot the entire multi-GB game.
	installRoots := map[string]bool{}
	for _, b := range installBases {
		installRoots[strings.ToLower(filepath.Clean(b))] = true
	}
	// A Wine prefix's windows folder is as broad as the real one, and only
	// its own var set knows where it is.
	var setBlocked map[string]bool
	noWiden := func(dir string) bool {
		l := strings.ToLower(filepath.Clean(dir))
		return installRoots[l] || blocked[l] || setBlocked[l]
	}

	var hits []string
	push := func(dir string) {
		if dir == "" || noWiden(dir) {
			return
		}
		for _, d := range hits {
			if d == dir {
				return
			}
		}
		if len(hits) < 12 {
			hits = append(hits, dir)
		}
	}

	for _, vars := range varSets {
		setBlocked = windowsDirRoots(vars)
		for _, p := range g.pathsFor(vars) {
			for _, expanded := range expandTemplate(p.tpl, p.vars, installBases, g.SteamID) {
				for _, hit := range statOrGlob(expanded, noWiden) {
					push(hit)
				}
			}
		}
	}

	// Collapse nesting: when one hit contains another (game dir plus its
	// profile subdir plus Screenshots…), tracking the ancestor covers all
	// of it — keep only the outermost dirs. At most three rows per game:
	// more is a loose template matching unrelated folders, and a row's id is
	// its position here, so the order is kept as it was.
	var out []string
	for _, cand := range hits {
		nested := false
		for _, other := range hits {
			if other != cand && isSubPath(other, cand) {
				nested = true
				break
			}
		}
		if !nested && len(out) < 3 {
			out = append(out, cand)
		}
	}
	return out
}

// isSubPath reports whether child lives inside parent.
func isSubPath(parent, child string) bool {
	p := strings.ToLower(parent) + string(filepath.Separator)
	return strings.HasPrefix(strings.ToLower(child), p)
}

// expandTemplate substitutes <placeholders>; a template may fan out to
// several concrete patterns (one per install-dir candidate). Templates
// with leftover placeholders are dropped.
func expandTemplate(tpl string, vars map[string]string, installBases []string, steamID string) []string {
	// A store id that becomes "*" below would, next to a "*" written beside
	// it, make "**": a walk of every folder underneath rather than one
	// folder. Ludusavi folds <storeUserId> the same way (parse_paths in its
	// src/scan.rs); <storeGameId> is "*" too when the Steam id is unknown.
	s := tpl
	wild := []string{"<storeUserId>"}
	if steamID == "" {
		wild = append(wild, "<storeGameId>")
	}
	for _, id := range wild {
		s = strings.ReplaceAll(s, "*"+id, id)
		s = strings.ReplaceAll(s, id+"*", id)
	}
	for k, v := range vars {
		s = strings.ReplaceAll(s, k, v)
	}
	if steamID != "" {
		s = strings.ReplaceAll(s, "<storeGameId>", steamID)
	} else {
		s = strings.ReplaceAll(s, "<storeGameId>", "*")
	}
	s = strings.ReplaceAll(s, "<storeUserId>", "*")

	var cands []string
	if strings.Contains(s, "<base>") || strings.Contains(s, "<game>") || strings.Contains(s, "<root>") {
		for _, base := range installBases {
			c := strings.ReplaceAll(s, "<base>", base)
			c = strings.ReplaceAll(c, "<game>", filepath.Base(base))
			c = strings.ReplaceAll(c, "<root>", filepath.Dir(base))
			cands = append(cands, c)
		}
	} else {
		cands = []string{s}
	}

	var out []string
	for _, c := range cands {
		if strings.ContainsRune(c, '<') { // unresolved placeholder
			continue
		}
		p := filepath.FromSlash(c)
		if !filepath.IsAbs(p) {
			// A relative template would resolve against the process CWD —
			// never a save location.
			continue
		}
		out = append(out, p)
	}
	return out
}

// statOrGlob resolves one concrete pattern to existing save locations:
// globs wildcards and maps files to their parent dir — except when noWiden
// says the parent is too broad to track (a game install root, Documents),
// in which case the save file itself is the location. Single-file save
// paths are supported end to end (watcher, snapshots, sync).
func statOrGlob(pattern string, noWiden func(dir string) bool) []string {
	toCandidate := func(p string) string {
		info, err := os.Stat(p)
		if err != nil {
			return ""
		}
		if info.IsDir() {
			return p
		}
		if parent := filepath.Dir(p); !noWiden(parent) {
			return parent
		}
		return p
	}

	if strings.ContainsAny(pattern, "*?[") {
		matches := globPaths(pattern)
		if len(matches) == 0 {
			return nil
		}
		// At most four locations from one pattern, so a loose one cannot
		// flood a game with rows. Counted after mapping files to their
		// folder: counting files, five saves in one folder hid the rest.
		var dirs []string
		seen := map[string]bool{}
		for _, m := range matches {
			if d := toCandidate(m); d != "" && !seen[d] {
				seen[d] = true
				dirs = append(dirs, d)
			}
			if len(dirs) >= 4 {
				break
			}
		}
		return dirs
	}

	if d := toCandidate(pattern); d != "" {
		return []string{d}
	}
	// Leaf missing: no parent-dir fallback. It sounds helpful, but in
	// practice it surfaces shared engine roots (RenPy, Godot, a studio's
	// LocalLow folder) as "the save" — and a game with no save files yet
	// has nothing worth tracking anyway.
	return nil
}

// knownDocumentsDir is where Windows says the Documents folder of the account
// with this home is, or "" when it cannot say (documentsKnownFolder). A
// variable so tests can stand in for the shell.
var knownDocumentsDir = documentsKnownFolder

// windowsPathVars maps manifest placeholders to this machine's paths.
func windowsPathVars() map[string]string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil
	}
	username := os.Getenv("USERNAME")
	if username == "" {
		username = filepath.Base(home)
	}
	// Documents is not always in the home folder: OneDrive's folder backup
	// moves it, and the games follow it.
	documents := knownDocumentsDir(home)
	if documents == "" {
		documents = filepath.Join(home, "Documents")
	}
	return map[string]string{
		"<home>":               home,
		"<winAppData>":         ResolvePath("%APPDATA%"),
		"<winLocalAppData>":    ResolvePath("%LOCALAPPDATA%"),
		"<winLocalAppDataLow>": filepath.Join(home, "AppData", "LocalLow"),
		"<winDocuments>":       documents,
		"<winPublic>":          ResolvePath("%PUBLIC%"),
		"<winProgramData>":     ResolvePath("%PROGRAMDATA%"),
		"<winDir>":             windowsDir(),
		"<osUserName>":         username,
	}
}

// windowsDir is the Windows folder, for <winDir>.
func windowsDir() string {
	for _, env := range []string{"SystemRoot", "WINDIR"} {
		if v := os.Getenv(env); v != "" {
			return v
		}
	}
	return `C:\Windows`
}

// windowsDirRoots are the Windows folder of a var set (the real one, or a Wine
// prefix's drive_c/windows) and the system folders in it, lower-cased: never a
// save location, whatever a <winDir> template matches. Nil without one.
func windowsDirRoots(vars map[string]string) map[string]bool {
	win := vars["<winDir>"]
	if win == "" {
		return nil
	}
	out := map[string]bool{}
	for _, p := range []string{win, filepath.Join(win, "System32"), filepath.Join(win, "SysWOW64")} {
		out[strings.ToLower(filepath.Clean(p))] = true
	}
	return out
}

// ludusaviVarSets returns the placeholder→path maps to try for one game on
// the scanner's target OS.
func (sc *Scanner) ludusaviVarSets(g indexedGame, protonIdx map[string][]string) []map[string]string {
	switch sc.goos() {
	case "windows":
		if v := windowsPathVars(); v != nil {
			return []map[string]string{v}
		}
		return nil
	case "darwin":
		// The manifest has no dedicated macOS placeholder — its ~2,800
		// mac-only entries write paths as "<home>/Library/Application
		// Support/…" directly, which <home> alone resolves. There is
		// nothing here for Proton: Wine prefixes are a Linux-only
		// mechanism, so a Windows-only template genuinely has no
		// counterpart to try on a Mac.
		if home := sc.linuxHome(); home != "" {
			return []map[string]string{{"<home>": home}}
		}
		return nil
	}
	home := sc.linuxHome()
	sets := []map[string]string{linuxNativeVars(home)}
	// Proton, and the other launchers' Wine prefixes: Windows-path templates
	// resolve inside this game's prefix.
	for _, userHome := range prefixHomesFor(g, protonIdx) {
		sets = append(sets, protonWinVars(userHome))
	}
	return sets
}

// linuxNativeVars maps the manifest's Linux placeholders. Windows
// placeholders are intentionally absent so win-only templates don't
// resolve to bogus native paths (they resolve under Proton instead).
func linuxNativeVars(home string) map[string]string {
	// $XDG_DATA_HOME, $XDG_CONFIG_HOME and $USER describe the account this
	// PROCESS is running as. They describe `home` only when `home` IS that
	// account's home directory, and taking them when it is not sends the scan
	// looking somewhere else entirely — for a home that was passed in
	// precisely because it is not the default one.
	//
	// That is not hypothetical. It is why a scan of a supplied home directory
	// found nothing on any machine where XDG_CONFIG_HOME happened to be set,
	// while passing everywhere it was not: the caller said which home to look
	// in, and this quietly used a different one.
	//
	// When the two do agree, the environment still wins, because a user who
	// has moved their config directory keeps their saves there and the whole
	// point of the variable is to say so.
	forThisUser := ownHome(home)

	dataHome := filepath.Join(home, ".local", "share")
	if v := os.Getenv("XDG_DATA_HOME"); v != "" && forThisUser {
		dataHome = v
	}
	configHome := filepath.Join(home, ".config")
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" && forThisUser {
		configHome = v
	}
	return map[string]string{
		"<home>":      home,
		"<xdgData>":   dataHome,
		"<xdgConfig>": configHome,
		"<osUserName>": func() string {
			if u := os.Getenv("USER"); u != "" && forThisUser {
				return u
			}
			return filepath.Base(home)
		}(),
	}
}

// ownHome reports whether home is the home directory of the account this
// process is running as.
//
// Unknown counts as "no": with nothing to compare against, the home the caller
// named is the better answer than an environment variable that may describe
// somebody else.
func ownHome(home string) bool {
	own := os.Getenv("HOME")
	if own == "" || home == "" {
		return false
	}
	return filepath.Clean(own) == filepath.Clean(home)
}

// protonWinVars maps Windows placeholders to their location inside a Wine
// prefix, for one of its users' home folders (drive_c/users/<user>): Wine
// lays out AppData/Documents there, and maps the user's home to the same
// profile dir. Proton's user is "steamuser"; Heroic, Lutris and Bottles name
// it after the account.
func protonWinVars(userHome string) map[string]string {
	driveC := filepath.Dir(filepath.Dir(userHome)) // …/pfx/drive_c
	return map[string]string{
		"<home>":               userHome,
		"<winAppData>":         filepath.Join(userHome, "AppData", "Roaming"),
		"<winLocalAppData>":    filepath.Join(userHome, "AppData", "Local"),
		"<winLocalAppDataLow>": filepath.Join(userHome, "AppData", "LocalLow"),
		"<winDocuments>":       filepath.Join(userHome, "Documents"),
		"<winPublic>":          filepath.Join(driveC, "users", "Public"),
		"<winProgramData>":     filepath.Join(driveC, "ProgramData"),
		"<winDir>":             filepath.Join(driveC, "windows"),
		"<osUserName>":         filepath.Base(userHome),
	}
}

// prefixNameKey is the key a non-Steam prefix is indexed under: its folder
// name, which Heroic and Bottles take from the game's title and Lutris from
// its slug, compared as install folder names are. "" for a name too short or
// too generic to say which game it is ("default", a bare ".wine").
func prefixNameKey(name string) string {
	n := installKey(name)
	if len(n) < 4 || genericInstallNames[n] || genericPrefixNames[n] {
		return ""
	}
	return "name:" + n
}

var genericPrefixNames = map[string]bool{
	"default": true, "wine": true, "prefix": true, "prefixes": true, "pfx": true,
	"bottle": true, "wine64": true, "windows": true,
}

// prefixHomesFor returns the user homes of the prefixes that are this game's:
// its Proton prefixes, and the other launchers' prefixes named after it or
// after one of its install folders.
//
// A prefix only gets the manifest paths of a game it can be told belongs to.
// Ludusavi, given a Wine prefix as a root, tries every game in it instead
// (Store::OtherWine in its src/scan.rs); here that would be tens of thousands
// of games' paths for each prefix on the machine, every scan.
func prefixHomesFor(g indexedGame, idx map[string][]string) []string {
	var out []string
	if g.SteamID != "" {
		out = append(out, idx[g.SteamID]...)
	}
	keys := map[string]bool{}
	for _, name := range append([]string{g.Name}, g.Installs...) {
		if k := prefixNameKey(name); k != "" && !keys[k] {
			keys[k] = true
			out = append(out, idx[k]...)
		}
	}
	return out
}

// protonPrefixIndex maps Steam AppID -> the user home dirs of its Proton
// prefixes across all libraries. Built once per scan.
func (sc *Scanner) protonPrefixIndex() map[string][]string {
	idx := map[string][]string{}
	for _, lib := range sc.steamLibraryPaths() {
		compat := filepath.Join(lib, "steamapps", "compatdata")
		entries, err := os.ReadDir(compat)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() || !isAppID(e.Name()) {
				continue
			}
			// Usually "steamuser", but a prefix made by another tool and
			// added to Steam can name its user after the account.
			pfx := filepath.Join(compat, e.Name(), "pfx")
			idx[e.Name()] = append(idx[e.Name()], prefixUserDirs(pfx)...)
		}
	}
	return idx
}

// prefixIndex is protonPrefixIndex plus, on Linux, the Heroic, Lutris,
// Bottles and Wine prefixes named after a game, by prefixNameKey (see
// prefixHomesFor). Finding those reads every launcher's folders and settings,
// so it is for a scan and a game lookup, not for the check made while a peer
// waits (CatalogueSaveLocation), which keeps to Proton.
func (sc *Scanner) prefixIndex() map[string][]string {
	idx := sc.protonPrefixIndex()
	if sc.goos() != "linux" {
		return idx
	}
	for _, prefix := range sc.winePrefixDirs() {
		if key := prefixNameKey(filepath.Base(prefix.path)); key != "" {
			idx[key] = append(idx[key], prefixUserDirs(prefix.path)...)
		}
	}
	return idx
}

// linuxBlockedRoots are directories too broad to ever offer as a save
// location on Linux.
func linuxBlockedRoots(home string) map[string]bool {
	blocked := map[string]bool{}
	add := func(p string) {
		if p != "" {
			blocked[strings.ToLower(filepath.Clean(p))] = true
		}
	}
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		dataHome = filepath.Join(home, ".local", "share")
	}
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		configHome = filepath.Join(home, ".config")
	}
	add(home)
	add(dataHome)
	add(configHome)
	add(filepath.Join(home, "Documents"))
	add(filepath.Join(home, "Documents", "My Games"))
	add(filepath.Join(home, "Saved Games"))
	add(filepath.Join(home, "Desktop"))
	add(filepath.Join(home, "Downloads"))
	return blocked
}

// blockedRoots are directories too broad to ever offer as a save location
// (a loose manifest pattern must never suggest tracking all of Documents).
func blockedRoots(vars map[string]string) map[string]bool {
	blocked := map[string]bool{}
	add := func(p string) {
		if p != "" {
			blocked[strings.ToLower(p)] = true
		}
	}
	for _, v := range vars {
		add(v)
	}
	home := vars["<home>"]
	add(filepath.Join(home, "Documents")) // still too broad when Documents is elsewhere
	add(filepath.Join(home, "Documents", "My Games"))
	if docs := vars["<winDocuments>"]; docs != "" {
		add(filepath.Join(docs, "My Games"))
	}
	add(filepath.Join(home, "Saved Games"))
	add(filepath.Join(home, "Desktop"))
	add(filepath.Join(home, "Downloads"))
	add(filepath.Join(vars["<winPublic>"], "Documents"))
	for p := range windowsDirRoots(vars) {
		add(p)
	}
	add(filepath.Join(vars["<winLocalAppData>"], "Programs"))
	add(filepath.Join(vars["<winLocalAppData>"], "Packages"))
	add(filepath.Join(vars["<winLocalAppData>"], "User Data"))
	// Shared engine roots hold every game made with that engine — a match
	// must point at a specific game's folder inside them, never the root.
	add(filepath.Join(vars["<winAppData>"], "RenPy"))
	add(filepath.Join(vars["<winAppData>"], "Godot"))
	add(filepath.Join(vars["<winAppData>"], "Godot", "app_userdata"))
	add(filepath.Join(vars["<winAppData>"], "LOVE"))
	add(filepath.Join(vars["<winLocalAppDataLow>"], "DefaultCompany"))
	return blocked
}

// installBaseCandidates returns a resolver from installDir names to
// existing absolute install paths: Steam's steamapps/common across every
// library, plus game folders sitting next to steamapps (where portable
// and cracked installs usually live).
func (sc *Scanner) installBaseCandidates() func([]string) []string {
	libs := sc.steamLibraryPaths()
	installed := map[string]string{}
	// Non-Steam launchers first, so a game present in both is resolved
	// against its Steam install — that is the copy whose userdata folder the
	// rest of the scan is already looking at.
	for name, dir := range sc.launcherInstallDirs() {
		installed[name] = dir
	}
	for _, a := range steamInstalledApps(libs) {
		installed[strings.ToLower(filepath.Base(a.InstallDir))] = a.InstallDir
	}
	return func(names []string) []string {
		var out []string
		for _, name := range names {
			if p := installed[strings.ToLower(name)]; p != "" {
				out = append(out, p)
				continue
			}
			for _, lib := range libs {
				for _, cand := range []string{
					filepath.Join(lib, "steamapps", "common", name),
					filepath.Join(lib, name),
				} {
					if dirExists(cand) {
						out = append(out, cand)
						break
					}
				}
			}
		}
		return out
	}
}

// ── embedded snapshot + manifest download + index ───────────────────────

// A compressed snapshot of the index ships inside the binary, so the very
// first scan works instantly and fully offline — no download required.
// Fresher data still arrives via the background weekly refresh; a local
// downloaded manifest always outranks the embedded snapshot.
// Regenerate with: GEN_EMBED=1 go test ./internal/presets/ -run GenerateEmbeddedIndex
//
//go:embed embedded/ludusavi-index.json.gz
var embeddedFS embed.FS

var embeddedIndex struct {
	once  sync.Once
	games []indexedGame
}

func loadEmbeddedIndex() []indexedGame {
	embeddedIndex.once.Do(func() {
		f, err := embeddedFS.Open("embedded/ludusavi-index.json.gz")
		if err != nil {
			return
		}
		defer f.Close()
		zr, err := gzip.NewReader(f)
		if err != nil {
			return
		}
		defer zr.Close()
		raw, err := io.ReadAll(zr)
		if err != nil {
			return
		}
		_ = json.Unmarshal(raw, &embeddedIndex.games)
	})
	return embeddedIndex.games
}

// loadManifestIndex returns the compact index: a locally downloaded
// manifest when present, the embedded snapshot otherwise. A refresh runs
// in the background when the local copy is missing or stale — scans never
// wait on the network. The embedded fallback only applies when the
// manifest feature is enabled (ManifestURL set — always true in
// production; hermetic tests leave it empty).
func (sc *Scanner) loadManifestIndex() []indexedGame {
	return sc.manifestIndex(-1)
}

// manifestIndex is loadManifestIndex for a caller that cannot wait long.
//
// Indexing the full manifest takes half a minute. A scan can wait for that; a
// peer's request cannot, as the peer gives up after thirty seconds and the
// sync fails. So when the index must first be built from a newer manifest,
// this waits at most wait for it (a negative wait: as long as it takes) and
// meanwhile answers with the index it had.
func (sc *Scanner) manifestIndex(wait time.Duration) []indexedGame {
	yamlPath, indexPath := sc.manifestPaths()

	embedded := func() []indexedGame {
		if sc.ManifestURL == "" {
			return nil
		}
		return loadEmbeddedIndex()
	}

	sc.refreshManifestAsync(yamlPath)

	yamlInfo, err := os.Stat(yamlPath)
	if err != nil {
		return embedded()
	}

	c := indexCacheFor(indexPath)
	if games := c.current(indexPath, yamlInfo); len(games) > 0 {
		return games
	}
	done := c.rebuild(yamlPath, indexPath)
	if wait < 0 {
		<-done
	} else {
		timer := time.NewTimer(wait)
		select {
		case <-done:
		case <-timer.C:
		}
		timer.Stop()
	}
	if games := c.lastKnown(indexPath); len(games) > 0 {
		return games
	}
	return embedded()
}

// loadManifestIndexNoWait is loadManifestIndex for a caller that must not wait
// on a rebuild at all: the daemon while it starts, before the window can open.
// Building the index parses the whole manifest, about a minute on an ordinary
// PC, and it is due at the first start after the index format changes and
// after every weekly manifest download. This answers with the index it had,
// however old, or the embedded one when there is none, while the rebuild runs
// in the background.
func (sc *Scanner) loadManifestIndexNoWait() []indexedGame {
	return sc.manifestIndex(0)
}

// indexCache holds one index file's games in memory, so a check made while a
// peer waits decodes nothing, and builds the index one build at a time: a scan
// and a peer's request share a build rather than each parsing the manifest.
type indexCache struct {
	mu       sync.Mutex
	games    []indexedGame
	from     manifestStamp // the manifest these games were built from
	building chan struct{} // closed when the running build ends; nil if none
}

// manifestStamp tells one copy of the manifest from another.
type manifestStamp struct {
	mod  time.Time
	size int64
}

func stampOf(info os.FileInfo) manifestStamp {
	return manifestStamp{info.ModTime(), info.Size()}
}

func (a manifestStamp) same(b manifestStamp) bool {
	return a.mod.Equal(b.mod) && a.size == b.size
}

var indexCaches sync.Map // index path → *indexCache

// buildIndex is buildManifestIndex, replaced by tests that need a build to
// take a while.
var buildIndex = buildManifestIndex

func indexCacheFor(indexPath string) *indexCache {
	c, _ := indexCaches.LoadOrStore(indexPath, &indexCache{})
	return c.(*indexCache)
}

// current returns the games when they are those of the manifest as it is now:
// built from it in this run, or read from an index file newer than it.
func (c *indexCache) current(indexPath string, yamlInfo os.FileInfo) []indexedGame {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.games != nil && c.from.same(stampOf(yamlInfo)) {
		return c.games
	}
	idxInfo, err := os.Stat(indexPath)
	if err != nil || !idxInfo.ModTime().After(yamlInfo.ModTime()) {
		return nil
	}
	games := readIndexFile(indexPath)
	if len(games) == 0 {
		return nil
	}
	c.games, c.from = games, stampOf(yamlInfo)
	return games
}

// lastKnown returns the games last built or read, else the index file however
// old: while a newer manifest is indexed, the previous one still answers.
func (c *indexCache) lastKnown(indexPath string) []indexedGame {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.games == nil {
		c.games = readIndexFile(indexPath)
	}
	return c.games
}

// rebuild starts indexing the manifest unless that is already under way, and
// returns a channel closed when it ends. A manifest replaced while it is read
// (a download landing) is read again, so whoever waits gets the newest.
func (c *indexCache) rebuild(yamlPath, indexPath string) <-chan struct{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.building != nil {
		return c.building
	}
	done := make(chan struct{})
	c.building = done
	go func() {
		for attempt := 0; attempt < 3; attempt++ {
			before, err := os.Stat(yamlPath)
			if err != nil {
				break
			}
			games := buildIndex(yamlPath)
			if len(games) == 0 {
				break
			}
			after, err := os.Stat(yamlPath)
			replaced := err != nil || !stampOf(after).same(stampOf(before))
			if !replaced {
				// Written only for the manifest it was built from: an index
				// file newer than a manifest is taken to be its index.
				if raw, err := json.Marshal(games); err == nil {
					_ = writeFileAtomic(indexPath, raw)
				}
			}
			c.mu.Lock()
			c.games, c.from = games, stampOf(before)
			c.mu.Unlock()
			if !replaced {
				break
			}
		}
		c.mu.Lock()
		c.building = nil
		c.mu.Unlock()
		close(done)
	}()
	return done
}

func readIndexFile(indexPath string) []indexedGame {
	raw, err := os.ReadFile(indexPath)
	if err != nil {
		return nil
	}
	var games []indexedGame
	if json.Unmarshal(raw, &games) != nil || len(games) == 0 {
		return nil
	}
	return games
}

// refreshInFlight guards against overlapping background downloads.
var refreshInFlight atomic.Bool

// refreshManifestAsync kicks off a background download when the local
// manifest is missing or older than a week. The current scan proceeds
// with whatever is available now; the next scan sees the fresh data.
func (sc *Scanner) refreshManifestAsync(yamlPath string) {
	url := sc.ManifestURL
	if url == "" {
		return
	}
	if info, err := os.Stat(yamlPath); err == nil && time.Since(info.ModTime()) < manifestMaxAge {
		return
	}
	if !refreshInFlight.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer refreshInFlight.Store(false)
		sc.downloadManifest(yamlPath)
	}()
}

// downloadManifest fetches the manifest. Best-effort: failures leave any
// existing copy in place.
func (sc *Scanner) downloadManifest(yamlPath string) {
	url := sc.ManifestURL
	if url == "" {
		return
	}

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}

	tmp := yamlPath + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return
	}
	_, copyErr := io.Copy(f, resp.Body)
	closeErr := f.Close()
	if copyErr != nil || closeErr != nil {
		os.Remove(tmp)
		return
	}
	// The index is built here, in the background, before the manifest takes
	// its place - not by the next caller that finds the manifest newer than
	// its index. That caller was whatever wanted the index next: a scan (the
	// app's own background scan, or "Scan saves"), or tracking any game (its
	// registry keys are looked up first). Parsing the 17.7 MB YAML held it up
	// for some thirty seconds, after every weekly download. The index is then
	// dated after the manifest it came from, so no one rebuilds it: the
	// manifest is dated a second before the index, as the clock can give the
	// two files the same time.
	_, indexPath := sc.manifestPaths()
	if games := buildManifestIndex(tmp); len(games) > 0 {
		if raw, err := json.Marshal(games); err == nil && writeFileAtomic(indexPath, raw) == nil {
			if info, err := os.Stat(indexPath); err == nil {
				older := info.ModTime().Add(-time.Second)
				_ = os.Chtimes(tmp, older, older)
			}
		}
	}
	_ = os.Remove(yamlPath)
	if os.Rename(tmp, yamlPath) != nil {
		return
	}
	// Indexed now, while nothing waits on it, rather than by the next check
	// made while a peer waits.
	sc.manifestIndex(-1)
}

// buildManifestIndex parses the manifest YAML and keeps only what a
// Windows save scan needs.
func buildManifestIndex(yamlPath string) []indexedGame {
	raw, err := os.ReadFile(yamlPath)
	if err != nil {
		return nil
	}
	var manifest map[string]manifestGame
	if err := yaml.Unmarshal(raw, &manifest); err != nil {
		return nil
	}

	games := make([]indexedGame, 0, len(manifest))
	for name, mg := range manifest {
		var paths, config []string
		var when map[string][]pathWhen
		for tpl, entry := range mg.Files {
			if entryIsConfigEntry(tpl, entry) {
				config = append(config, tpl)
			}
			if !entryIsSaveEntry(tpl, entry) {
				continue
			}
			paths = append(paths, tpl)
			if w := scanConditions(tpl, entry); w != nil {
				if when == nil {
					when = map[string][]pathWhen{}
				}
				when[tpl] = w
			}
		}
		sort.Strings(config)
		var regKeys []string
		for key, entry := range mg.Registry {
			if !registryEntryIsSave(key, entry) {
				continue
			}
			regKeys = append(regKeys, key)
		}
		// A game whose save lives only in the registry used to be dropped here
		// for having no file paths — 619 of them, invisible to every scan.
		//
		// A game with neither is kept too, when the manifest knows its Steam
		// id. It has nothing for a scan to look for — its saves are Steam
		// Cloud's business, which is why the manifest declares no paths — but
		// its name still maps to an App ID, and that mapping is what gets a
		// game its cover art. Dropping it cost 29,246 of the manifest's 48,946
		// Steam ids, so a game like Killer Bean was found by another pass,
		// correctly named, and left blank on the shelf.
		//
		// These carry no paths and no registry keys, so every scan pass skips
		// them by the checks it already makes.
		steamID := ""
		if mg.Steam.ID > 0 {
			steamID = strconv.FormatInt(mg.Steam.ID, 10)
		}
		if len(paths) == 0 && len(regKeys) == 0 && steamID == "" {
			continue
		}
		sort.Strings(regKeys) // stable index across rebuilds
		g := indexedGame{Name: name, Paths: paths, Registry: regKeys, SteamID: steamID, Config: config, When: when}
		for dir := range mg.InstallDir {
			g.Installs = append(g.Installs, dir)
		}
		games = append(games, g)
	}
	return games
}

// scanConditions returns an entry's conditions when pathsFor would act on
// them, else nil: most of the manifest's conditions change nothing a scan
// does, and the index is shipped in the binary.
func scanConditions(tpl string, entry manifestFileEntry) []pathWhen {
	var out []pathWhen
	for _, w := range entry.When {
		out = append(out, pathWhen{OS: w.OS, Store: w.Store})
	}
	if whenHasStore(out, "uplay") && strings.Contains(tpl, "<root>") {
		return out
	}
	if installRelative(tpl) || whenAllowsOS(out, "") {
		return nil
	}
	return out
}

// entryIsSaveEntry keeps manifest file entries that are save data and whose
// template can plausibly resolve on Windows or Linux (native XDG paths, or
// Windows paths that resolve inside a Proton prefix). Per-OS filtering of
// the survivors happens at scan time — templates with placeholders the
// current platform can't resolve are dropped there.
// registryEntryIsSave reports whether a manifest registry key holds save data.
//
// Unlike a file template there is no placeholder to resolve — a registry path
// is already absolute — so only the tags decide, by the same rule the file side
// uses: an entry tagged at all must be tagged "save", and an untagged entry
// counts.
func registryEntryIsSave(key string, entry manifestFileEntry) bool {
	if strings.TrimSpace(key) == "" {
		return false
	}
	if len(entry.Tags) == 0 {
		return true
	}
	for _, t := range entry.Tags {
		if t == "save" {
			return true
		}
	}
	return false
}

func entryIsSaveEntry(tpl string, entry manifestFileEntry) bool {
	if len(entry.Tags) > 0 && !hasTag(entry, "save") {
		return false
	}
	return entryResolvable(tpl, entry)
}

// entryIsConfigEntry keeps manifest file entries that are a game's settings
// and nothing else: tagged "config" and not "save". An entry carrying both
// is part of the save, whatever else it holds.
func entryIsConfigEntry(tpl string, entry manifestFileEntry) bool {
	if !hasTag(entry, "config") || hasTag(entry, "save") {
		return false
	}
	return entryResolvable(tpl, entry)
}

func hasTag(entry manifestFileEntry, tag string) bool {
	for _, t := range entry.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

// entryResolvable reports whether a file template can plausibly resolve on
// a platform this app runs on.
func entryResolvable(tpl string, entry manifestFileEntry) bool {
	// <dataDrive> is not among Ludusavi's placeholders (src/resource/
	// manifest.rs) nor in the manifest today; nothing says what it would be.
	if strings.Contains(tpl, "<dataDrive>") {
		return false
	}
	// Must reference a base we know how to expand (Windows, XDG, or home).
	resolvable := strings.Contains(tpl, "<win") ||
		strings.Contains(tpl, "<xdg") ||
		strings.Contains(tpl, "<home>") ||
		strings.Contains(tpl, "<base>") || strings.Contains(tpl, "<root>")
	if !resolvable {
		return false
	}

	if len(entry.When) == 0 {
		return true
	}
	// "mac" was missing here, which dropped every entry restricted to it at
	// index build time — before ludusaviVarSets, the runtime resolver, ever
	// got a chance to run. 3,181 file templates in the manifest are mac-only,
	// and every one of them was invisible on every platform, Windows and
	// Linux included, because the index simply never carried them.
	for _, w := range entry.When {
		if w.OS == "" || w.OS == "windows" || w.OS == "linux" || w.OS == "mac" {
			return true
		}
	}
	return false
}

// BuildEmbeddedIndexJSON builds the compact index from a manifest and returns
// it as JSON, for regenerating the copy embedded in the binary.
//
// The embedded index is what a fresh install scans against before any manifest
// download has finished, so it has to carry every field a scan reads. It fell
// behind once already: registry save keys were added to indexedGame and the
// embedded copy still held none, which left registry-only games undetectable
// until the first background refresh rebuilt the index.
func BuildEmbeddedIndexJSON(yamlPath string) ([]byte, error) {
	games := buildManifestIndex(yamlPath)
	if len(games) == 0 {
		return nil, fmt.Errorf("no games parsed from %s", yamlPath)
	}
	sort.Slice(games, func(i, j int) bool { return games[i].Name < games[j].Name })
	return json.Marshal(games)
}
