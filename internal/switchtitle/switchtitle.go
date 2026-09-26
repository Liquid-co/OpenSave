// Package switchtitle recognises a Nintendo Switch game by its title id
// wherever it turns up: in the path of its save folder, and in the id it was
// tracked under — on this device or on another one.
//
// The title id is what makes a Switch save the same game everywhere. The
// emulators all keep a game's save in a folder named for it, whatever else
// differs between them: which emulator it is, the profile id each install
// generates for itself, the language names are shown in, and whether the
// emulator knows the game's name at all. So a Switch game is tracked under an
// id made from its title id (GameID), and a game tracked before that — under
// the id earlier versions made from the name they gave it — is still known
// by the title id inside that id (FromGameID).
package switchtitle

import (
	"regexp"
	"strings"
)

// Valid reports whether s is a title id: 16 hex digits.
func Valid(s string) bool {
	if len(s) != 16 {
		return false
	}
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}

// FromSavePath returns the title id of a save folder in the NAND layout the
// yuzu family of emulators keeps — …/save/<account>/<profile>/<title id> —
// in upper case, or "" for any other folder.
//
// Either separator is accepted, since the path may be another device's, from
// another system: a Windows path arriving on a Steam Deck is still read.
func FromSavePath(p string) string {
	parts := strings.FieldsFunc(p, func(r rune) bool { return r == '/' || r == '\\' })
	if len(parts) < 4 {
		return ""
	}
	id := parts[len(parts)-1]
	if !Valid(id) || !strings.EqualFold(parts[len(parts)-4], "save") {
		return ""
	}
	return strings.ToUpper(id)
}

// GameID is the id a Switch game is tracked under: "switch-" and its title
// id in lower case, which is what makes it the same game on every device.
func GameID(titleID string) string {
	return "switch-" + strings.ToLower(titleID)
}

// gameIDRe matches the ids a Switch game has been tracked under: GameID's,
// and the one earlier versions derived from the name they gave these games
// ("Citron Switch Emulator - Title ID: 0100…" became
// "citron-switch-emulator-title-id-0100…"). Either may carry the "-2" a
// second copy on one device is given.
var gameIDRe = regexp.MustCompile(`^(?:switch|[a-z0-9-]+-title-id)-([0-9a-f]{16})(?:-\d+)?$`)

// FromGameID returns the title id a game id carries, in upper case, or "".
func FromGameID(id string) string {
	m := gameIDRe.FindStringSubmatch(id)
	if m == nil {
		return ""
	}
	return strings.ToUpper(m[1])
}

// Of is the title id of a tracked game, from its save folder or its id.
func Of(gameID, savePath string) string {
	if id := FromSavePath(savePath); id != "" {
		return id
	}
	return FromGameID(gameID)
}
