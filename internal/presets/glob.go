package presets

import (
	"os"
	"path/filepath"
	"strings"
)

// Recursive wildcards for manifest templates.
//
// The Ludusavi manifest writes "**" for "any number of folders, none
// included" — "<winLocalAppData>/Game/Saved/SaveGames/**/*.sav" — as
// Ludusavi's own globbing reads it. filepath.Glob has no such thing and takes
// "**" for a single "*", which needs exactly one folder there: a save sitting
// directly in SaveGames, or two folders down, was never found. 2,070 of the
// manifest's templates use it.

// A "**" walk visits at most this many folders, this deep, per pattern: 605 of
// those templates start at the game's install folder, which can be tens of
// thousands of folders of game data.
const (
	maxGlobWalkDirs  = 2000
	maxGlobWalkDepth = 10
)

// globPaths returns the paths that exist and match pattern, as filepath.Glob
// does, with "**" as a whole path element matching any number of folders.
// Matches come shallowest first. Links to folders are not followed.
func globPaths(pattern string) []string {
	budget := maxGlobWalkDirs
	return globWithin(pattern, &budget)
}

// globWithin is globPaths drawing on one folder budget for the whole pattern,
// however many roots a wildcard before "**" gives it and however many "**"
// it holds.
func globWithin(pattern string, budget *int) []string {
	sep := string(filepath.Separator)
	parts := strings.Split(pattern, sep)
	star := -1
	for i, p := range parts {
		if p == "**" {
			star = i
			break
		}
	}
	if star < 0 {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil
		}
		return matches
	}

	head := strings.Join(parts[:star], sep)
	if head == "" {
		head = sep
	}
	rest := parts[star+1:]
	for len(rest) > 0 && (rest[0] == "**" || rest[0] == "") {
		rest = rest[1:]
	}

	roots := []string{head}
	if strings.ContainsAny(head, "*?[") {
		var err error
		if roots, err = filepath.Glob(head); err != nil {
			return nil
		}
	}

	var out []string
	seen := map[string]bool{}
	for _, root := range roots {
		if !dirExists(root) {
			continue
		}
		if len(rest) == 0 {
			// A trailing "**" is everything in the folder: the folder.
			if !seen[root] {
				seen[root] = true
				out = append(out, root)
			}
			continue
		}
		tail := strings.Join(rest, sep)
		for _, dir := range walkDirs(root, budget) {
			for _, m := range globWithin(filepath.Join(dir, tail), budget) {
				if !seen[m] {
					seen[m] = true
					out = append(out, m)
				}
			}
		}
	}
	return out
}

// walkDirs lists root and the folders below it, breadth first, taking each
// from budget and stopping when it runs out or at the depth limit. Links are
// left alone: a link to a folder is not a folder here, so a walk never leaves
// the tree it was asked about or goes round in a loop.
func walkDirs(root string, budget *int) []string {
	if *budget <= 0 {
		return nil
	}
	*budget--
	out := []string{root}
	level := []string{root}
	for depth := 0; depth < maxGlobWalkDepth && len(level) > 0; depth++ {
		var next []string
		for _, dir := range level {
			entries, err := os.ReadDir(dir)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				if *budget <= 0 {
					return out
				}
				*budget--
				p := filepath.Join(dir, e.Name())
				out = append(out, p)
				next = append(next, p)
			}
		}
		level = next
	}
	return out
}
