package delta

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// IsSafePath reports whether relPath, joined onto baseDir, stays inside
// baseDir, both as written and as it is on disk.
//
// As written: a path that climbs out ("../../etc/passwd") is refused, the
// same way delta.js's isSafePath() did.
//
// On disk: a path that passes through a symbolic link or a Windows junction
// below baseDir is refused too. Checked as text alone, "saves/link/x.sav"
// stays inside the save folder, but if "link" points elsewhere, the write,
// the read or the deletion happens wherever it points — and every caller
// here acts on a path another device named. Nothing behind such a link is
// ever legitimately synced: building a manifest skips links of both kinds,
// so a link that is in the way only ever leads out. baseDir itself may be a
// link (a save folder kept on another drive): only what is below it is
// looked at.
//
// The parts of the path that exist are looked at as they are now; one that
// does not exist yet is created by the caller as a plain folder. What this
// does not stop is a link put in place between this check and the caller's
// write, which needs someone already able to write into the save folder.
func IsSafePath(baseDir, relPath string) bool {
	base, err := filepath.Abs(baseDir)
	if err != nil {
		return false
	}
	target, err := filepath.Abs(filepath.Join(base, relPath))
	if err != nil {
		return false
	}

	base = filepath.Clean(base)
	target = filepath.Clean(target)

	if target == base {
		return true
	}
	if !strings.HasPrefix(target, base+string(os.PathSeparator)) {
		return false
	}
	if throughLink(base, target) {
		return false
	}
	// Callers act on the spelling this disk holds (LocalNameFor), which for
	// a name with accents can differ from the one asked for — a link stored
	// decomposed, under an ask made composed, is the same link.
	if rel, err := filepath.Rel(base, target); err == nil && !isASCII(rel) {
		if local := filepath.Clean(LocalNameFor(base, filepath.ToSlash(rel))); local != target && throughLink(base, local) {
			return false
		}
	}
	return true
}

// throughLink reports whether any part of target below base is a symbolic
// link or a junction (which Go reports as ModeIrregular on Windows), stopping
// at the first part that does not exist.
func throughLink(base, target string) bool {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return true
	}
	at := base
	for _, part := range strings.Split(rel, string(os.PathSeparator)) {
		if part == "" || part == "." {
			continue
		}
		at = filepath.Join(at, part)
		info, err := os.Lstat(at)
		if err != nil {
			// Not there (yet), or not readable: nothing below it exists to
			// lead anywhere, and a caller that writes creates it plainly.
			return false
		}
		if info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			return true
		}
	}
	return false
}

// ResolveLocalSaveFilePath inspects a game's configured save path and
// reports whether it should be treated as a single-file save (isFile=true,
// in which case the manifest/watcher/patch logic operates on one file
// keyed by its base name) or a directory tree. A path that does not exist
// yet (game never launched) defaults to directory mode, since it will be
// created as a folder on first sync/snapshot the same way the JS app
// assumes a directory when it can't stat the configured path.
func ResolveLocalSaveFilePath(savePath string) (isFile bool, err error) {
	info, statErr := os.Stat(savePath)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return false, nil
		}
		return false, fmt.Errorf("stat save path: %w", statErr)
	}
	return !info.IsDir(), nil
}
