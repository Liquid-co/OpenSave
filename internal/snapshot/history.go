package snapshot

import "fmt"

// How often files changed along with a game's save, read from its
// snapshots: a settings file is changed now and then, when someone changes a
// setting; a save changes with nearly every session. A file taken for a
// game's settings that changes like a save is not its settings
// (daemon/detectsettings.go).
//
// Read from the recorded file lists only, and compared by the database
// (store.SnapshotsDiffer): a save can hold a quarter of a million files, and
// reading each snapshot's whole list — or, for one taken before lists were
// recorded, its whole archive — to find whether the rest of it changed was
// tens of millions of rows each time it was asked. A snapshot with no recorded
// list is left out.

const (
	historySnapshots = 40 // the most recent ones are read
	historyMinSaves  = 4  // changes of the save needed before anything is said
)

// ChangesLikeASave returns, for each of paths (relative to the main save
// folder) that changed in more than half of the snapshots in which the rest
// of the save changed, why it is judged a save. Paths with too little history
// to tell are not in it.
func (m *Manager) ChangesLikeASave(gameID string, paths []string) map[string]string {
	out := map[string]string{}
	if len(paths) == 0 {
		return out
	}
	snaps, err := m.Store.AllSnapshotsOfGame(gameID)
	if err != nil {
		return out
	}
	if len(snaps) > historySnapshots {
		snaps = snaps[len(snaps)-historySnapshots:]
	}

	saves := 0
	moved := map[string]int{}
	prevID, prev := "", map[string]string(nil)
	for _, s := range snaps {
		if !m.Store.SnapshotHasFiles(s.ID) {
			continue
		}
		hashes, err := m.Store.SnapshotPathHashes(s.ID, "", paths)
		if err != nil {
			continue
		}
		if prevID != "" {
			// Only where the rest of the save changed is there anything to
			// compare the watched files with.
			if differ, err := m.Store.SnapshotsDiffer(prevID, s.ID, "", paths); err == nil && differ {
				saves++
				for _, p := range paths {
					if prev[p] != hashes[p] {
						moved[p]++
					}
				}
			}
		}
		prevID, prev = s.ID, hashes
	}
	if saves < historyMinSaves {
		return out
	}
	for p, n := range moved {
		if 2*n > saves {
			out[p] = fmt.Sprintf("changed in %d of the %d snapshots in which the save changed", n, saves)
		}
	}
	return out
}
