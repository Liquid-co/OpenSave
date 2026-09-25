package snapshot

import (
	"archive/zip"
	"fmt"
	"sort"
	"strings"
)

// Comparing two snapshots of a game: which files one has that the other does
// not, and which both have in different versions — "what did that session
// change?", or "which of these two is the save from before the boss?".
//
// Read from the archives' own directories, which record every file's size and
// checksum, so nothing is unpacked and it takes no time however large the
// save is.

// Diff kinds, going from the first snapshot to the second.
const (
	DiffAdded   = "added"   // only in the second
	DiffRemoved = "removed" // only in the first
	DiffChanged = "changed" // in both, with different contents
)

// SnapshotDiff is one file that differs.
type SnapshotDiff struct {
	// Location is the extra save location the file belongs to; empty for the
	// game's main save folder.
	Location string `json:"location,omitempty"`
	Path     string `json:"path"`
	Change   string `json:"change"`
	FromSize int64  `json:"fromSize"`
	ToSize   int64  `json:"toSize"`
}

// Comparison is everything that differs between two snapshots.
type Comparison struct {
	From      string         `json:"from"`
	To        string         `json:"to"`
	Changes   []SnapshotDiff `json:"changes"`
	Unchanged int            `json:"unchanged"`
}

// Compare works out what changed from one snapshot of a game to another.
func (m *Manager) Compare(gameID, fromID, toID string) (Comparison, error) {
	var files [2]map[[2]string]archived
	for i, id := range []string{fromID, toID} {
		snap, err := m.Store.GetSnapshot(id)
		if err != nil {
			return Comparison{}, err
		}
		if snap.GameID != gameID {
			return Comparison{}, fmt.Errorf("snapshot %q is not one of %q's", id, gameID)
		}
		listed, err := archiveContents(snap.ZipPath)
		if err != nil {
			return Comparison{}, fmt.Errorf("read snapshot %s: %w", id, err)
		}
		files[i] = listed
	}

	out := Comparison{From: fromID, To: toID, Changes: []SnapshotDiff{}}
	for key, a := range files[0] {
		b, ok := files[1][key]
		switch {
		case !ok:
			out.Changes = append(out.Changes, SnapshotDiff{Location: key[0], Path: key[1], Change: DiffRemoved, FromSize: int64(a.size)})
		case a.crc != b.crc || a.size != b.size:
			out.Changes = append(out.Changes, SnapshotDiff{Location: key[0], Path: key[1], Change: DiffChanged, FromSize: int64(a.size), ToSize: int64(b.size)})
		default:
			out.Unchanged++
		}
	}
	for key, b := range files[1] {
		if _, ok := files[0][key]; !ok {
			out.Changes = append(out.Changes, SnapshotDiff{Location: key[0], Path: key[1], Change: DiffAdded, ToSize: int64(b.size)})
		}
	}
	sort.Slice(out.Changes, func(i, j int) bool {
		a, b := out.Changes[i], out.Changes[j]
		if a.Location != b.Location {
			return a.Location < b.Location
		}
		return a.Path < b.Path
	})
	return out, nil
}

// archiveContents lists a snapshot's files by location and path.
func archiveContents(path string) (map[[2]string]archived, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	out := map[[2]string]archived{}
	for _, f := range r.File {
		location, isRoot := rootOfEntry(f.Name)
		rel := f.Name
		if isRoot {
			rel = strings.TrimPrefix(f.Name, RootPrefix+location+"/")
		}
		if rel == "" || strings.HasSuffix(rel, "/") {
			continue
		}
		out[[2]string{location, rel}] = archived{size: f.UncompressedSize64, crc: f.CRC32}
	}
	return out, nil
}
