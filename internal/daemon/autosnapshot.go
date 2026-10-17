package daemon

import (
	"errors"
	"path/filepath"
	"sync"

	"github.com/opensave/opensave/internal/snapshot"
)

// An automatic snapshot that could not read every file of the save is
// discarded (snapshot.CreateAutomatic), and the watcher takes the change again
// at its next pass. That is right once or twice: a file was being written.
// A game that holds a file open the whole time it runs — a log, a lock file —
// has every one discarded until it closes, and with them the sync each would
// have started. Said only in the log, that looked like a save neither backed
// up nor synced for no reason; so after a few passes in a row it is said on
// screen, once, until a snapshot of the game succeeds again.

// incompleteAutoSnapshotsToSay is how many passes in a row must end with the
// snapshot discarded before it is said on screen.
const incompleteAutoSnapshotsToSay = 3

// SnapshotsIncomplete is what is said: the game, and some of the files that
// could not be read.
type SnapshotsIncomplete struct {
	GameID   string   `json:"gameId"`
	GameName string   `json:"gameName"`
	Files    []string `json:"files"` // names only, at most three
	Count    int      `json:"count"` // how many could not be read
}

type incompleteAutoSnapshots struct {
	mu     sync.Mutex
	inARow map[string]int
}

// autoSnapshotTaken ends a game's run of discarded automatic snapshots.
func (d *Daemon) autoSnapshotTaken(gameID string) {
	d.incomplete.mu.Lock()
	delete(d.incomplete.inARow, gameID)
	d.incomplete.mu.Unlock()
}

// autoSnapshotGaveUp is the watcher's pass ending without a snapshot. Only an
// incomplete one counts: any other failure is the log's to tell.
func (d *Daemon) autoSnapshotGaveUp(gameID string, err error) {
	var inc *snapshot.IncompleteError
	if !errors.As(err, &inc) {
		return
	}
	d.incomplete.mu.Lock()
	if d.incomplete.inARow == nil {
		d.incomplete.inARow = map[string]int{}
	}
	d.incomplete.inARow[gameID]++
	n := d.incomplete.inARow[gameID]
	d.incomplete.mu.Unlock()
	if n != incompleteAutoSnapshotsToSay || d.OnSnapshotsIncomplete == nil {
		return
	}
	said := SnapshotsIncomplete{GameID: gameID, GameName: inc.Game, Count: len(inc.Skipped)}
	for _, p := range inc.Skipped {
		if len(said.Files) == 3 {
			break
		}
		said.Files = append(said.Files, filepath.Base(p))
	}
	d.OnSnapshotsIncomplete(said)
}
