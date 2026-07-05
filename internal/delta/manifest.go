// Package delta implements block-level content hashing and manifest
// diff/patch, mirroring src/daemon/delta.js from the original Node app:
// variable block size (64KB / 512KB above 20MB / 2MB above 100MB), SHA-256
// per block plus a whole-file hash, and a manifest tree of relative paths.
package delta

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"
)

const (
	defaultBlockSize = 64 * 1024
	mediumBlockSize  = 512 * 1024
	largeBlockSize   = 2 * 1024 * 1024

	mediumFileThreshold = 20 * 1024 * 1024
	largeFileThreshold  = 100 * 1024 * 1024
)

// BlockSizeFor returns the chunking block size for a file of the given
// size, matching delta.js's scaling thresholds exactly.
func BlockSizeFor(fileSize int64) int {
	switch {
	case fileSize > largeFileThreshold:
		return largeBlockSize
	case fileSize > mediumFileThreshold:
		return mediumBlockSize
	default:
		return defaultBlockSize
	}
}

// Block describes one fixed-size (except possibly the last) chunk of a file.
type Block struct {
	Index  int    `json:"index"`
	Hash   string `json:"hash"`
	Length int    `json:"length"`
}

// Milli is a millisecond timestamp. It unmarshals from any JSON number —
// the original JS daemon sends Node's fractional mtimeMs values
// (e.g. 1783279365872.0251), which would fail to decode into a plain
// int64 — and marshals back as a whole integer.
type Milli int64

// UnmarshalJSON accepts integers and floats, truncating fractions.
func (m *Milli) UnmarshalJSON(b []byte) error {
	var f float64
	if err := json.Unmarshal(b, &f); err != nil {
		return err
	}
	*m = Milli(f)
	return nil
}

// MarshalJSON emits a plain integer.
func (m Milli) MarshalJSON() ([]byte, error) {
	return []byte(strconv.FormatInt(int64(m), 10)), nil
}

// FileEntry is one file's record inside a Manifest.
type FileEntry struct {
	Size      int64   `json:"size"`
	Hash      string  `json:"hash"`
	Blocks    []Block `json:"blocks"`
	BlockSize int     `json:"blockSize"`
	MtimeMs   Milli   `json:"mtime"`
}

// Manifest is the full tree snapshot of a tracked save folder (or single
// file), keyed by path relative to the save root.
type Manifest struct {
	Timestamp   time.Time            `json:"timestamp"`
	LatestMtime Milli                `json:"latestMtime"`
	Files       map[string]FileEntry `json:"files"`
	Dirs        []string             `json:"dirs"`
}

// hashBytes returns the lowercase hex SHA-256 of b.
func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// HashFile computes the whole-file SHA-256 and per-block SHA-256 list for
// the file at path, using the block size dictated by its size.
func HashFile(path string) (FileEntry, error) {
	info, err := os.Stat(path)
	if err != nil {
		return FileEntry{}, err
	}

	blockSize := BlockSizeFor(info.Size())
	f, err := os.Open(path)
	if err != nil {
		return FileEntry{}, err
	}
	defer f.Close()

	whole := sha256.New()
	buf := make([]byte, blockSize)
	var blocks []Block
	index := 0
	for {
		n, readErr := io.ReadFull(f, buf)
		if n > 0 {
			chunk := buf[:n]
			whole.Write(chunk)
			blocks = append(blocks, Block{
				Index:  index,
				Hash:   hashBytes(chunk),
				Length: n,
			})
			index++
		}
		if readErr == io.EOF || readErr == io.ErrUnexpectedEOF {
			break
		}
		if readErr != nil {
			return FileEntry{}, readErr
		}
	}

	return FileEntry{
		Size:      info.Size(),
		Hash:      hex.EncodeToString(whole.Sum(nil)),
		Blocks:    blocks,
		BlockSize: blockSize,
		MtimeMs:   Milli(info.ModTime().UnixMilli()),
	}, nil
}

// BuildManifest walks root (a directory or a single file, per
// ResolveLocalSaveFilePath) and returns a full Manifest of its contents.
func BuildManifest(root string) (Manifest, error) {
	info, err := os.Stat(root)
	if err != nil {
		return Manifest{}, err
	}

	m := Manifest{
		Timestamp: time.Now().UTC(),
		Files:     map[string]FileEntry{},
	}

	if !info.IsDir() {
		entry, err := HashFile(root)
		if err != nil {
			return Manifest{}, err
		}
		m.Files[filepath.Base(root)] = entry
		m.LatestMtime = entry.MtimeMs
		return m, nil
	}

	var dirs []string
	err = filepath.Walk(root, func(path string, walkInfo os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if isDotEntry(rel) {
			if walkInfo.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if walkInfo.IsDir() {
			dirs = append(dirs, rel)
			return nil
		}

		entry, err := HashFile(path)
		if err != nil {
			return err
		}
		m.Files[rel] = entry
		if entry.MtimeMs > m.LatestMtime {
			m.LatestMtime = entry.MtimeMs
		}
		return nil
	})
	if err != nil {
		return Manifest{}, err
	}

	sort.Strings(dirs)
	m.Dirs = dirs
	return m, nil
}

// ManifestHash returns a single stable hash summarizing a manifest's
// content: the per-file hashes (sorted by relative path) plus the dir
// list. Two manifests with identical file contents yield the same hash
// regardless of mtimes — this is what the watcher and sync engine compare
// to decide "has anything actually changed".
func (m Manifest) ManifestHash() string {
	paths := make([]string, 0, len(m.Files))
	for p := range m.Files {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	h := sha256.New()
	for _, p := range paths {
		io.WriteString(h, p)
		io.WriteString(h, ":")
		io.WriteString(h, m.Files[p].Hash)
		io.WriteString(h, "\n")
	}
	for _, d := range m.Dirs {
		io.WriteString(h, "dir:"+d+"\n")
	}
	return hex.EncodeToString(h.Sum(nil))
}

// isDotEntry reports whether any path segment of rel starts with a dot,
// matching chokidar's default `ignored: /(^|[\/\\])\../` behavior.
func isDotEntry(rel string) bool {
	for _, seg := range splitSlash(rel) {
		if len(seg) > 0 && seg[0] == '.' {
			return true
		}
	}
	return false
}

func splitSlash(p string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(p); i++ {
		if p[i] == '/' {
			parts = append(parts, p[start:i])
			start = i + 1
		}
	}
	parts = append(parts, p[start:])
	return parts
}
