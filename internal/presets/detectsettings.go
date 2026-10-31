package presets

import (
	"bytes"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf16"
)

// Detecting a game's settings files by what they hold, for games the game
// database does not cover (DeviceSettings covers the ones it does).
//
// What tells a device's settings from a save is its contents: resolution,
// vsync, shadow quality, upscaler — the vocabulary of one machine's display,
// which no save has a use for. Names were measured against the Ludusavi
// manifest's own labels (16,700 settings entries, 9,000 save entries) and do
// not tell them apart on their own: "settings" names 1,521 settings files and
// 70 saves, ".ini" 1,831 and 137, ".json" 359 and 216. So a name only adds
// weight to what the contents say, by how reliably the manifest shows it to
// mean settings; a word that names saves rules a file out.
//
// What this misses stays syncing, which is how every file synced before; what
// it wrongly takes is caught by how a save behaves — it changes with every
// session — and given back (daemon/detectsettings.go).

// DetectedSettings is one file found to be a game's settings.
type DetectedSettings struct {
	Path   string // relative to the save folder, slash-separated
	Reason string
}

const (
	detectMaxSize  = 256 << 10
	detectMaxFiles = 4000 // text-like files read per folder, at most
)

var detectExts = map[string]bool{
	"ini": true, "cfg": true, "conf": true, "config": true, "xml": true, "json": true, "lua": true,
	"txt": true, "yaml": true, "yml": true, "toml": true, "vdf": true, "set": true, "settings": true,
	"opt": true, "prefs": true, "local": true, "properties": true,
}

// Terms of a machine's display and graphics, as sequences of words. Matched
// in identifiers split at case changes and separators, so
// "_enableFrameGeneration", "r_materialAniso" and "Shadow Quality" all count.
var strongTerms = [][]string{
	{"resolution"}, {"fullscreen"}, {"windowed"}, {"borderless"}, {"vsync"}, {"v", "sync"},
	{"antialiasing"}, {"anti", "aliasing"}, {"aniso"}, {"anisotropic"}, {"anisotropy"},
	{"msaa"}, {"fxaa"}, {"taa"}, {"smaa"}, {"ssao"}, {"hbao"}, {"ssr"}, {"bloom"}, {"gamma"},
	{"brightness"}, {"hdr"}, {"dlss"}, {"fsr"}, {"xess"}, {"upscaling"}, {"upscaler"},
	{"monitor"}, {"adapter"}, {"gpu"}, {"vram"}, {"tessellation"}, {"raytracing"},
	{"ray", "tracing"}, {"fov"}, {"field", "view"}, {"lod"}, {"framerate"}, {"frame", "rate"},
	{"refreshrate"}, {"refresh", "rate"}, {"frame", "generation"}, {"frame", "limit"},
	{"fps", "limit"}, {"max", "fps"}, {"texture", "quality"}, {"texture", "filtering"},
	{"shadow", "quality"}, {"shadow", "resolution"}, {"motion", "blur"}, {"depth", "field"},
	{"view", "distance"}, {"draw", "distance"}, {"render", "scale"}, {"resolution", "scale"},
	{"display", "mode"}, {"window", "mode"}, {"screen", "mode"}, {"ambient", "occlusion"},
	{"screen", "width"}, {"screen", "height"}, {"quality", "preset"}, {"graphics", "quality"},
	{"film", "grain"}, {"lens", "flare"}, {"volumetric"}, {"scalability"}, {"supersampling"},
	{"sharpening"}, {"chromatic", "aberration"}, {"vignette"}, {"directx"}, {"vulkan"},
	{"dx11"}, {"dx12"}, {"d3d11"}, {"d3d12"},
}

var weakTerms = []string{
	"width", "height", "quality", "preset", "driver", "spec", "shadows", "shadow", "texture",
	"textures", "video", "display", "graphics", "render", "screen", "fps", "sync", "filtering",
}

// Words in a file name, weighted by how reliably the Ludusavi manifest shows
// them to mean settings rather than a save.
var nameWeights = map[string]float64{
	"options": 1.5, "option": 1.5, "graphics": 1.5, "graphic": 1.5, "video": 1.5, "display": 1.5,
	"render": 1.5, "engine": 1.5, "scalability": 1.5, "gameusersettings": 1.5, "screen": 1,
	"settings": 0.5, "setting": 0.5, "config": 0.5, "configuration": 0.5, "cfg": 0.5,
	"prefs": 0.5, "preferences": 0.5,
}

// Words that name saves somewhere in the manifest often enough that a file
// carrying one is never taken, whatever it holds. Logs are not settings
// either, though leaving them out would harm nothing.
var saveWords = map[string]bool{
	"save": true, "saves": true, "savegame": true, "savedata": true, "slot": true, "progress": true,
	"autosave": true, "quicksave": true, "world": true, "player": true, "character": true,
	"campaign": true, "level": true, "levels": true, "achievement": true, "achievements": true,
	"achievments": true, "stats": true, "unlock": true, "unlocks": true, "inventory": true,
	"profile": true, "career": true, "highscore": true, "highscores": true, "score": true,
	"scores": true, "records": true, "besttimes": true, "gamedata": true, "log": true,
	"logs": true, "crash": true, "output": true, "history": true,
}

var identRe = regexp.MustCompile(`[A-Za-z][A-Za-z0-9]*`)
var camelRe = regexp.MustCompile(`[A-Z]?[a-z]+|[A-Z]+(?:[^a-z]|$)|[0-9]+`)

// words splits text into lowercase words: identifiers, then at case changes
// ("FrameGeneration" → frame generation), keeping a whole short identifier
// too ("dx11", "vsync").
func words(text string) []string {
	var out []string
	for _, id := range identRe.FindAllString(text, -1) {
		parts := camelRe.FindAllString(id, -1)
		low := strings.ToLower(id)
		if len(parts) > 1 && len(low) <= 6 {
			out = append(out, low)
		}
		for _, p := range parts {
			p = strings.ToLower(p)
			if p == "of" || p == "the" || p == "b" || p == "r" || p == "e" {
				continue // filler, and engine prefixes like r_ and bEnable
			}
			out = append(out, p)
		}
	}
	return out
}

// textOf returns a file's text, decoding UTF-16, or false for binary.
func textOf(b []byte) (string, bool) {
	switch {
	case bytes.HasPrefix(b, []byte{0xFF, 0xFE}), bytes.HasPrefix(b, []byte{0xFE, 0xFF}):
		big := b[0] == 0xFE
		b = b[2:]
		u := make([]uint16, len(b)/2)
		for i := range u {
			if big {
				u[i] = uint16(b[2*i])<<8 | uint16(b[2*i+1])
			} else {
				u[i] = uint16(b[2*i+1])<<8 | uint16(b[2*i])
			}
		}
		return string(utf16.Decode(u)), true
	case len(b) >= 4 && b[1] == 0 && b[3] == 0 && b[0] != 0 && b[2] != 0:
		// UTF-16LE without a mark, as some engines write their XML.
		u := make([]uint16, len(b)/2)
		for i := range u {
			u[i] = uint16(b[2*i+1])<<8 | uint16(b[2*i])
		}
		return string(utf16.Decode(u)), true
	case bytes.IndexByte(b, 0) >= 0:
		return "", false
	}
	return string(b), true
}

// contentScore counts the distinct display terms a text holds.
func contentScore(text string) (strong int, score float64) {
	ws := words(text)
	present := make(map[string]bool, len(ws))
	pairs := make(map[[2]string]bool, len(ws))
	for i, w := range ws {
		present[w] = true
		if i+1 < len(ws) {
			pairs[[2]string{w, ws[i+1]}] = true
		}
	}
	for _, t := range strongTerms {
		if (len(t) == 1 && present[t[0]]) || (len(t) == 2 && pairs[[2]string{t[0], t[1]}]) {
			strong++
		}
	}
	weak := 0.0
	for _, w := range weakTerms {
		if present[w] {
			weak += 0.5
		}
	}
	if weak > 2 {
		weak = 2
	}
	return strong, float64(strong) + weak
}

// nameSignal weighs a file's name: the support it lends, and whether a word
// in it names saves.
func nameSignal(rel string) (weight float64, saveWord, strongName bool) {
	base := path.Base(rel)
	ext := strings.ToLower(strings.TrimPrefix(path.Ext(base), "."))
	stem := strings.TrimSuffix(base, path.Ext(base))
	ws := words(stem)
	low := strings.ToLower(stem)
	for w := range nameWeights {
		if len(w) >= 6 && strings.Contains(low, w) {
			ws = append(ws, w)
		}
	}
	seen := map[string]bool{}
	for _, w := range ws {
		if seen[w] {
			continue
		}
		seen[w] = true
		if saveWords[w] {
			saveWord = true
		}
		weight += nameWeights[w]
		if nameWeights[w] >= 1.5 {
			strongName = true
		}
	}
	if strings.Contains(low, "save") || strings.Contains(low, "slot") || ext == "log" {
		saveWord = true
	}
	if ext == "ini" || ext == "cfg" || ext == "conf" {
		weight += 0.5
	}
	if weight > 2 {
		weight = 2
	}
	return weight, saveWord, strongName
}

// unrealSettings reports a file in an Unreal Engine game's settings folder,
// Saved/Config/<platform>: the engine writes a machine's settings there and
// nothing else, often as files too short to judge by content.
func unrealSettings(rel string) bool {
	segs := strings.Split(strings.ToLower(rel), "/")
	if len(segs) < 2 || !strings.HasSuffix(segs[len(segs)-1], ".ini") {
		return false
	}
	for i := 0; i+1 < len(segs)-1; i++ {
		if segs[i] == "config" && (strings.HasPrefix(segs[i+1], "windows") || strings.HasPrefix(segs[i+1], "linux") ||
			strings.HasPrefix(segs[i+1], "mac")) {
			return true
		}
	}
	return false
}

// judgeSettings decides one file from its path and bytes.
func judgeSettings(rel string, b []byte) (bool, string) {
	weight, saveWord, strongName := nameSignal(rel)
	if saveWord && !strongName {
		return false, ""
	}
	if unrealSettings(rel) && !saveWord {
		return true, "Unreal Engine settings folder"
	}
	text, ok := textOf(b)
	if !ok {
		return false, ""
	}
	strong, score := contentScore(text)
	switch {
	case saveWord:
		// "user_engine_option_save.xml": a strong settings word and a save
		// word together. Taken only on overwhelming contents.
		if strong >= 6 && score+weight >= 8 {
			return true, "display settings in its contents"
		}
	case len(b) > 32<<10:
		// A long file holds a lot besides; a save with a settings section is
		// long. Only a file that is mostly settings is taken.
		if strong >= 6 {
			return true, "display settings in its contents"
		}
	case strong >= 3 && score+weight >= 4:
		return true, "display settings in its contents"
	case strong >= 2 && weight >= 1.5 && score+weight >= 4:
		return true, "display settings in its contents, and its name"
	}
	return false, ""
}

// DetectSettings finds the settings files in root by what they hold. The
// game's save locations in the game database (by name or App ID) are never
// taken, whatever they hold.
func (ds *DeviceSettings) DetectSettings(name, appID, root string) []DetectedSettings {
	var savePats []string
	if ds != nil {
		g, ok := ds.byID[appID]
		if !ok || appID == "" {
			g, ok = ds.byName[normalizeGameName(stripNameSuffixes(name))]
		}
		if ok {
			for _, vars := range ds.sc.ludusaviVarSets(g, ds.protonIx) {
				for _, tpl := range g.Paths {
					for _, expanded := range expandTemplate(tpl, vars, ds.bases(g.Installs), g.SteamID) {
						if rel, ok := relativeUnder(root, expanded); ok {
							savePats = append(savePats, rel)
						}
					}
				}
			}
		}
	}
	return detectIn(root, savePats)
}

// isSaveLocation reports whether rel is, or is inside, one of the game
// database's save locations (relative patterns). A location naming a folder
// covers what is in it; one with a file extension names that file.
func isSaveLocation(rel string, savePats []string) bool {
	segs := strings.Split(strings.ToLower(rel), "/")
	for _, p := range savePats {
		ps := strings.Split(p, "/")
		if len(ps) > len(segs) {
			continue
		}
		match := true
		for i, pc := range ps {
			if ok, err := path.Match(pc, segs[i]); err != nil || !ok {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		if len(ps) < len(segs) || path.Ext(ps[len(ps)-1]) != "" {
			return true
		}
	}
	return false
}

func detectIn(root string, savePats []string) []DetectedSettings {
	var out []DetectedSettings
	read := 0
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != root && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(p), "."))
		if !detectExts[ext] || read >= detectMaxFiles {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if strings.Contains(strings.ToLower(rel), ".sync-conflict-") || isSaveLocation(rel, savePats) {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() > detectMaxSize {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		read++
		if ok, reason := judgeSettings(rel, b); ok {
			out = append(out, DetectedSettings{Path: rel, Reason: reason})
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}
