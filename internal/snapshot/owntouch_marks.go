package snapshot

import (
	"io/fs"
	"os"
	"path/filepath"

	"github.com/opensave/opensave/internal/owntouch"
)

// A restore is OpenSave writing a save folder, not the game. Every file it
// puts down, folder it makes and path it removes is marked (internal/owntouch)
// so the watcher records the restored save as the one the folder now holds,
// rather than taking it for the game saving and snapshotting it again. Without
// this every restore, branch switch and "put back" was followed a moment later
// by an "Auto backup" of the very save just restored, listed as "Save changed".

// markRemovedTree marks path, and everything under it, as about to be removed
// by OpenSave. Deepest first: marking a path marks its folder too, as merely
// touched, which would undo a folder's own "removed" mark made before it.
func markRemovedTree(path string) {
	var all []string
	_ = filepath.WalkDir(path, func(p string, _ fs.DirEntry, err error) error {
		if err == nil {
			all = append(all, p)
		}
		return nil
	})
	for i := len(all) - 1; i >= 0; i-- {
		owntouch.MarkRemoved(all[i])
	}
	if len(all) == 0 {
		owntouch.MarkRemoved(path)
	}
}

// mkdirMarked is os.MkdirAll that marks each folder it creates as OpenSave's.
//
// Marked as OpenSave's whether found there or gone (MarkRemoved), not just as
// touched: a folder a restore emptied away is usually made again a moment
// later, and the event for its removal can arrive after this mark and before
// the folder exists again. A plain mark would undo the removal's, and that
// late event read as somebody else deleting the folder: an automatic snapshot
// of the save just restored. A marked folder that exists is OpenSave's either
// way, so nothing else changes.
func mkdirMarked(dir string) error {
	for p := dir; ; {
		if _, err := os.Lstat(p); err == nil {
			break
		}
		owntouch.MarkRemoved(p)
		parent := filepath.Dir(p)
		if parent == p {
			break
		}
		p = parent
	}
	return os.MkdirAll(dir, 0o777)
}

// writeMarked marks dest as OpenSave's, runs write (which creates and fills
// it), and once that succeeds records the time and size it left the file
// with: a game saving over it a moment later is then the game's change.
func writeMarked(dest string, write func() error) error {
	owntouch.Mark(dest)
	if err := write(); err != nil {
		return err
	}
	owntouch.Settled(dest)
	return nil
}
