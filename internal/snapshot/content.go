package snapshot

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/opensave/opensave/internal/delta"
	"github.com/opensave/opensave/internal/store"
)

// Snapshot content.
//
// A snapshot is named by what it holds: every file's name in the archive and
// the SHA-256 of its content, hashed together. The file hashes are the ones the
// sync compares (the hash cache both use), so the same save gives the same
// value on every device — which is how a device knows the snapshot another
// device took is one it already has, and keeps the save archived once.
//
// Before this, a save of a quarter-million files was archived again for every
// device it was compared with, and again on every new snapshot over there:
// gigabytes of identical archives for one 780 MB save.

// contentEntry is one file of a snapshot: its name in the archive and its hash.
type contentEntry struct{ name, hash string }

func contentKeyOf(entries []contentEntry) string {
	sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
	h := sha256.New()
	for _, e := range entries {
		h.Write([]byte(e.name))
		h.Write([]byte{0})
		h.Write([]byte(e.hash))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// archiveName is the name a captured file has in the archive.
func archiveName(c store.CapturedFile) string {
	if c.Root == "" {
		return c.Path
	}
	return RootPrefix + c.Root + "/" + c.Path
}

// ContentKey names what a snapshot holds, from its recorded files.
func ContentKey(files []store.CapturedFile) string {
	entries := make([]contentEntry, 0, len(files))
	for _, f := range files {
		entries = append(entries, contentEntry{archiveName(f), f.Hash})
	}
	return contentKeyOf(entries)
}

// ContentKeyOfArchive names what a snapshot holds by reading its archive: for
// a snapshot recorded without its file list (one taken for a sync with a peer).
func ContentKeyOfArchive(zipPath string) (string, error) {
	files, err := ArchiveFiles(zipPath)
	if err != nil {
		return "", err
	}
	return ContentKey(files), nil
}

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

// contentHashOf works out what a snapshot holds (filesOf).
func (m *Manager) contentHashOf(snap store.Snapshot) (string, error) {
	files, err := m.filesOf(snap)
	if err != nil {
		return "", err
	}
	return ContentKey(files), nil
}

// PruneContained removes automatic snapshots that a newer snapshot on the same
// branch holds entirely: every file of the older one is in the newer one, with
// the same content. Such a snapshot is only a step on the way to the newer
// one — a pull that stopped part-way, a save that only gained files — and
// restoring it would give nothing the newer one lacks but the absence of what
// was added since. The other way round is kept: a newer snapshot contained in
// an older one is a save something was deleted from, on purpose. Pinned
// snapshots and ones taken by hand are never removed.
//
// The largest snapshots are looked at first as the ones that may hold others —
// they hold the most — and passes repeat until one removes nothing. After
// that, each new snapshot is checked against the older ones as it is taken
// (PruneContainedBy), which is all it takes to stay that way.
func (m *Manager) PruneContained(gameID string) (removed int, freed int64, err error) {
	for {
		n, f, err := m.pruneContainedPass(gameID)
		removed += n
		freed += f
		if err != nil || n == 0 {
			return removed, freed, err
		}
	}
}

func (m *Manager) pruneContainedPass(gameID string) (removed int, freed int64, err error) {
	snaps, err := m.Store.AllSnapshotsOfGame(gameID) // oldest first
	if err != nil {
		return 0, 0, err
	}
	byBranch := map[string][]store.Snapshot{}
	counts := map[string]int{}
	for _, s := range snaps {
		if !ArchiveExists(s.ZipPath) {
			continue
		}
		files, ferr := m.filesOf(s)
		if ferr != nil {
			continue
		}
		counts[s.ID] = len(files)
		byBranch[s.BranchName] = append(byBranch[s.BranchName], s)
	}
	for _, list := range byBranch {
		order := make(map[string]int, len(list)) // position in time
		for i, s := range list {
			order[s.ID] = i
		}
		holders := append([]store.Snapshot(nil), list...)
		sort.SliceStable(holders, func(i, j int) bool { return counts[holders[i].ID] > counts[holders[j].ID] })
		gone := map[string]bool{}
		for _, holder := range holders {
			if gone[holder.ID] || counts[holder.ID] == 0 {
				continue
			}
			holderFiles, ferr := m.filesOf(holder)
			if ferr != nil {
				continue
			}
			have := make(map[string]string, len(holderFiles))
			for _, f := range holderFiles {
				have[archiveName(f)] = f.Hash
			}
			for _, older := range list[:order[holder.ID]] {
				if gone[older.ID] || older.Pinned || !older.IsSystemAuto || counts[older.ID] == 0 || counts[older.ID] > len(holderFiles) {
					continue
				}
				olderFiles, ferr := m.filesOf(older)
				if ferr != nil || !containedIn(olderFiles, have) {
					continue
				}
				identical := len(olderFiles) == len(holderFiles)
				if identical {
					_ = m.Store.RepointSnapshotAliases(older.ID, holder.ID)
					m.carryName(older, &holder)
				}
				f, derr := m.DeleteSnapshot(gameID, older.ID)
				if derr != nil {
					continue
				}
				if identical {
					// The same files: its id still leads to them.
					_ = m.Store.AddSnapshotAlias(older.ID, holder.ID)
				}
				gone[older.ID] = true
				removed++
				freed += f
				if m.Log != nil {
					m.Log("info", fmt.Sprintf("snapshot %s removed: every file of it is in the newer %s", older.ID, holder.ID))
				}
			}
		}
	}
	return removed, freed, nil
}

func containedIn(files []store.CapturedFile, have map[string]string) bool {
	for _, f := range files {
		if have[archiveName(f)] != f.Hash {
			return false
		}
	}
	return true
}

// PruneContainedBy checks the older snapshots of snap's branch against snap,
// just taken, and removes those it holds entirely (PruneContained). Only when
// PruneOnCreate is set; in the background, counted for shutdown.
func (m *Manager) PruneContainedBy(snap store.Snapshot, files []store.CapturedFile) {
	if !m.PruneOnCreate || len(files) == 0 {
		return
	}
	m.inFlight.Add()
	go func() {
		defer m.inFlight.Done()
		have := make(map[string]string, len(files))
		for _, f := range files {
			have[archiveName(f)] = f.Hash
		}
		snaps, err := m.Store.ListSnapshots(snap.GameID, snap.BranchName)
		if err != nil {
			return
		}
		for _, older := range snaps {
			if older.ID == snap.ID || older.Timestamp >= snap.Timestamp || older.Pinned || !older.IsSystemAuto ||
				!ArchiveExists(older.ZipPath) {
				continue
			}
			olderFiles, ferr := m.filesOf(older)
			if ferr != nil || len(olderFiles) == 0 || len(olderFiles) > len(files) || !containedIn(olderFiles, have) {
				continue
			}
			identical := len(olderFiles) == len(files)
			if identical {
				_ = m.Store.RepointSnapshotAliases(older.ID, snap.ID)
				m.carryName(older, &snap)
			}
			_, derr := m.DeleteSnapshot(snap.GameID, older.ID)
			if derr == nil && identical {
				_ = m.Store.AddSnapshotAlias(older.ID, snap.ID)
			}
			if derr == nil && m.Log != nil {
				m.Log("info", fmt.Sprintf("snapshot %s removed: every file of it is in the newer %s", older.ID, snap.ID))
			}
		}
	}()
}

// isNewest reports whether id is the newest snapshot on a game's branch.
func (m *Manager) isNewest(gameID, branch, id string) bool {
	snaps, err := m.Store.ListSnapshots(gameID, branch)
	return err == nil && len(snaps) > 0 && snaps[0].ID == id
}

// MergeDuplicates names every snapshot of a game by its content and keeps each
// content archived once per branch: of snapshots holding the same files, one
// keeps its archive — a pinned one, else one taken by hand, else the newest,
// which retention keeps longest —
// and the others are removed, their ids kept as aliases of it so a device that
// knows a snapshot by one of them still finds it. Pinned snapshots are never
// removed. Returns how many were merged and the bytes freed.
func (m *Manager) MergeDuplicates(gameID string) (merged int, freed int64, err error) {
	missing, err := m.Store.SnapshotsWithoutContentHash(gameID)
	if err != nil {
		return 0, 0, err
	}
	for _, snap := range missing {
		if !ArchiveExists(snap.ZipPath) {
			continue
		}
		key, err := m.contentHashOf(snap)
		if err != nil {
			if m.Log != nil {
				m.Log("warn", fmt.Sprintf("could not read snapshot %s to name its content: %v", snap.ID, err))
			}
			continue
		}
		_ = m.Store.SetSnapshotContentHash(snap.ID, key)
	}

	snaps, err := m.Store.AllSnapshotsOfGame(gameID)
	if err != nil {
		return 0, 0, err
	}
	groups := map[string][]store.Snapshot{}
	for _, s := range snaps {
		if s.ContentHash == "" {
			continue
		}
		k := s.BranchName + "\x00" + s.ContentHash
		groups[k] = append(groups[k], s)
	}
	for _, group := range groups {
		if len(group) < 2 {
			continue
		}
		keep := group[len(group)-1] // newest (oldest first)
		for _, s := range group {
			if s.Pinned && !keep.Pinned || s.Pinned == keep.Pinned && !s.IsSystemAuto && keep.IsSystemAuto {
				keep = s
			}
		}
		for _, s := range group {
			if s.ID == keep.ID || s.Pinned {
				continue
			}
			_ = m.Store.RepointSnapshotAliases(s.ID, keep.ID)
			f, err := m.DeleteSnapshot(gameID, s.ID)
			if err != nil {
				continue
			}
			_ = m.Store.AddSnapshotAlias(s.ID, keep.ID)
			merged++
			freed += f
			if m.Log != nil {
				m.Log("info", fmt.Sprintf("snapshot %s held the same files as %s; kept once", s.ID, keep.ID))
			}
		}
	}
	return merged, freed, nil
}

// defaultAutoComment is what an automatic snapshot taken for no particular
// reason is called.
const defaultAutoComment = "Auto backup"

// carryName keeps what an older snapshot is called when a newer one holding
// exactly the same files takes its place: its reason ("After playing (1 h)",
// "Put back") if the newer one has only the default an automatic snapshot
// gets, and a note someone wrote if the newer one has none. A session's
// named snapshot and the watcher's copy of the same save, taken a moment
// later as the game's last write settled, otherwise left only "Auto backup".
func (m *Manager) carryName(older store.Snapshot, newer *store.Snapshot) {
	if newer.Comment == defaultAutoComment && older.Comment != "" && older.Comment != defaultAutoComment {
		if m.Store.SetSnapshotComment(newer.ID, older.Comment) == nil {
			newer.Comment = older.Comment
		}
	}
	if newer.Note == "" && older.Note != "" {
		if m.Store.SetSnapshotNote(newer.ID, older.Note) == nil {
			newer.Note = older.Note
		}
	}
}
