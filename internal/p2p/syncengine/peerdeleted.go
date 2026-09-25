package syncengine

import (
	"sort"
	"time"

	"github.com/opensave/opensave/internal/delta"
)

// Deletions another device asked this one to make, remembered for a while.
//
// A device's deletions reach this one as a request per file, and this
// device's own syncs carry on while they arrive. A sync that lands between
// two of them finds its save part-way through the other device's deletions:
// no longer the state the two agreed on, and the other device's copy not
// either. Taken at face value that is both sides having changed — a conflict
// that stops the game syncing until someone resolves it by hand — though the
// only thing that changed here was the other device's own deletions.
//
// So each deletion made on request is remembered with the manifest entry of
// what it removed, and a sync that is about to judge whether this side
// changed puts those entries back first (asBeforePeerDeletions). If that is
// exactly the agreed state, nothing changed here of this device's own doing,
// and the sync carries on — it sees the other device lacks the files and
// finishes the deletions itself. If it is not, something else changed here
// too, and the conflict stands as it always did.

// peerDeletionMemory is how long a deletion made on request is remembered.
// Long enough for a slow relay to deliver a large batch; short enough that
// what it describes is still the recent past.
const peerDeletionMemory = 15 * time.Minute

type peerDeletion struct {
	entry delta.FileEntry
	dir   bool
	at    time.Time
}

// NotePeerDeletion remembers a deletion this device made because another
// device asked: rel in the save location root ("" is the main folder), and
// the manifest entry of the file it removed. dir marks a folder, which has
// no entry. Called after the removal succeeded.
func (e *Engine) NotePeerDeletion(gameID, root, rel string, entry delta.FileEntry, dir bool) {
	e.peerDelMu.Lock()
	defer e.peerDelMu.Unlock()
	if e.peerDeleted == nil {
		e.peerDeleted = map[string]map[string]peerDeletion{}
	}
	key := gameID + "\x00" + root
	if e.peerDeleted[key] == nil {
		e.peerDeleted[key] = map[string]peerDeletion{}
	}
	now := time.Now()
	for p, d := range e.peerDeleted[key] {
		if now.Sub(d.at) > peerDeletionMemory {
			delete(e.peerDeleted[key], p)
		}
	}
	e.peerDeleted[key][rel] = peerDeletion{entry: entry, dir: dir, at: now}
}

// asBeforePeerDeletions is a location's manifest as it was before the
// deletions other devices asked for lately: what they removed put back,
// where this device has not got it again since. ok is false when there is
// nothing to put back. Excluded paths stay out, as in the manifest given.
func (e *Engine) asBeforePeerDeletions(gameID, root string, local delta.Manifest) (before delta.Manifest, ok bool) {
	e.peerDelMu.Lock()
	defer e.peerDelMu.Unlock()
	deleted := e.peerDeleted[gameID+"\x00"+root]
	if len(deleted) == 0 {
		return local, false
	}
	rules := e.rulesFor(gameID)
	before = delta.Manifest{
		Timestamp:   local.Timestamp,
		LatestMtime: local.LatestMtime,
		Files:       make(map[string]delta.FileEntry, len(local.Files)+len(deleted)),
		Dirs:        append([]string{}, local.Dirs...),
	}
	for p, f := range local.Files {
		before.Files[p] = f
	}
	dirs := map[string]bool{}
	for _, d := range local.Dirs {
		dirs[d] = true
	}
	now := time.Now()
	for p, d := range deleted {
		if now.Sub(d.at) > peerDeletionMemory || rules.Match(p) {
			continue
		}
		if d.dir {
			if !dirs[p] {
				dirs[p] = true
				before.Dirs = append(before.Dirs, p)
				ok = true
			}
			continue
		}
		if _, present := before.Files[p]; !present {
			before.Files[p] = d.entry
			ok = true
		}
	}
	sort.Strings(before.Dirs) // as BuildManifest orders them, which the hash follows
	return before, ok
}

// unchangedButForPeerDeletions is the manifest to judge this side by when
// asking whether it changed since the agreed state: the one given, or — when
// the only difference from the agreed state is deletions other devices asked
// for — the agreed state itself, as it was before they arrived.
func (e *Engine) unchangedButForPeerDeletions(gameID, root string, local delta.Manifest, agreed string) delta.Manifest {
	if agreed == "" || local.ManifestHash() == agreed {
		return local
	}
	if before, ok := e.asBeforePeerDeletions(gameID, root, local); ok && before.ManifestHash() == agreed {
		e.Log("info", "this device's save differs from the agreed state only by deletions another device asked for — not a change of its own")
		return before
	}
	return local
}
