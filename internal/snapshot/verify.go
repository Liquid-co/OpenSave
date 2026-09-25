package snapshot

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"io/fs"

	"github.com/opensave/opensave/internal/store"
)

// Checking that a snapshot can be restored, before it has to be.
//
// A snapshot is a zip archive, and zip records a checksum for every file in
// it. Reading each file to its end compares the two, so a disk that corrupted
// a few bytes, an archive cut short by a crash or a full disk, or a file
// removed by hand are all found — by a daily check in the background, and
// before a restore empties the save folder (Restore): finding out part-way
// through putting a save back leaves neither the old save nor the new one.

// ErrDamaged marks a snapshot whose archive cannot be read back whole.
var ErrDamaged = errors.New("snapshot is damaged")

// VerifyArchive reads a snapshot archive back in full. It returns nil when
// every file in it matches its checksum, and otherwise an error wrapping
// ErrDamaged that says what is wrong.
func VerifyArchive(path string) error {
	r, err := zip.OpenReader(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%w: its archive is missing (%s)", ErrDamaged, path)
		}
		return fmt.Errorf("%w: its archive cannot be opened: %v", ErrDamaged, err)
	}
	defer r.Close()
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("%w: %s cannot be read: %v", ErrDamaged, f.Name, err)
		}
		_, err = io.Copy(io.Discard, rc)
		rc.Close()
		if err != nil {
			if errors.Is(err, zip.ErrChecksum) {
				return fmt.Errorf("%w: %s no longer matches its checksum", ErrDamaged, f.Name)
			}
			return fmt.Errorf("%w: %s cannot be read: %v", ErrDamaged, f.Name, err)
		}
	}
	return nil
}

// Verify checks one snapshot and records what it found.
func (m *Manager) Verify(snap store.Snapshot) error {
	err := VerifyArchive(snap.ZipPath)
	problem := ""
	if err != nil {
		problem = err.Error()
	}
	if recErr := m.Store.SetSnapshotCheck(snap.ID, m.now().UnixMilli(), problem); recErr != nil && err == nil {
		return recErr
	}
	return err
}
