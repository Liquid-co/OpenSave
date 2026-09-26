package presets

import (
	"path/filepath"
	"strings"

	"github.com/opensave/opensave/internal/switchtitle"
)

// Where a Switch save arriving from another device belongs on this one.
//
// A game a paired device syncs, and this one does not track yet, is tracked
// here at the other device's save path translated to this device. For most
// games that is right. For a Switch save it never is: the path holds the
// other install's profile id, which this device's emulator does not have, so
// the save was written somewhere the emulator never looks — beside the real
// one, which then went unsynced. And when the other device plays the game in
// a different emulator, the translated path names an emulator that may not
// be here at all.
//
// So a Switch save goes where this device's own emulator keeps that game:
//
//  1. the translated folder, when it exists here — nothing to correct;
//  2. otherwise the game's own save folder in an emulator here, when there is
//     exactly one;
//  3. otherwise a new folder for it under the one profile of the emulator the
//     other device used, when it is here, or else of the only Switch emulator
//     here.
//
// Anything less certain than that — two emulators with the game, or several
// profiles — keeps the translated path, as before: picking between two
// people's profiles is not a guess to make on someone's behalf.

// SwitchSaveFolder picks the folder for titleID's save on this device, given
// the other device's save path translated to this one. Returns translated
// when nothing better is certain.
func (sc *Scanner) SwitchSaveFolder(titleID, translated string) string {
	if !switchtitle.Valid(titleID) {
		return translated
	}
	if dirExists(translated) {
		return translated
	}

	// The save roots to consider: the one the translated path is in, when
	// this device has that emulator, then every Switch emulator here.
	var roots []string
	seen := map[string]bool{}
	addRoot := func(root string) {
		root = filepath.Clean(root)
		if !seen[strings.ToLower(root)] && dirExists(root) {
			seen[strings.ToLower(root)] = true
			roots = append(roots, root)
		}
	}
	sameEmulator := false
	if switchtitle.FromSavePath(translated) != "" {
		addRoot(filepath.Dir(filepath.Dir(filepath.Dir(translated))))
		sameEmulator = len(roots) == 1
	}
	for _, p := range presetDefs {
		if !p.SwitchNAND {
			continue
		}
		for _, root := range p.resolvedPaths(sc) {
			addRoot(root)
		}
	}

	// 2. The game's own folder, where exactly one emulator here has it.
	var existing []string
	for _, root := range roots {
		existing = append(existing, titleFolders(root, titleID)...)
	}
	switch {
	case len(existing) == 1:
		return existing[0]
	case len(existing) > 1:
		return translated
	}

	// 3. A new folder: under the emulator the other device used when it is
	// here, or else under the only Switch emulator here — either way, only
	// when it has exactly one profile.
	var root string
	switch {
	case sameEmulator:
		root = roots[0]
	case len(roots) == 1:
		root = roots[0]
	default:
		return translated
	}
	if profile := onlyProfile(root); profile != "" {
		return filepath.Join(profile, titleID)
	}
	return translated
}

// titleFolders is every folder titleID's save has under a NAND save root,
// whatever the account and profile.
func titleFolders(root, titleID string) []string {
	var out []string
	for _, account := range listSubdirs(root) {
		for _, profile := range listSubdirs(filepath.Join(root, account)) {
			for _, title := range listSubdirs(filepath.Join(root, account, profile)) {
				if strings.EqualFold(title, titleID) {
					out = append(out, filepath.Join(root, account, profile, title))
				}
			}
		}
	}
	return out
}

// userAccount is the account folder the yuzu family keeps saves under.
const userAccount = "0000000000000000"

// onlyProfile is the profile folder of a NAND save root whose user account
// has exactly one profile, or "". The all-zero "profile" is where games that
// keep a save per console rather than per player put it, and is not one.
func onlyProfile(root string) string {
	var profiles []string
	for _, p := range listSubdirs(filepath.Join(root, userAccount)) {
		if len(p) == 32 && strings.Trim(p, "0") != "" && isHex(p) {
			profiles = append(profiles, filepath.Join(root, userAccount, p))
		}
	}
	if len(profiles) != 1 {
		return ""
	}
	return profiles[0]
}

func isHex(s string) bool {
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}
