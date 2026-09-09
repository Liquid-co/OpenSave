package delta

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// The same filename, spelled two ways.
//
// macOS stores filenames decomposed: "café.sav" is c,a,f,e,U+0301. Windows
// and Linux store it composed, as c,a,f,U+00E9. Neither NTFS nor ext4
// normalises on lookup, so the two spellings are two different files. Verified
// on Windows 11: writing both produced two entries in one directory, both
// rendering as "café.sav".
//
// Left alone, a macOS peer and a Windows peer each see the other's spelling as
// a file they are missing, pull it, and end up with a duplicate save that
// looks identical on screen. Neither side ever converges, and every sync adds
// the pair back.
//
// Manifest keys are therefore composed (NFC) before they go into a manifest,
// giving both machines one agreed spelling to compare. NFC is deliberate
// rather than arbitrary: Windows and Linux filenames are already composed in
// practice, so normalising is a no-op for every save that exists today and
// changes no manifest hash. Only a macOS peer's decomposed names move, and
// macOS is new enough here that there is nothing to migrate.
//
// The agreed spelling is for MATCHING. It is not necessarily what the local
// disk calls the file, which is what LocalNameFor is for.

// NormalizeRelPath returns the agreed (composed) spelling of a manifest key.
func NormalizeRelPath(relPath string) string {
	if relPath == "" || isASCII(relPath) {
		// The overwhelmingly common case, and norm.NFC on ASCII is a copy
		// that always returns the input unchanged.
		return relPath
	}
	return norm.NFC.String(relPath)
}

// isASCII avoids the normaliser entirely for names that cannot differ.
func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// LocalNameFor turns an agreed manifest key into the path to use on this
// machine, under root.
//
// A file the local filesystem already holds under a different normalisation
// has to be UPDATED, not duplicated: writing the composed spelling beside an
// existing decomposed one is precisely the duplicate this is here to prevent.
// So when the exact path is absent, the containing directory is scanned for an
// entry whose name normalises the same, and that entry's real spelling wins.
//
// When nothing matches, the key is returned as given — a genuinely new file is
// created under the agreed spelling.
func LocalNameFor(root, relPath string) string {
	exact := filepath.Join(root, filepath.FromSlash(relPath))
	if isASCII(relPath) {
		return exact
	}
	if _, err := os.Lstat(exact); err == nil {
		return exact
	}

	// Resolve one component at a time: a decomposed directory name is just as
	// capable of splitting a save in two as a decomposed file name.
	current := root
	parts := strings.Split(relPath, "/")
	for i, part := range parts {
		candidate := filepath.Join(current, part)
		if _, err := os.Lstat(candidate); err == nil {
			current = candidate
			continue
		}
		if match := entryMatchingNormalized(current, part); match != "" {
			current = filepath.Join(current, match)
			continue
		}
		// Nothing on disk matches this component, so neither will anything
		// below it. The rest is created under the agreed spelling.
		return filepath.Join(append([]string{current}, parts[i:]...)...)
	}
	return current
}

// entryMatchingNormalized finds an entry of dir whose name normalises to the
// same string as want, returning its real on-disk name.
func entryMatchingNormalized(dir, want string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	wanted := NormalizeRelPath(want)
	for _, e := range entries {
		if NormalizeRelPath(e.Name()) == wanted {
			return e.Name()
		}
	}
	return ""
}
