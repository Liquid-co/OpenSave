package store

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxGameNameLength is the longest name a game can be given, in characters.
// Long enough for any real title with an edition after it; short enough that
// it still fits a tile, a menu and a notification.
const MaxGameNameLength = 120

// CleanGameName is the name a person typed for a game, as it is kept: with
// the spaces around it trimmed, and refused when it is empty, too long, or
// holds a control character (a newline from a paste, say), which no screen
// that shows a name can draw sensibly.
//
// A game's name is what this device calls it, and nothing more. Its ID, which
// sync, snapshots and cloud backups all go by, is never changed by a rename,
// so another device can call the same game something else.
func CleanGameName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("a game's name cannot be empty")
	}
	if n := utf8.RuneCountInString(name); n > MaxGameNameLength {
		return "", fmt.Errorf("a game's name can be at most %d characters (this one is %d)", MaxGameNameLength, n)
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return "", errors.New("a game's name cannot contain line breaks or other control characters")
		}
	}
	return name, nil
}
