package snapshot

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/opensave/opensave/internal/fsx"
	"github.com/opensave/opensave/internal/store"
)

// Snapshots that share what they have in common.
//
// Every snapshot is written as a complete zip archive. A game that keeps
// twenty save slots and changes one between snapshots stores all twenty in
// every one of them, so older snapshots are compacted in the background:
// each large file in the archive is kept once per game folder, in .shared/,
// under the SHA-256 of its bytes as the archive stores them, and the
// snapshot keeps a list of what it held (<id>.files.json beside where its
// zip was) plus a small archive of its folders and small files (<id>.rest).
//
// The stored bytes, not the file's contents: a file compressed in one
// snapshot and stored as it is in another (snapshots from before 2.4 kept
// everything uncompressed) are two shared files, not one. That costs a
// little sharing across that change and buys an archive rebuilt byte for
// byte as it was written — the same size its record gives, so an upload that
// compares sizes does not take it for a different file.
//
// Nothing outside this file sees the difference. Anything that needs the
// archive itself — a restore, an upload, a peer, an export — asks for it with
// OpenArchive and gets a whole zip back, rebuilt from the shared files when
// it has been compacted; the listing that compare and the restore preview
// read comes from ArchiveEntries without rebuilding anything. What leaves
// this device is always a complete zip, so peers, the cloud and exports are
// unchanged, and a device running an older version never sees a manifest.
//
// The cost is that one damaged shared file damages every snapshot that holds
// it. Three things bound that: the newest snapshot on every branch, and every
// pinned one, stay complete archives of their own; a zip is only removed once
// the archive rebuilt in its place has been read back and found to hold the
// same files; and the daily check reads every shared file back against the
// hash it is named for.
//
// One process at a time works on a store: the app and a command-line run of
// `opensave` share the folder, and a lock file in the store (lockStore) keeps
// one from clearing out a shared file the other has just started to use.
// Within a process, sharedMu does the same without touching the disk.
//
// The layout is what makes clean-up safe. A manifest at <root>/<a>/<b>.files.json
// uses <root>/.shared and no other store, so every manifest that can name a
// shared file sits exactly two levels below the store's parent, and reading
// those is all it takes to know which shared files are still needed.

const (
	sharedDirName   = ".shared"
	manifestSuffix  = ".files.json"
	restSuffix      = ".rest"
	blobSuffix      = ".blob"
	manifestVersion = 1

	// SharedMinBytes is the smallest entry, compressed, that goes to the
	// shared store. Smaller ones stay with their snapshot: a file system
	// allocates whole clusters, so a store of thousands of tiny files would
	// take more room than the archives it replaced.
	SharedMinBytes = 16 << 10

	// CompactMinAge is how old a snapshot must be before it is compacted: by
	// then its upload and any peer that wanted it have had their chance at
	// the zip as it was written.
	CompactMinAge = time.Hour
)

// sharedEntry is one entry of a compacted archive, in the order the archive
// had them.
type sharedEntry struct {
	Name string `json:"name"`
	// Blob names the entry's file in the shared store, by the SHA-256 of its
	// stored bytes; empty, the entry is in the snapshot's own .rest archive,
	// header and all.
	Blob string `json:"blob,omitempty"`
	// What a listing needs, so compare and the restore preview read nothing
	// else.
	CRC32 uint32 `json:"crc32"`
	Size  uint64 `json:"size"`
	// The rest of a shared entry's header as the original archive had it, so
	// the rebuilt archive holds the same entry: its name's encoding, its time
	// (the MS-DOS fields and the extended timestamp both), and its attributes.
	Flags          uint16 `json:"flags,omitempty"`
	ModDate        uint16 `json:"modDate,omitempty"`
	ModTime        uint16 `json:"modTime,omitempty"`
	Extra          []byte `json:"extra,omitempty"`
	CreatorVersion uint16 `json:"creatorVersion,omitempty"`
	ExternalAttrs  uint32 `json:"externalAttrs,omitempty"`
}

type sharedManifest struct {
	Version int           `json:"version"`
	Entries []sharedEntry `json:"entries"`
}

func manifestPath(zipPath string) string { return strings.TrimSuffix(zipPath, ".zip") + manifestSuffix }
func restPath(zipPath string) string     { return strings.TrimSuffix(zipPath, ".zip") + restSuffix }
func storeRoot(zipPath string) string    { return filepath.Dir(filepath.Dir(zipPath)) }
func sharedStore(zipPath string) string  { return filepath.Join(storeRoot(zipPath), sharedDirName) }
func blobPath(store, sum string) string  { return filepath.Join(store, sum[:2], sum+blobSuffix) }

// compactable says whether a snapshot's archive sits where compaction can
// work: a .zip, two folders down from somewhere that is not the file
// system's root.
func compactable(zipPath string) bool {
	if !strings.HasSuffix(zipPath, ".zip") || !filepath.IsAbs(zipPath) {
		return false
	}
	root := storeRoot(zipPath)
	return root != filepath.Dir(root) && root != filepath.Dir(zipPath)
}

// reading counts who has a snapshot's zip open through OpenArchive, so
// compaction never removes a zip something is part-way through reading.
// Counted, not locked: nothing waits on anything, so nothing can deadlock.
var reading = struct {
	sync.Mutex
	n map[string]int
}{n: map[string]int{}}

// OpenArchive gives a snapshot's archive as a zip file to read, whether it
// is kept whole or has been compacted; done must be called once finished
// with it. A missing snapshot is an error that matches fs.ErrNotExist; a
// compacted one whose shared files are damaged, one that matches ErrDamaged.
func OpenArchive(zipPath string) (path string, done func(), err error) {
	reading.Lock()
	if _, statErr := os.Stat(zipPath); statErr == nil {
		reading.n[zipPath]++
		reading.Unlock()
		var once sync.Once
		return zipPath, func() {
			once.Do(func() {
				reading.Lock()
				defer reading.Unlock()
				if reading.n[zipPath]--; reading.n[zipPath] <= 0 {
					delete(reading.n, zipPath)
				}
			})
		}, nil
	}
	reading.Unlock()

	if _, statErr := os.Stat(manifestPath(zipPath)); statErr != nil {
		return "", func() {}, fmt.Errorf("snapshot archive %s: %w", zipPath, fs.ErrNotExist)
	}
	tmp, err := os.CreateTemp("", "opensave-snapshot-*.zip")
	if err != nil {
		return "", func() {}, err
	}
	name := tmp.Name()
	err = rebuild(zipPath, tmp)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(name)
		return "", func() {}, err
	}
	return name, func() { os.Remove(name) }, nil
}

// ArchiveExists says whether a snapshot still has its archive, whole or
// compacted.
func ArchiveExists(zipPath string) bool {
	if _, err := os.Stat(zipPath); err == nil {
		return true
	}
	_, err := os.Stat(manifestPath(zipPath))
	return err == nil
}

// ArchiveEntry is one entry of a snapshot's archive, as its listing has it.
type ArchiveEntry struct {
	Name  string
	Size  uint64
	CRC32 uint32
	IsDir bool
}

// ArchiveEntries lists what a snapshot's archive holds, in order, without
// reading any file in it.
func ArchiveEntries(zipPath string) ([]ArchiveEntry, error) {
	if r, err := zip.OpenReader(zipPath); err == nil {
		defer r.Close()
		out := make([]ArchiveEntry, 0, len(r.File))
		for _, f := range r.File {
			out = append(out, ArchiveEntry{Name: f.Name, Size: f.UncompressedSize64, CRC32: f.CRC32, IsDir: f.FileInfo().IsDir()})
		}
		return out, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	man, err := readManifest(manifestPath(zipPath))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("snapshot archive %s: %w", zipPath, fs.ErrNotExist)
		}
		return nil, err
	}
	out := make([]ArchiveEntry, 0, len(man.Entries))
	for _, e := range man.Entries {
		out = append(out, ArchiveEntry{Name: e.Name, Size: e.Size, CRC32: e.CRC32, IsDir: strings.HasSuffix(e.Name, "/")})
	}
	return out, nil
}

func readManifest(path string) (sharedManifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return sharedManifest{}, err
	}
	var man sharedManifest
	if err := json.Unmarshal(raw, &man); err != nil {
		return sharedManifest{}, fmt.Errorf("%w: its list of files cannot be read: %v", ErrDamaged, err)
	}
	if man.Version != manifestVersion {
		return sharedManifest{}, fmt.Errorf("its list of files is version %d, which this version of OpenSave cannot read", man.Version)
	}
	for _, e := range man.Entries {
		if e.Blob != "" && !validSum(e.Blob) {
			return sharedManifest{}, fmt.Errorf("%w: its list of files names %q, which is not a shared file", ErrDamaged, e.Blob)
		}
	}
	return man, nil
}

func validSum(s string) bool {
	if len(s) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil && strings.ToLower(s) == s
}

// rebuild writes a compacted snapshot's archive back out whole.
func rebuild(zipPath string, out io.Writer) error {
	man, err := readManifest(manifestPath(zipPath))
	if err != nil {
		return err
	}
	store := sharedStore(zipPath)
	var rest []*zip.File
	if r, err := zip.OpenReader(restPath(zipPath)); err == nil {
		defer r.Close()
		rest = r.File
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%w: its small files cannot be read: %v", ErrDamaged, err)
	}
	w := zip.NewWriter(out)
	next := 0
	for _, e := range man.Entries {
		if e.Blob != "" {
			if err := copyShared(w, store, e); err != nil {
				return err
			}
			continue
		}
		if next >= len(rest) || rest[next].Name != e.Name {
			return fmt.Errorf("%w: %s is missing from its small files", ErrDamaged, e.Name)
		}
		if err := w.Copy(rest[next]); err != nil {
			return fmt.Errorf("%w: %s cannot be read: %v", ErrDamaged, e.Name, err)
		}
		next++
	}
	return w.Close()
}

// copyShared writes one shared entry into a rebuilt archive: its compressed
// bytes as they are, under the header the original archive gave it.
func copyShared(w *zip.Writer, store string, e sharedEntry) error {
	b, err := zip.OpenReader(blobPath(store, e.Blob))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%w: the shared copy of %s is missing", ErrDamaged, e.Name)
		}
		return fmt.Errorf("%w: the shared copy of %s cannot be opened: %v", ErrDamaged, e.Name, err)
	}
	defer b.Close()
	if len(b.File) != 1 || b.File[0].CRC32 != e.CRC32 || b.File[0].UncompressedSize64 != e.Size {
		return fmt.Errorf("%w: the shared copy of %s is not the file it should be", ErrDamaged, e.Name)
	}
	f := b.File[0]
	raw, err := f.OpenRaw()
	if err != nil {
		return fmt.Errorf("%w: the shared copy of %s cannot be read: %v", ErrDamaged, e.Name, err)
	}
	hw, err := w.CreateRaw(&zip.FileHeader{
		Name:               e.Name,
		Flags:              e.Flags&^0x8 | f.Flags&0x8, // the data descriptor goes with the bytes
		Method:             f.Method,
		CRC32:              f.CRC32,
		CompressedSize64:   f.CompressedSize64,
		UncompressedSize64: f.UncompressedSize64,
		ModifiedDate:       e.ModDate,
		ModifiedTime:       e.ModTime,
		Extra:              e.Extra,
		CreatorVersion:     e.CreatorVersion,
		ReaderVersion:      f.ReaderVersion,
		ExternalAttrs:      e.ExternalAttrs,
	})
	if err != nil {
		return err
	}
	if _, err := io.Copy(hw, raw); err != nil {
		return fmt.Errorf("%w: the shared copy of %s cannot be read: %v", ErrDamaged, e.Name, err)
	}
	return nil
}

// timestampExtra keeps an entry's extended timestamp and nothing else from
// its extra field: the writer adds a Zip64 field of its own when one is
// needed, and a second copy of it would confuse a reader.
func timestampExtra(extra []byte) []byte {
	var out []byte
	for len(extra) >= 4 {
		id := binary.LittleEndian.Uint16(extra)
		size := int(binary.LittleEndian.Uint16(extra[2:]))
		if 4+size > len(extra) {
			break
		}
		if id == 0x5455 {
			out = append(out, extra[:4+size]...)
		}
		extra = extra[4+size:]
	}
	return out
}

// putShared puts one archive entry in the shared store, unless it is there
// already, and returns the name it is kept under: the SHA-256 of its stored
// bytes. Nothing damaged stays referenced — compact reads the rebuilt
// archive back through each file's checksum before a list names any of it.
func putShared(store string, f *zip.File) (sum string, added int64, err error) {
	sum, err = storedSum(f)
	if err != nil {
		return "", 0, fmt.Errorf("%s: %w", f.Name, err)
	}
	dest := blobPath(store, sum)
	if sharedIntact(dest, f) {
		return sum, 0, nil
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o777); err != nil {
		return "", 0, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".incoming-*")
	if err != nil {
		return "", 0, err
	}
	fail := func(err error) (string, int64, error) {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", 0, err
	}
	w := zip.NewWriter(tmp)
	raw, err := f.OpenRaw()
	if err != nil {
		return fail(err)
	}
	fh := f.FileHeader
	fh.Name = sum
	fh.Flags &^= 0x800
	fh.Comment = ""
	fh.Extra = nil
	hw, err := w.CreateRaw(&fh)
	if err != nil {
		return fail(err)
	}
	if _, err := io.Copy(hw, raw); err != nil {
		return fail(err)
	}
	if err := w.Close(); err != nil {
		return fail(err)
	}
	if err := tmp.Sync(); err != nil {
		return fail(err)
	}
	info, _ := tmp.Stat()
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return "", 0, err
	}
	// Over a damaged copy, if that is what was there: the new one is good.
	if err := os.Rename(tmp.Name(), dest); err != nil {
		os.Remove(tmp.Name())
		if sharedIntact(dest, f) {
			return sum, 0, nil
		}
		return "", 0, err
	}
	if info != nil {
		added = info.Size()
	}
	return sum, added, nil
}

// storedSum is the SHA-256 of an entry's bytes as its archive stores them.
func storedSum(f *zip.File) (string, error) {
	raw, err := f.OpenRaw()
	if err != nil {
		return "", err
	}
	h := sha256.New()
	if _, err := io.Copy(h, raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// sharedIntact says whether a shared file is there and, by its header, the
// entry it should be. Its bytes are read by the daily check, not here.
func sharedIntact(path string, f *zip.File) bool {
	b, err := zip.OpenReader(path)
	if err != nil {
		return false
	}
	defer b.Close()
	if len(b.File) != 1 {
		return false
	}
	g := b.File[0]
	return g.Method == f.Method && g.CRC32 == f.CRC32 && g.CompressedSize64 == f.CompressedSize64 && g.UncompressedSize64 == f.UncompressedSize64
}

// errNothingShared is a snapshot with no file large enough to share: left
// as it is. errStoreBusy is one whose store another process is working on:
// left for the next pass.
var (
	errNothingShared = errors.New("nothing in it is large enough to share")
	errStoreBusy     = errors.New("another process is working on its shared files")
)

// lockStore takes the lock on the shared store under root, for work that
// adds or removes shared files. ok is false when another process has it.
func lockStore(root string) (unlock func(), ok bool) {
	store := filepath.Join(root, sharedDirName)
	if err := os.MkdirAll(store, 0o777); err != nil {
		return nil, false
	}
	unlock, ok, err := fsx.TryLock(filepath.Join(store, ".lock"))
	if err != nil || !ok {
		return nil, false
	}
	return unlock, true
}

// compact turns one snapshot's zip into shared files, a list of them, and
// its own small files, then removes the zip — only once the archive rebuilt
// from those has been read back and found to hold what the zip holds.
// Returns how much less room the snapshot takes. The caller holds sharedMu.
func (m *Manager) compact(snap store.Snapshot) (saved int64, err error) {
	zipPath := snap.ZipPath
	if !compactable(zipPath) {
		return 0, errNothingShared
	}
	info, err := os.Stat(zipPath)
	if err != nil {
		return 0, err
	}
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrDamaged, err)
	}
	defer r.Close()
	worth := false
	for _, f := range r.File {
		if !f.FileInfo().IsDir() && f.CompressedSize64 >= SharedMinBytes {
			worth = true
			break
		}
	}
	if !worth {
		return 0, errNothingShared
	}
	unlock, ok := lockStore(storeRoot(zipPath))
	if !ok {
		return 0, errStoreBusy
	}
	defer unlock()

	store := sharedStore(zipPath)
	restTmp, err := os.CreateTemp(filepath.Dir(zipPath), ".rest-*")
	if err != nil {
		return 0, err
	}
	var written []string // what to take away again if this does not finish
	undo := func() {
		restTmp.Close()
		os.Remove(restTmp.Name())
		for _, p := range written {
			os.Remove(p)
		}
	}
	rw := zip.NewWriter(restTmp)
	man := sharedManifest{Version: manifestVersion, Entries: make([]sharedEntry, 0, len(r.File))}
	var added int64
	for _, f := range r.File {
		e := sharedEntry{Name: f.Name, CRC32: f.CRC32, Size: f.UncompressedSize64}
		if !f.FileInfo().IsDir() && f.CompressedSize64 >= SharedMinBytes {
			sum, n, err := putShared(store, f)
			if err != nil {
				undo()
				return 0, err
			}
			added += n
			e.Blob = sum
			e.Flags = f.Flags
			e.ModDate = f.ModifiedDate
			e.ModTime = f.ModifiedTime
			e.Extra = timestampExtra(f.Extra)
			e.CreatorVersion = f.CreatorVersion
			e.ExternalAttrs = f.ExternalAttrs
		} else if err := rw.Copy(f); err != nil {
			undo()
			return 0, err
		}
		man.Entries = append(man.Entries, e)
	}
	if err := rw.Close(); err != nil {
		undo()
		return 0, err
	}
	if err := restTmp.Sync(); err != nil {
		undo()
		return 0, err
	}
	restInfo, _ := restTmp.Stat()
	if err := restTmp.Close(); err != nil {
		undo()
		return 0, err
	}
	if err := os.Rename(restTmp.Name(), restPath(zipPath)); err != nil {
		undo()
		return 0, err
	}
	written = append(written, restPath(zipPath))

	raw, err := json.Marshal(man)
	if err != nil {
		undo()
		return 0, err
	}
	if err := writeFileAtomic(manifestPath(zipPath), raw); err != nil {
		undo()
		return 0, err
	}
	written = append(written, manifestPath(zipPath))

	// The zip stays until what replaces it is known to be the same archive.
	if err := sameArchive(zipPath, r); err != nil {
		undo()
		return 0, fmt.Errorf("the rebuilt archive did not match, so the snapshot was left as it was: %w", err)
	}
	// Deleted while this ran: what was just written is nobody's.
	if _, err := m.Store.GetSnapshot(snap.ID); err != nil {
		undo()
		return 0, nil
	}
	r.Close() // Windows will not remove a file that is open

	reading.Lock()
	inUse := reading.n[zipPath] > 0
	if !inUse {
		err = os.Remove(zipPath)
	}
	reading.Unlock()
	if inUse || err != nil {
		// Both are there, and the zip is what anything reads: the next pass
		// removes it.
		return 0, nil
	}
	after := int64(len(raw)) + added
	if restInfo != nil {
		after += restInfo.Size()
	}
	return info.Size() - after, nil
}

// sameArchive rebuilds a compacted snapshot and checks it against the zip it
// came from: the same entries, in the same order, with the same names,
// sizes, checksums and times — and every file in it read to its end, which
// checks each against its checksum.
func sameArchive(zipPath string, orig *zip.ReadCloser) error {
	tmp, err := os.CreateTemp("", "opensave-check-*.zip")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	err = rebuild(zipPath, tmp)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	got, err := zip.OpenReader(tmp.Name())
	if err != nil {
		return err
	}
	defer got.Close()
	if len(got.File) != len(orig.File) {
		return fmt.Errorf("%d entries, not %d", len(got.File), len(orig.File))
	}
	for i, g := range got.File {
		o := orig.File[i]
		if g.Name != o.Name || g.CRC32 != o.CRC32 || g.UncompressedSize64 != o.UncompressedSize64 || !g.Modified.Equal(o.Modified) {
			return fmt.Errorf("%s is not the same entry", o.Name)
		}
		if g.FileInfo().IsDir() {
			continue
		}
		rc, err := g.Open()
		if err != nil {
			return err
		}
		_, err = io.Copy(io.Discard, rc)
		rc.Close()
		if err != nil {
			return fmt.Errorf("%s: %w", g.Name, err)
		}
	}
	return nil
}

func writeFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".incoming-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}

// collectShared removes the shared files under root that no snapshot names
// any more, and returns the room that freed. The caller holds sharedMu, so
// no compaction here is part-way through naming new ones, and the store's
// lock keeps out one in another process. ran is false when that lock was
// taken: the clean-up is for the next pass.
func collectShared(root string) (freed int64, ran bool) {
	store := filepath.Join(root, sharedDirName)
	if _, err := os.Stat(store); err != nil {
		return 0, true
	}
	unlock, ok := lockStore(root)
	if !ok {
		return 0, false
	}
	defer unlock()
	return sweepShared(root, store), true
}

func sweepShared(root, store string) (freed int64) {
	used := map[string]bool{}
	folders, err := os.ReadDir(root)
	if err != nil {
		return 0
	}
	for _, folder := range folders {
		if !folder.IsDir() || folder.Name() == sharedDirName {
			continue
		}
		files, err := os.ReadDir(filepath.Join(root, folder.Name()))
		if err != nil {
			return 0 // cannot see every list: remove nothing
		}
		for _, f := range files {
			if leftover(f) {
				os.Remove(filepath.Join(root, folder.Name(), f.Name()))
				continue
			}
			if f.IsDir() || !strings.HasSuffix(f.Name(), manifestSuffix) {
				continue
			}
			man, err := readManifest(filepath.Join(root, folder.Name(), f.Name()))
			if err != nil {
				return 0 // a list that cannot be read might name any of them
			}
			for _, e := range man.Entries {
				if e.Blob != "" {
					used[e.Blob] = true
				}
			}
		}
	}
	shards, err := os.ReadDir(store)
	if err != nil {
		return 0
	}
	for _, shard := range shards {
		dir := filepath.Join(store, shard.Name())
		if !shard.IsDir() {
			continue
		}
		files, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		left := len(files)
		for _, f := range files {
			path := filepath.Join(dir, f.Name())
			info, err := f.Info()
			if err != nil {
				continue
			}
			stale := strings.HasPrefix(f.Name(), ".incoming-") && time.Since(info.ModTime()) > time.Hour
			unused := strings.HasSuffix(f.Name(), blobSuffix) && !used[strings.TrimSuffix(f.Name(), blobSuffix)]
			if (stale || unused) && os.Remove(path) == nil {
				freed += info.Size()
				left--
			}
		}
		if left == 0 {
			os.Remove(dir)
		}
	}
	return freed
}

// leftover is what a compaction cut short by a crash left beside the
// snapshots: its small files or its list, half written, an hour ago or more.
func leftover(f fs.DirEntry) bool {
	if f.IsDir() || !(strings.HasPrefix(f.Name(), ".rest-") || strings.HasPrefix(f.Name(), ".incoming-")) {
		return false
	}
	info, err := f.Info()
	return err == nil && time.Since(info.ModTime()) > time.Hour
}

// dropArchives removes the files of snapshots whose rows are gone — the zip,
// or the list and small files of a compacted one — and any shared file none
// of the rest still names. Returns the room freed.
func (m *Manager) dropArchives(snaps []store.Snapshot) (freed int64) {
	roots := map[string]bool{}
	for _, s := range snaps {
		if s.ZipPath == "" {
			continue
		}
		for _, p := range []string{s.ZipPath, manifestPath(s.ZipPath), restPath(s.ZipPath)} {
			if info, err := os.Stat(p); err == nil && os.Remove(p) == nil {
				freed += info.Size()
				if p != s.ZipPath && compactable(s.ZipPath) {
					roots[storeRoot(s.ZipPath)] = true
				}
			}
		}
	}
	if len(roots) == 0 {
		return freed
	}
	// Not while a compaction runs, which may be about to name a shared file
	// no list names yet: the next pass collects them instead.
	if !m.sharedMu.TryLock() {
		for root := range roots {
			m.putOff(root)
		}
		return freed
	}
	defer m.sharedMu.Unlock()
	for root := range roots {
		n, ran := collectShared(root)
		freed += n
		if !ran {
			m.putOff(root)
		}
	}
	return freed
}

// putOff remembers a clean-up for the next compaction pass.
func (m *Manager) putOff(root string) {
	m.pendingMu.Lock()
	defer m.pendingMu.Unlock()
	if m.pendingRoots == nil {
		m.pendingRoots = map[string]bool{}
	}
	m.pendingRoots[root] = true
}

// CompactResult is what a compaction pass did.
type CompactResult struct {
	// Compacted is how many snapshots now share their large files.
	Compacted int `json:"compacted"`
	// Freed is how much less room snapshots take for it.
	Freed int64 `json:"freed"`
	// Failed is how many could not be compacted, each left as it was.
	Failed int `json:"failed"`
}

// CompactAll compacts every snapshot that can be: not the newest on its
// branch, not pinned, at least minAge old, and still a zip. pace is a pause
// between snapshots, so a pass in the background stays in the background.
func (m *Manager) CompactAll(ctx context.Context, minAge, pace time.Duration) (CompactResult, error) {
	var res CompactResult
	games, err := m.Store.ListGames()
	if err != nil {
		return res, err
	}
	cutoff := m.now().Add(-minAge)
	roots := map[string]bool{}
	for _, g := range games {
		branches, err := m.Store.ListBranches(g.ID)
		if err != nil {
			continue
		}
		for _, b := range branches {
			snaps, err := m.Store.ListSnapshots(g.ID, b)
			if err != nil {
				continue
			}
			for i, s := range snaps {
				if i == 0 || s.Pinned || !compactable(s.ZipPath) {
					continue // newest first: the first stays whole
				}
				if t, err := time.Parse(time.RFC3339Nano, s.Timestamp); err != nil || t.After(cutoff) {
					continue
				}
				if _, err := os.Stat(s.ZipPath); err != nil {
					continue
				}
				if err := ctx.Err(); err != nil {
					return res, err
				}
				m.inFlight.Add(1)
				m.sharedMu.Lock()
				saved, err := m.compact(s)
				m.sharedMu.Unlock()
				m.inFlight.Done()
				switch {
				case errors.Is(err, errNothingShared), errors.Is(err, errStoreBusy):
				case err != nil:
					res.Failed++
					if m.Log != nil {
						m.Log("warn", fmt.Sprintf("could not share the files of %s's snapshot %s: %v", g.Name, s.ID, err))
					}
				default:
					roots[storeRoot(s.ZipPath)] = true
					if _, err := os.Stat(s.ZipPath); err != nil {
						res.Compacted++
						res.Freed += saved
					}
				}
				if pace > 0 {
					select {
					case <-ctx.Done():
						return res, ctx.Err()
					case <-time.After(pace):
					}
				}
			}
		}
	}
	m.pendingMu.Lock()
	for root := range m.pendingRoots {
		roots[root] = true
	}
	m.pendingRoots = nil
	m.pendingMu.Unlock()
	m.sharedMu.Lock()
	for root := range roots {
		n, ran := collectShared(root)
		res.Freed += n
		if !ran {
			m.putOff(root)
		}
	}
	m.sharedMu.Unlock()
	return res, nil
}

// DiskBytes is how much room these snapshots take on disk: their zips, the
// lists and small files of compacted ones, and the shared files those use.
func DiskBytes(snaps []store.Snapshot) int64 {
	var total int64
	roots := map[string]bool{}
	for _, s := range snaps {
		if s.ZipPath == "" {
			continue
		}
		if info, err := os.Stat(s.ZipPath); err == nil {
			total += info.Size()
			continue
		}
		for _, p := range []string{manifestPath(s.ZipPath), restPath(s.ZipPath)} {
			if info, err := os.Stat(p); err == nil {
				total += info.Size()
				roots[storeRoot(s.ZipPath)] = true
			}
		}
	}
	for root := range roots {
		_ = filepath.WalkDir(filepath.Join(root, sharedDirName), func(_ string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				if info, err := d.Info(); err == nil {
					total += info.Size()
				}
			}
			return nil
		})
	}
	return total
}
