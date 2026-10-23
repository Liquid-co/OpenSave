package snapshot

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	"github.com/opensave/opensave/internal/delta"
	"github.com/opensave/opensave/internal/store"
)

// ArchiveFiles reads a snapshot's archive and lists its files with their
// content hashes, as the snapshot would have recorded them.
func ArchiveFiles(zipPath string) ([]store.CapturedFile, error) {
	path, done, err := OpenArchive(zipPath)
	if err != nil {
		return nil, err
	}
	defer done()
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	out := make([]store.CapturedFile, 0, len(r.File))
	for _, f := range r.File {
		if f.FileInfo().IsDir() || strings.HasSuffix(f.Name, "/") || strings.HasSuffix(f.Name, delta.TmpSuffix) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.Name, err)
		}
		h := sha256.New()
		_, err = io.Copy(h, rc)
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.Name, err)
		}
		c := store.CapturedFile{Path: f.Name, Hash: hex.EncodeToString(h.Sum(nil))}
		if rest, ok := strings.CutPrefix(f.Name, RootPrefix); ok {
			if root, p, ok := strings.Cut(rest, "/"); ok {
				c.Root, c.Path = root, p
			}
		}
		out = append(out, c)
	}
	return out, nil
}

// filesOf is what a snapshot holds, from its recorded file list — read from
// its archive once, and recorded, when it has none.
func (m *Manager) filesOf(snap store.Snapshot) ([]store.CapturedFile, error) {
	if files, err := m.Store.SnapshotFiles(snap.ID); err == nil && len(files) > 0 {
		return files, nil
	}
	files, err := ArchiveFiles(snap.ZipPath)
	if err != nil {
		return nil, err
	}
	if len(files) > 0 {
		_ = m.Store.RecordSnapshotFiles(snap.ID, files)
	}
	return files, nil
}
