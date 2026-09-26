package presets

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/opensave/opensave/internal/switchtitle"
)

// Naming a Switch game from its title id.
//
// A Switch save is found as a folder named for its game's title id —
// 0100F2C0115B6000 — which tells nobody anything. The emulators that made
// those folders know what each one is: they read the name and icon out of the
// game itself to draw their game list, and keep a copy on disk so the list
// opens quickly. Those copies are what this reads. They are exact, they are in
// the language the emulator shows the game in, and nothing is asked of the
// network.
//
//   - The yuzu family (yuzu, Suyu, Sudachi, Citron, Eden) keeps them in its
//     cache folder's game_list/: <TITLEID>.appname.txt and <TITLEID>.jpeg for
//     a game installed to its NAND, and — in Citron and Eden — every game of a
//     ROM folder in game_metadata_cache.json, the icon base64 inside it.
//   - Ryujinx and Ryubing keep games/<titleid>/gui/metadata.json, whose
//     "title" is the game's name. There is no icon in it.
//
// The cache folder is the emulator's own: %APPDATA%\<emu>\cache on Windows,
// <emu folder>\user\cache for a portable copy, and $XDG_CACHE_HOME/<emu> on
// Linux — inside ~/.var/app/<id>/cache for a Flatpak.
//
// A game none of them has listed keeps the name it was found under.

// SwitchTitleName is the name the Switch emulators on this device know a
// title by, or "" when none of them has it. savePath, when given, is the
// title's save folder: it leads to the emulator that made the save, which is
// asked first, since its name is the one the person sees.
func (sc *Scanner) SwitchTitleName(titleID, savePath string) string {
	num, ok := titleNumber(titleID)
	if !ok {
		return ""
	}
	lists, ryujinx := sc.switchSources(savePath)
	for _, dir := range lists {
		if name := appNameFile(dir, titleID); name != "" {
			return name
		}
		if e, ok := readMetadataCache(filepath.Join(dir, metadataCacheName))[num]; ok {
			if name := cleanTitle(e.Title); name != "" {
				return name
			}
		}
	}
	for _, dir := range ryujinx {
		if name := ryujinxTitle(dir, titleID); name != "" {
			return name
		}
	}
	return ""
}

// SwitchTitleIcon is a title's icon as an emulator here cached it — a JPEG,
// square — or nil.
func (sc *Scanner) SwitchTitleIcon(titleID, savePath string) []byte {
	num, ok := titleNumber(titleID)
	if !ok {
		return nil
	}
	lists, _ := sc.switchSources(savePath)
	for _, dir := range lists {
		for _, name := range []string{strings.ToUpper(titleID), strings.ToLower(titleID)} {
			if b, err := os.ReadFile(filepath.Join(dir, name+".jpeg")); err == nil && len(b) > 0 {
				return b
			}
		}
		if e, ok := readMetadataCache(filepath.Join(dir, metadataCacheName))[num]; ok && e.Icon != "" {
			if b, err := base64.StdEncoding.DecodeString(e.Icon); err == nil && len(b) > 0 {
				return b
			}
		}
	}
	return nil
}

// nameSwitchTitles gives each Switch title found by a scan the name an
// emulator here knows it by, in place.
func (sc *Scanner) nameSwitchTitles(found []DiscoveredSave) {
	for i := range found {
		if found[i].TitleID == "" {
			continue
		}
		if name := sc.SwitchTitleName(found[i].TitleID, found[i].SavePath); name != "" {
			found[i].Name = name
		}
	}
}

const metadataCacheName = "game_metadata_cache.json"

// switchSources lists where to look for a title: the yuzu family's
// game_list folders, then Ryujinx's games folders. The emulator the save
// folder belongs to comes first, then every Switch emulator this device has.
func (sc *Scanner) switchSources(savePath string) (lists, ryujinx []string) {
	seen := map[string]bool{}
	add := func(dst *[]string, dir string) {
		if dir != "" && !seen[dir] {
			seen[dir] = true
			*dst = append(*dst, dir)
		}
	}
	if savePath != "" && switchtitle.FromSavePath(savePath) != "" {
		// title -> profile -> account -> the save root.
		root := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Clean(savePath))))
		if base := emulatorBase(root, "nand"); base != "" {
			for _, dir := range sc.gameListDirs(base) {
				add(&lists, dir)
			}
		}
	}
	for _, p := range presetDefs {
		for _, root := range p.resolvedPaths(sc) {
			if base := emulatorBase(root, "nand"); base != "" {
				for _, dir := range sc.gameListDirs(base) {
					add(&lists, dir)
				}
			} else if base := emulatorBase(root, "bis"); base != "" {
				add(&ryujinx, filepath.Join(base, "games"))
			}
		}
	}
	return lists, ryujinx
}

// emulatorBase is the emulator's own folder for a save root ending in
// <first>/user/save — nand/user/save for the yuzu family, bis/user/save for
// Ryujinx — or "" for any other.
func emulatorBase(saveRoot, first string) string {
	save := filepath.Clean(saveRoot)
	user := filepath.Dir(save)
	nand := filepath.Dir(user)
	if !strings.EqualFold(filepath.Base(save), "save") || !strings.EqualFold(filepath.Base(user), "user") ||
		!strings.EqualFold(filepath.Base(nand), first) {
		return ""
	}
	return filepath.Dir(nand)
}

// gameListDirs is where a yuzu-family emulator whose folder is base keeps its
// game list cache.
func (sc *Scanner) gameListDirs(base string) []string {
	out := []string{filepath.Join(base, "cache", "game_list")} // Windows, and a portable copy anywhere
	if sc.goos() != "windows" {
		name := filepath.Base(base)
		parent := filepath.Dir(base)
		if filepath.Base(parent) == "data" { // Flatpak: ~/.var/app/<id>/data/<emu>
			out = append(out, filepath.Join(filepath.Dir(parent), "cache", name, "game_list"))
		}
		out = append(out, filepath.Join(sc.xdgCacheHome(), name, "game_list"))
	}
	return out
}

func (sc *Scanner) xdgCacheHome() string {
	if sc.HomeDir == "" {
		if v := os.Getenv("XDG_CACHE_HOME"); v != "" {
			return v
		}
	}
	return filepath.Join(sc.linuxHome(), ".cache")
}

func titleNumber(titleID string) (uint64, bool) {
	if !switchtitle.Valid(titleID) {
		return 0, false
	}
	n, err := strconv.ParseUint(titleID, 16, 64)
	return n, err == nil
}

// cleanTitle is a name as it can be shown, or "" for one that cannot: the
// emulators write " " for a game whose name they could not read.
func cleanTitle(s string) string {
	s = strings.TrimRight(s, "\x00")
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	if !utf8.ValidString(s) {
		return ""
	}
	if utf8.RuneCountInString(s) > 200 {
		s = string([]rune(s)[:200])
	}
	return s
}

func appNameFile(dir, titleID string) string {
	for _, name := range []string{strings.ToUpper(titleID), strings.ToLower(titleID)} {
		if b, err := os.ReadFile(filepath.Join(dir, name+".appname.txt")); err == nil {
			if t := cleanTitle(string(b)); t != "" {
				return t
			}
		}
	}
	return ""
}

func ryujinxTitle(gamesDir, titleID string) string {
	for _, name := range []string{strings.ToLower(titleID), strings.ToUpper(titleID)} {
		b, err := os.ReadFile(filepath.Join(gamesDir, name, "gui", "metadata.json"))
		if err != nil {
			continue
		}
		var meta struct {
			Title string `json:"title"`
		}
		if json.Unmarshal(b, &meta) == nil {
			if t := cleanTitle(meta.Title); t != "" {
				return t
			}
		}
	}
	return ""
}

// metadataEntry is one game in Citron's and Eden's game_metadata_cache.json.
type metadataEntry struct {
	Title string
	Icon  string // base64
}

// metadataCaches holds each game_metadata_cache.json read, for as long as the
// file is unchanged: a scan, or a screen of covers, asks it about every title
// at once, and the file carries every game's icon.
var metadataCaches = struct {
	sync.Mutex
	m map[string]metadataFile
}{m: map[string]metadataFile{}}

type metadataFile struct {
	size    int64
	modTime time.Time
	byID    map[uint64]metadataEntry
}

func readMetadataCache(path string) map[uint64]metadataEntry {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return nil
	}
	metadataCaches.Lock()
	f, ok := metadataCaches.m[path]
	metadataCaches.Unlock()
	if ok && f.size == info.Size() && f.modTime.Equal(info.ModTime()) {
		return f.byID
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var doc struct {
		Entries []struct {
			ProgramID json.RawMessage `json:"program_id"`
			Title     string          `json:"title"`
			Icon      string          `json:"icon"`
		} `json:"entries"`
	}
	byID := map[uint64]metadataEntry{}
	if json.Unmarshal(raw, &doc) == nil {
		for _, e := range doc.Entries {
			if id, ok := programID(e.ProgramID); ok {
				byID[id] = metadataEntry{Title: e.Title, Icon: e.Icon}
			}
		}
	}
	metadataCaches.Lock()
	metadataCaches.m[path] = metadataFile{size: info.Size(), modTime: info.ModTime(), byID: byID}
	metadataCaches.Unlock()
	return byID
}

// programID reads a program id as the file has it: a hex string, which
// Citron writes without its leading zero ("100f2c0115b6000"), or a number.
func programID(raw json.RawMessage) (uint64, bool) {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		n, err := strconv.ParseUint(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(s)), "0x"), 16, 64)
		return n, err == nil && n != 0
	}
	var n uint64
	if json.Unmarshal(raw, &n) == nil {
		return n, n != 0
	}
	return 0, false
}
