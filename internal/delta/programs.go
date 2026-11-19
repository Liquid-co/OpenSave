package delta

import (
	"io/fs"
	"path/filepath"
	"strings"
)

// programExts are files a save folder never needs.
var programExts = map[string]bool{
	".exe": true, ".dll": true, ".so": true, ".dylib": true, ".sys": true,
	".com": true, ".bat": true, ".cmd": true, ".ps1": true, ".msi": true,
	".scr": true, ".asi": true,
}

// HoldsPrograms reports whether a folder, or one below it, holds a program or
// library. Only the first few thousand entries are looked at: a save folder is
// small, and one too large to look through is not taken on trust either. A
// folder that does not exist holds nothing.
func HoldsPrograms(dir string) bool {
	const limit = 5000
	seen, found := 0, false
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if seen++; seen > limit {
			found = true
			return fs.SkipAll
		}
		if !d.IsDir() && programExts[strings.ToLower(filepath.Ext(p))] {
			found = true
			return fs.SkipAll
		}
		return nil
	})
	return found
}
