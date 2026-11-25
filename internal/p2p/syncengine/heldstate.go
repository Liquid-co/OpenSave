package syncengine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/opensave/opensave/internal/delta"
)

// Two devices meeting with no agreed state yet, both holding a save, are
// asked which to keep: nothing says which is newer. Unless one of them has
// held the other's save before. A device joining with an old copy — from a
// backup, an earlier install, a folder copied across — holds a state the
// other device has in its own history, and then there is nothing to ask: a
// state both have held is a merge base by definition, the same proof the push
// and served records give (served.go). Banked, the sync judges the two
// against it as always, and the old copy is simply behind.
//
// Each side can only look in its own history. The device holding the newer
// save finds the peer's state there on its own sync; the device holding the
// old copy asks the peer, on a second manifest request identifying
// its own files (ManifestQuery.Held, heldKey), and the peer answers HeldYours. Usually
// the old copy's device syncs first: tracking the game is what starts it.
//
// Compared on files alone, as snapshots record them, after each side's own
// exclusion rules: two devices with different rules simply find no match, and
// are asked as before. So is a peer that predates the question.

// heldKey identifies a save by its files alone, as a snapshot records them:
// how many there are, then a hash of their paths and content hashes, no
// folders. The count goes first so that the device asked can pass over every
// snapshot that holds fewer files without reading it.
func heldKey(files map[string]string) string {
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	h := sha256.New()
	for _, p := range paths {
		io.WriteString(h, p)
		io.WriteString(h, ":")
		io.WriteString(h, files[p])
		io.WriteString(h, "\n")
	}
	return strconv.Itoa(len(files)) + ":" + hex.EncodeToString(h.Sum(nil))
}

func manifestHeldKey(m delta.Manifest) string {
	files := make(map[string]string, len(m.Files))
	for p, f := range m.Files {
		files[p] = f.Hash
	}
	return heldKey(files)
}

// HeldState reports whether a snapshot of this game here holds exactly the
// files key names (heldKey), as this device's exclusion rules see them.
// Answers a peer's ManifestQuery.Held.
//
// One snapshot at a time, newest first, stopping at the first match: a save
// can hold hundreds of thousands of files and a game dozens of snapshots, so
// at most one snapshot's files are in memory. A snapshot with fewer files than
// the key is passed over unread — exclusion rules only take files away.
//
// Only snapshots taken since the game last forgot what it agreed with its
// peers, at an untrack (store.SyncForgottenAt), count. One from before held a
// folder that may have changed while nothing tracked it: a file removed then
// is not a deletion this device made, but with that snapshot as the shared
// starting point the peer read it as one, and deleted its own copy. A
// snapshot whose time cannot be read is passed over too, the safe direction:
// with no proof the two devices are asked, as before.
func (e *Engine) HeldState(gameID, key string) bool {
	count, _, ok := strings.Cut(key, ":")
	want, err := strconv.Atoi(count)
	if !ok || err != nil || want <= 0 {
		return false
	}
	snaps, err := e.Store.SnapshotFileCounts(gameID, delta.PrimaryRoot, want)
	if err != nil {
		return false
	}
	rules := e.rulesFor(gameID)
	forgotten := e.Store.SyncForgottenAt(gameID)
	for _, snap := range snaps {
		if !forgotten.IsZero() {
			at, err := time.Parse(time.RFC3339, snap.Timestamp)
			if err != nil || !at.After(forgotten) {
				continue
			}
		}
		files, err := e.Store.SnapshotRootFiles(snap.SnapshotID, delta.PrimaryRoot)
		if err != nil {
			continue
		}
		for p := range files {
			if excluded(rules, p) {
				delete(files, p)
			}
		}
		if len(files) == want && heldKey(files) == key {
			return true
		}
	}
	return false
}

// heldBase is the merge base two devices with no agreed state have after all:
// the peer's save when this device has held it, this device's when the peer
// has. "" when neither has held the other's, or either holds nothing.
//
// The peer's answer is about its save as it is when it answers, which it
// sends again with it; it counts only while that is still the save this sync
// compares against. A peer that synced in between — took this device's
// missing file from it, say, and snapshotted the result — truly has held this
// device's save by then. Banked against the save it had before, a file it did
// not have yet read as one it deleted, and this device deleted its own copy.
func (e *Engine) heldBase(ctx context.Context, gameID string, peer Peer, q ManifestQuery, local, remote delta.Manifest) string {
	if len(local.Files) == 0 || len(remote.Files) == 0 || sameFiles(local, remote) {
		return ""
	}
	if e.HeldState(gameID, manifestHeldKey(remote)) {
		return remote.ManifestHash()
	}
	q.Held = manifestHeldKey(local)
	again, err := e.Transport.FetchManifest(ctx, peer, gameID, q)
	if err == nil && again.HeldYours && sameFiles(filterManifest(again.Manifest, e.rulesFor(gameID)), remote) {
		return local.ManifestHash()
	}
	return ""
}
