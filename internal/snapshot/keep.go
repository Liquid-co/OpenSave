package snapshot

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/opensave/opensave/internal/delta"
	"github.com/opensave/opensave/internal/ignore"
	"github.com/opensave/opensave/internal/store"
)

// RestoreKeeping is Restore for a snapshot that may have come from another
// device — through the cloud, or in a backup file — leaving alone the files
// this device's rules exclude from syncing.
//
// Those files are the ones someone has said belong to this machine: its
// graphics settings, its logs. Syncing between paired devices never touches
// them (see syncengine/ignore.go). A plain Restore empties the save folder and
// puts back the snapshot whole, so a save brought from another device through
// the cloud replaced this device's settings with that device's — or deleted
// them, when that device had none.
//
// So the excluded files are copied aside first, the restore runs as it always
// has, and afterwards whatever excluded files the snapshot brought are removed
// and this device's own are put back: excluded files end exactly as they were.
// Restoring one of this device's own snapshots is a different request —
// "make the folder look like it did" — and still uses Restore.
func (m *Manager) RestoreKeeping(gameID, snapshotID string, keep ignore.Rules) (store.Snapshot, error) {
	if keep.Empty() {
		return m.Restore(gameID, snapshotID)
	}
	game, err := m.Store.GetGame(gameID)
	if err != nil {
		return store.Snapshot{}, err
	}
	roots := []string{game.SavePath}
	if extra, err := m.Store.GameRootPaths(gameID); err == nil {
		for _, path := range extra {
			roots = append(roots, path)
		}
	}

	aside, err := os.MkdirTemp(m.keepDir(), "keep-*")
	if err != nil {
		return store.Snapshot{}, fmt.Errorf("set aside this device's own files: %w", err)
	}
	defer os.RemoveAll(aside)

	kept := make([][]string, len(roots))
	for i, root := range roots {
		files, err := copyMatching(root, filepath.Join(aside, fmt.Sprint(i)), keep)
		if err != nil {
			return store.Snapshot{}, fmt.Errorf("set aside this device's own files in %s: %w", root, err)
		}
		kept[i] = files
	}

	snap, restoreErr := m.Restore(gameID, snapshotID)
	// Put them back whether or not the restore succeeded: a restore that
	// failed part-way may already have emptied the folder.
	for i, root := range roots {
		if err := removeMatching(root, keep); err != nil && restoreErr == nil {
			restoreErr = fmt.Errorf("remove the other device's excluded files from %s: %w", root, err)
		}
		for _, rel := range kept[i] {
			if err := copyFile(filepath.Join(aside, fmt.Sprint(i), rel), filepath.Join(root, rel)); err != nil && restoreErr == nil {
				restoreErr = fmt.Errorf("put back %s: %w", rel, err)
			}
		}
		delta.InvalidateRoot(root)
	}
	return snap, restoreErr
}

// keepDir is where excluded files wait during a restore: beside the
// snapshots, which are on a drive with room for them, rather than in the
// system's temp folder.
func (m *Manager) keepDir() string {
	if settings, err := m.Store.GetSettings(); err == nil && settings.BackupsDir != "" {
		if err := os.MkdirAll(settings.BackupsDir, 0o777); err == nil {
			return settings.BackupsDir
		}
	}
	return ""
}

// walkMatching calls fn for every file under root whose path relative to it
// the rules match. A root that is a single file, or absent, has nothing to
// match: rules name files inside a folder.
func walkMatching(root string, keep ignore.Rules, fn func(rel string) error) error {
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil
	}
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable: nothing to keep from it
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		if keep.Match(filepath.ToSlash(rel)) {
			return fn(rel)
		}
		return nil
	})
}

func copyMatching(root, dest string, keep ignore.Rules) ([]string, error) {
	var files []string
	err := walkMatching(root, keep, func(rel string) error {
		if err := copyFile(filepath.Join(root, rel), filepath.Join(dest, rel)); err != nil {
			return err
		}
		files = append(files, rel)
		return nil
	})
	return files, err
}

func removeMatching(root string, keep ignore.Rules) error {
	return walkMatching(root, keep, func(rel string) error {
		path := filepath.Join(root, rel)
		_ = os.Chmod(path, 0o666)
		return os.Remove(path)
	})
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o777); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
