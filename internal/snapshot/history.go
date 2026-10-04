package snapshot

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
)

// How often files changed along with a game's save, read from its
// snapshots: a settings file is changed now and then, when someone changes a
// setting; a save changes with nearly every session. A file taken for a
// game's settings that changes like a save is not its settings
// (daemon/detectsettings.go).

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
	watched := make(map[string]bool, len(paths))
	for _, p := range paths {
		watched[p] = true
	}
	snaps, err := m.Store.AllSnapshotsOfGame(gameID)
	if err != nil {
		return out
	}
	if len(snaps) > historySnapshots {
		snaps = snaps[len(snaps)-historySnapshots:]
	}

	// Per snapshot, the hashes of the watched files and one value for the
	// rest of the save.
	type state struct {
		watched map[string]string
		rest    string
	}
	var states []state
	for _, s := range snaps {
		files, err := m.filesOf(s)
		if err != nil || len(files) == 0 {
			continue
		}
		st := state{watched: map[string]string{}}
		var rest []string
		for _, f := range files {
			if f.Root != "" {
				continue
			}
			if watched[f.Path] {
				st.watched[f.Path] = f.Hash
			} else {
				rest = append(rest, f.Path+"\x00"+f.Hash)
			}
		}
		st.rest = restKey(rest)
		states = append(states, st)
	}

	saves := 0
	moved := map[string]int{}
	for i := 1; i < len(states); i++ {
		a, b := states[i-1], states[i]
		if a.rest == b.rest {
			continue // the save did not change: nothing to compare with
		}
		saves++
		for p := range watched {
			if a.watched[p] != b.watched[p] {
				moved[p]++
			}
		}
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

func restKey(entries []string) string {
	sort.Strings(entries)
	h := sha256.New()
	for _, e := range entries {
		h.Write([]byte(e))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}
