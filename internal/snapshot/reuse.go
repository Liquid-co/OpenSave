package snapshot

import (
	"archive/zip"
	"os"
	"sync/atomic"

	"github.com/opensave/opensave/internal/delta"
)

// Snapshots that re-pack only what changed.
//
// Every snapshot is a complete zip, restorable on its own — that stays. What
// changes is how it is written: a file whose content is exactly what the
// previous snapshot of the same branch holds is copied into the new archive
// as its already-compressed entry (zip.Writer.Copy), not read and deflated
// again. Only new and changed files are compressed.
//
// For a save of a quarter-million small files (Project Zomboid map chunks)
// where a play session touches a few hundred, that turns a snapshot from a
// minute of reading and deflating 0.76GB into writing mostly copied bytes.
//
// Content is identified by SHA-256 — the hash each snapshot already records
// per file (store.SnapshotFiles) and the one the manifest's hash cache holds,
// so an unchanged file is recognised without being read. Size is checked as
// well, cheaply, before the hash is asked for.
type reuseSource struct {
	zr     *zip.ReadCloser
	files  map[string]*zip.File // entry name -> previous entry
	hashes map[string]string    // entry name -> sha256 recorded for it
	reused int                  // entries copied, for tests
}

// lastReused is how many entries the most recent snapshot copied from its
// predecessor. Tests read it; nothing else does.
var lastReused atomic.Int64

// openReuseSource prepares the newest snapshot of a branch for reuse. Any
// problem — no snapshot, no recorded file list (an older snapshot, or a
// mirror), a zip that does not open — means no reuse, never a failed snapshot.
func (m *Manager) openReuseSource(gameID, branch string) *reuseSource {
	prev, err := m.LatestSnapshot(gameID, branch)
	if err != nil || prev.ZipPath == "" {
		return nil
	}
	recorded, err := m.Store.SnapshotFiles(prev.ID)
	if err != nil || len(recorded) == 0 {
		return nil
	}
	zr, err := zip.OpenReader(prev.ZipPath)
	if err != nil {
		return nil
	}
	r := &reuseSource{
		zr:     zr,
		files:  make(map[string]*zip.File, len(zr.File)),
		hashes: make(map[string]string, len(recorded)),
	}
	for _, f := range zr.File {
		r.files[f.Name] = f
	}
	for _, c := range recorded {
		name := c.Path
		if c.Root != "" {
			name = RootPrefix + c.Root + "/" + c.Path
		}
		r.hashes[name] = c.Hash
	}
	return r
}

func (r *reuseSource) Close() {
	if r == nil {
		lastReused.Store(0)
		return
	}
	lastReused.Store(int64(r.reused))
	if r.zr != nil {
		r.zr.Close()
	}
}

// addFileEntry adds filePath under entryName, copying the previous
// snapshot's entry when the content is the same, and archiving it afresh
// otherwise. Returns the file's sha256 either way. A nil receiver archives
// afresh, as every snapshot did before.
func (r *reuseSource) addFileEntry(w *zip.Writer, filePath, entryName string, info os.FileInfo) (string, error) {
	if r != nil {
		if hash, ok := r.reusable(filePath, entryName, info); ok {
			if err := w.Copy(r.files[entryName]); err != nil {
				return "", err
			}
			r.reused++
			return hash, nil
		}
	}
	return addFileEntry(w, filePath, entryName)
}

// reusable reports whether the previous snapshot holds exactly this file's
// current content under the same name.
func (r *reuseSource) reusable(filePath, entryName string, info os.FileInfo) (string, bool) {
	prevHash, ok := r.hashes[entryName]
	prev := r.files[entryName]
	if !ok || prev == nil || prev.Mode().IsDir() {
		return "", false
	}
	if info == nil {
		var err error
		if info, err = os.Stat(filePath); err != nil {
			return "", false
		}
	}
	if uint64(info.Size()) != prev.UncompressedSize64 {
		return "", false
	}
	// From the hash cache when the file is unchanged since it was last
	// hashed, which after the watcher's manifest build it nearly always is.
	entry, err := delta.FileEntryForInfo(filePath, info)
	if err != nil || entry.Hash != prevHash {
		return "", false
	}
	return prevHash, true
}
