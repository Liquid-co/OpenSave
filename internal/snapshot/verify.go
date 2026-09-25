package snapshot

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"sync"
	"time"

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
//
// A snapshot compacted to share its files with others (shared.go) is checked
// by its parts: its own small files read in full, and every shared file it
// names read back against the hash of the bytes it was stored with.
func VerifyArchive(path string) error {
	r, err := zip.OpenReader(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			if _, statErr := os.Stat(manifestPath(path)); statErr == nil {
				return verifyCompacted(path)
			}
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

// damagedArchive says why a snapshot's archive could not be had, in the
// terms VerifyArchive uses.
func damagedArchive(path string, err error) error {
	switch {
	case errors.Is(err, ErrDamaged):
		return err
	case errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("%w: its archive is missing (%s)", ErrDamaged, path)
	default:
		return fmt.Errorf("%w: its archive cannot be opened: %v", ErrDamaged, err)
	}
}

// verifyCompacted checks a compacted snapshot: its list, its small files,
// and each shared file it names.
func verifyCompacted(path string) error {
	man, err := readManifest(manifestPath(path))
	if err != nil {
		return damagedArchive(path, err)
	}
	needRest := false
	for _, e := range man.Entries {
		if e.Blob == "" {
			needRest = true
			break
		}
	}
	if needRest {
		if err := VerifyArchive(restPath(path)); err != nil {
			return fmt.Errorf("%w (its small files)", err)
		}
	}
	store := sharedStore(path)
	for _, e := range man.Entries {
		if e.Blob == "" {
			continue
		}
		if err := verifyShared(blobPath(store, e.Blob), e); err != nil {
			return err
		}
	}
	return nil
}

// sharedChecked remembers shared files read back whole recently, so the
// daily check reads each once rather than once for every snapshot that
// names it. Keyed by path; an entry counts while the file's size and time
// are what they were when it was read.
var sharedChecked sync.Map // path -> sharedCheck

type sharedCheck struct {
	size int64
	mod  time.Time
	at   time.Time
}

const sharedCheckFor = 12 * time.Hour

func verifyShared(path string, e sharedEntry) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%w: the shared copy of %s is missing", ErrDamaged, e.Name)
	}
	if v, ok := sharedChecked.Load(path); ok {
		c := v.(sharedCheck)
		if c.size == info.Size() && c.mod.Equal(info.ModTime()) && time.Since(c.at) < sharedCheckFor {
			return nil
		}
	}
	b, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("%w: the shared copy of %s cannot be opened: %v", ErrDamaged, e.Name, err)
	}
	defer b.Close()
	if len(b.File) != 1 || b.File[0].CRC32 != e.CRC32 || b.File[0].UncompressedSize64 != e.Size {
		return fmt.Errorf("%w: the shared copy of %s is not the file it should be", ErrDamaged, e.Name)
	}
	// Its stored bytes, exactly: the ones a rebuilt archive is made of, and
	// the ones the file is named for.
	sum, err := storedSum(b.File[0])
	if err != nil || sum != e.Blob {
		sharedChecked.Delete(path)
		return fmt.Errorf("%w: the shared copy of %s no longer matches its checksum", ErrDamaged, e.Name)
	}
	sharedChecked.Store(path, sharedCheck{size: info.Size(), mod: info.ModTime(), at: time.Now()})
	return nil
}
