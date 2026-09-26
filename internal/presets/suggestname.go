package presets

import (
	"path/filepath"
	"strings"

	"github.com/opensave/opensave/internal/switchtitle"
)

// SuggestName is a name for a game whose save folder (or file) was picked by
// hand, for the person to confirm or change: the name a Switch emulator here
// knows the game by, or the nearest folder that names a game rather than a
// kind of folder — "Hollow Knight", not "Saves" or a Steam account's number.
// "" when nothing in the path says.
//
// A Switch save no emulator here can name yet gets the name a scan gives it,
// which is renamed by itself once one can (daemon/switchnames.go).
func (sc *Scanner) SuggestName(path string) string {
	if titleID := switchtitle.FromSavePath(path); titleID != "" {
		if name := sc.SwitchTitleName(titleID, path); name != "" {
			return name
		}
		return "Switch game - Title ID: " + titleID
	}
	parts := strings.FieldsFunc(path, func(r rune) bool { return r == '/' || r == '\\' })
	for i := len(parts) - 1; i >= 0; i-- {
		name := parts[i]
		if i == len(parts)-1 {
			if ext := filepath.Ext(name); ext != "" && i > 0 {
				// A single save file: its folder usually names the game.
				continue
			}
		}
		if namesAGame(name) {
			if appName, ok := popularSteamGames[name]; ok {
				return appName
			}
			return name
		}
		if isAppID(name) {
			if appName, ok := popularSteamGames[name]; ok {
				return appName
			}
		}
	}
	return ""
}

// notGameNames are folders on the way to a save that name the place, not the
// game.
var notGameNames = map[string]bool{
	"appdata": true, "roaming": true, "locallow": true, "documents": true,
	"my games": true, "mygames": true, "steam": true, "steamapps": true,
	"userdata": true, "common": true, "program files": true, "program files (x86)": true,
	"users": true, "home": true, "wine": true, "pfx": true, "drive_c": true,
	"nand": true, "sdmc": true, "bis": true, "cache": true, ".local": true, "share": true, ".config": true,
}

func namesAGame(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	switch {
	case len([]rune(lower)) <= 2 || strings.HasSuffix(lower, ":"): // a drive, or a code ("b1")
		return false
	case isAppID(name) || switchtitle.Valid(name) || isHexID(name):
		return false
	case notGameNames[lower] || genericNames[groupNameKey(name)] || looksLikeSaveDirName(name) || isCacheDirName(name):
		return false
	}
	return true
}

// isHexID is an id rather than a name: a profile or account folder, 16 or
// more hex digits.
func isHexID(name string) bool {
	return len(name) >= 16 && isHex(strings.ReplaceAll(name, "-", ""))
}
