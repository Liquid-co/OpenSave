package snapshot

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/opensave/opensave/internal/iopar"
)

// Progress is told how far a long operation on a save has come: which phase
// ("rebuilding", "checking", "writing"), how many of how many done.
type Progress func(phase string, done, total int)

// Phases of a restore, as Progress names them.
const (
	PhaseRebuilding = "rebuilding"
	PhaseChecking   = "checking"
	PhaseWriting    = "writing"
)

// extractJob is one archive entry to write: under destDir, at rel.
type extractJob struct {
	f       *zip.File
	destDir string
	rel     string
}

// extractAll writes archive entries into place. Every folder is made first,
// once — not looked up again for each of a quarter of a million files — and
// the files are then written as many at a time as suits the drive they go to
// (iopar): one at a time on a spinning disk, many at once on an SSD, where
// written one after another a large save took most of a quarter of an hour
// at a fraction of what the drive can do.
func extractAll(jobs []extractJob, onDrive string, progress Progress) error {
	type target struct {
		f    *zip.File
		dest string
	}
	var files []target
	dirs := map[string]struct{}{}
	for _, j := range jobs {
		dest, err := entryDest(j.f.Name, j.destDir, j.rel)
		if err != nil {
			return err
		}
		if j.f.FileInfo().IsDir() {
			dirs[dest] = struct{}{}
			continue
		}
		dirs[filepath.Dir(dest)] = struct{}{}
		files = append(files, target{j.f, dest})
	}
	ordered := make([]string, 0, len(dirs))
	for d := range dirs {
		ordered = append(ordered, d)
	}
	sort.Strings(ordered) // a parent before what is inside it
	for _, d := range ordered {
		if err := mkdirMarked(d); err != nil {
			return err
		}
	}

	var report func(done, total int)
	if progress != nil {
		report = func(done, total int) { progress(PhaseWriting, done, total) }
	}
	st, err := iopar.RunKind(context.Background(), driveKindOf(onDrive), len(files), func(i int) error {
		return writeEntry(files[i].f, files[i].dest)
	}, report)
	lastExtract.Store(&st)
	return err
}

// driveKindOf is iopar.DriveKind; a variable, so a test can compare one at a
// time with many on the same drive.
var driveKindOf = iopar.DriveKind

// lastExtract is how the last extraction went, for a test to report.
var lastExtract atomic.Pointer[iopar.Stats]

// entryDest is where an entry is written: rel under destDir, refused if it
// would land outside it (zip-slip). Checked on the name actually written,
// since that is the one that decides where bytes land.
func entryDest(name, destDir, rel string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(rel))
	if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		return "", fmt.Errorf("zip entry %q escapes destination", name)
	}
	return filepath.Join(destDir, clean), nil
}

var copyBuffers = sync.Pool{New: func() any { b := make([]byte, 64<<10); return &b }}

// writeEntry writes one file entry to dest, whose folder exists.
func writeEntry(f *zip.File, dest string) error {
	return writeMarked(dest, func() error {
		src, err := f.Open()
		if err != nil {
			return err
		}
		defer src.Close()
		dst, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o666)
		if err != nil {
			return err
		}
		buf := copyBuffers.Get().(*[]byte)
		defer copyBuffers.Put(buf)
		if _, err := io.CopyBuffer(dst, src, *buf); err != nil {
			dst.Close()
			return err
		}
		return dst.Close()
	})
}

// progressFor is how a restore of gameID tells OnRestoreProgress how far it
// has come; nil when nobody listens.
func (m *Manager) progressFor(gameID string) Progress {
	if m.OnRestoreProgress == nil {
		return nil
	}
	return func(phase string, done, total int) { m.OnRestoreProgress(gameID, phase, done, total) }
}

// progressDone tells OnRestoreProgress the restore of gameID is over.
func (m *Manager) progressDone(gameID string) {
	if m.OnRestoreProgress != nil {
		m.OnRestoreProgress(gameID, "", 0, 0)
	}
}
