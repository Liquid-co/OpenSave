package snapshot

import (
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/opensave/opensave/internal/fsx"
	"github.com/opensave/opensave/internal/store"
)

// What restoring a snapshot would do, worked out without doing it.
//
// Restore empties each save location and puts the snapshot's files back, so
// "Restore" on its own asks someone to trust a timestamp. This says, file by
// file, what would change: files the snapshot has in another version, files
// it would bring back, and files it would remove because they came later —
// all of which the safety snapshot taken first keeps.
//
// It mirrors UnzipRoots rather than approximating it: the same locations are
// emptied, a location the snapshot names but this device has no folder for is
// left alone (and listed as such), and a single-file save is compared as that
// file.

// Change kinds, as the preview names them.
const (
	ChangeModified = "changed"  // in both, with different contents
	ChangeRestored = "restored" // in the snapshot only: it comes back
	ChangeRemoved  = "removed"  // on disk only: restoring removes it
)

// FileChange is one file a restore would touch.
type FileChange struct {
	// Location is the extra save location the file belongs to; empty for the
	// game's main save folder.
	Location     string `json:"location,omitempty"`
	Path         string `json:"path"`
	Change       string `json:"change"`
	CurrentSize  int64  `json:"currentSize"`
	SnapshotSize int64  `json:"snapshotSize"`
}

// RestorePreview is everything a restore would change.
type RestorePreview struct {
	SnapshotID string       `json:"snapshotId"`
	Changes    []FileChange `json:"changes"`
	// Unchanged counts the files a restore would write back exactly as they
	// already are.
	Unchanged int `json:"unchanged"`
	// Unplaced names locations the snapshot has files for and this device has
	// no folder for; a restore leaves them out, and so does this.
	Unplaced []string `json:"unplaced,omitempty"`
}

// Identical reports whether restoring would change nothing at all.
func (p RestorePreview) Identical() bool { return len(p.Changes) == 0 }

type archived struct {
	size uint64
	crc  uint32
}

// PreviewRestore works out what Restore(gameID, snapshotID) would change.
func (m *Manager) PreviewRestore(gameID, snapshotID string) (RestorePreview, error) {
	game, err := m.Store.GetGame(gameID)
	if err != nil {
		return RestorePreview{}, err
	}
	snap, err := m.Store.GetSnapshot(snapshotID)
	if err != nil {
		return RestorePreview{}, err
	}
	if snap.GameID != gameID {
		return RestorePreview{}, fmt.Errorf("snapshot %q does not belong to game %q: %w", snapshotID, gameID, store.ErrNotFound)
	}
	roots, err := m.Store.GameRootPaths(gameID)
	if err != nil {
		roots = nil
	}

	entries, err := ArchiveEntries(snap.ZipPath)
	if err != nil {
		return RestorePreview{}, fmt.Errorf("open snapshot %s: %w", snapshotID, err)
	}

	primaryIsFile := false
	if info, statErr := os.Stat(game.SavePath); statErr == nil {
		primaryIsFile = !info.IsDir()
	}

	// What the snapshot holds, per location, and which locations a restore
	// would empty: the main folder always, an extra one when the snapshot
	// has anything for it and this device has a folder to put it in.
	inSnapshot := map[string]map[string]archived{"": {}}
	unplaced := map[string]bool{}
	for _, f := range entries {
		location, isRoot := rootOfEntry(f.Name)
		rel := f.Name
		if isRoot {
			if path, ok := roots[location]; !ok || strings.TrimSpace(path) == "" {
				unplaced[location] = true
				continue
			}
			rel = strings.TrimPrefix(f.Name, RootPrefix+location+"/")
			if inSnapshot[location] == nil {
				inSnapshot[location] = map[string]archived{}
			}
		} else if primaryIsFile {
			rel = filepath.Base(game.SavePath)
		}
		if rel == "" || strings.HasSuffix(rel, "/") {
			continue // a folder, not a file
		}
		inSnapshot[location][rel] = archived{size: f.Size, crc: f.CRC32}
	}

	preview := RestorePreview{SnapshotID: snapshotID, Changes: []FileChange{}}
	for location, files := range inSnapshot {
		dir := game.SavePath
		if location != "" {
			dir = roots[location]
		}
		onDisk, err := filesUnder(dir)
		if err != nil {
			return RestorePreview{}, fmt.Errorf("read %s: %w", dir, err)
		}
		for rel, a := range files {
			size, present := onDisk[rel]
			switch {
			case !present:
				preview.Changes = append(preview.Changes, FileChange{Location: location, Path: rel, Change: ChangeRestored, SnapshotSize: int64(a.size)})
			case uint64(size) != a.size || !sameContent(filepath.Join(pathBase(dir, primaryIsFile && location == ""), filepath.FromSlash(rel)), a.crc):
				preview.Changes = append(preview.Changes, FileChange{Location: location, Path: rel, Change: ChangeModified, CurrentSize: size, SnapshotSize: int64(a.size)})
			default:
				preview.Unchanged++
			}
		}
		for rel, size := range onDisk {
			if _, kept := files[rel]; !kept {
				preview.Changes = append(preview.Changes, FileChange{Location: location, Path: rel, Change: ChangeRemoved, CurrentSize: size})
			}
		}
	}
	for name := range unplaced {
		preview.Unplaced = append(preview.Unplaced, name)
	}
	sort.Strings(preview.Unplaced)
	sort.Slice(preview.Changes, func(i, j int) bool {
		a, b := preview.Changes[i], preview.Changes[j]
		if a.Location != b.Location {
			return a.Location < b.Location
		}
		return a.Path < b.Path
	})
	return preview, nil
}

// filesUnder lists the files in a save location by slash-separated path
// relative to it, with their sizes. A save that is a single file is listed as
// that file; a location that does not exist yet holds nothing.
func filesUnder(root string) (map[string]int64, error) {
	out := map[string]int64{}
	info, err := os.Stat(root)
	if errors.Is(err, fs.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		out[filepath.Base(root)] = info.Size()
		return out, nil
	}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if path == root {
				return walkErr
			}
			return nil // unreadable below the root: the same skip a snapshot makes
		}
		if d.IsDir() {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = fi.Size()
		return nil
	})
	return out, err
}

// pathBase is the folder a location's relative paths are joined to: the
// location itself, or the folder holding a single-file save.
func pathBase(dir string, singleFile bool) string {
	if singleFile {
		return filepath.Dir(dir)
	}
	return dir
}

// sameContent compares a file on disk with an archived one by the checksum
// the archive already holds, so nothing is extracted. Read shared, the way a
// snapshot reads it, so a game holding the file open does not fail this.
func sameContent(path string, want uint32) bool {
	f, err := fsx.OpenShared(path)
	if err != nil {
		return false
	}
	defer f.Close()
	h := crc32.NewIEEE()
	if _, err := io.Copy(h, f); err != nil {
		return false
	}
	return h.Sum32() == want
}
