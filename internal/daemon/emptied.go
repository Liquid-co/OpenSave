package daemon

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/opensave/opensave/internal/p2p/syncengine"
	"github.com/opensave/opensave/internal/snapshot"
	"github.com/opensave/opensave/internal/store"
)

// Saves emptied on this device, held back from the others until someone says
// whether that was meant (see p2p/syncengine/hold.go). Asked in the app, with
// `POST /api/games/{id}/emptied`, and with `opensave emptied`.

// EmptiedSave is one game held back because its save was emptied here.
type EmptiedSave struct {
	GameID  string `json:"gameId"`
	Name    string `json:"name"`
	SinceMs int64  `json:"sinceMs"`
	// State is "held" while it waits for an answer, and "fetching" after
	// "put them back" while files are still to come from other devices.
	State string `json:"state"`
	// Files is how many files the other devices hold that "delete them there
	// too" would remove.
	Files int `json:"files"`
	// Locations is the save locations found empty; "" is the main folder.
	Locations []string `json:"locations"`
	// PutBackFrom is the snapshot "put them back" restores: the newest with
	// any files in it. Empty when there is none, and the files come from the
	// other devices alone.
	PutBackFrom     string `json:"putBackFrom,omitempty"`
	PutBackFromTime string `json:"putBackFromTime,omitempty"`
}

// EmptiedSaves lists the games held back now.
func (d *Daemon) EmptiedSaves() ([]EmptiedSave, error) {
	holds, err := d.Store.ListDeletionHolds()
	if err != nil {
		return nil, err
	}
	out := []EmptiedSave{}
	for _, h := range holds {
		if e, ok := d.emptiedSave(h); ok {
			out = append(out, e)
		}
	}
	return out, nil
}

// EmptiedSaveOf is one game's, when it is held back.
func (d *Daemon) EmptiedSaveOf(gameID string) (EmptiedSave, bool) {
	h, has, err := d.Store.GetDeletionHold(gameID)
	if err != nil || !has {
		return EmptiedSave{}, false
	}
	return d.emptiedSave(h)
}

func (d *Daemon) emptiedSave(h store.DeletionHold) (EmptiedSave, bool) {
	if h.State != store.HoldAsking && h.State != store.HoldFetching {
		return EmptiedSave{}, false
	}
	game, err := d.Store.GetGame(h.GameID)
	if err != nil {
		return EmptiedSave{}, false
	}
	e := EmptiedSave{GameID: h.GameID, Name: game.Name, SinceMs: h.SinceMs, State: h.State, Files: h.FileCount(), Locations: h.LocationNames()}
	if h.State == store.HoldAsking {
		if snap, ok := d.newestWithFiles(h.GameID); ok {
			e.PutBackFrom, e.PutBackFromTime = snap.ID, snap.Timestamp
		}
	}
	return e, true
}

// NewestSnapshotWithFiles is the newest snapshot of a game with any files in
// it that can be read back, on its current branch if that has one, or else
// on any: the save as it last was, for a game whose folder is now empty or
// gone.
func (d *Daemon) NewestSnapshotWithFiles(gameID string) (store.Snapshot, bool) {
	return d.newestWithFiles(gameID)
}

func (d *Daemon) newestWithFiles(gameID string) (store.Snapshot, bool) {
	game, err := d.Store.GetGame(gameID)
	if err != nil {
		return store.Snapshot{}, false
	}
	branches, _ := d.Store.ListBranches(gameID)
	order := []string{game.ActiveBranch}
	for _, b := range branches {
		if b != game.ActiveBranch {
			order = append(order, b)
		}
	}
	var best store.Snapshot
	for i, b := range order {
		snaps, _ := d.Store.ListSnapshots(gameID, b) // newest first
		for _, s := range snaps {
			if n, err := snapshot.ArchiveFileCount(s.ZipPath); err == nil && n > 0 && s.Problem == "" {
				if best.ID == "" || s.Timestamp > best.Timestamp {
					best = s
				}
				break
			}
		}
		if i == 0 && best.ID != "" {
			return best, true // the current branch's own, before any other's
		}
	}
	return best, best.ID != ""
}

// The two answers.
const (
	EmptiedDelete  = "delete"
	EmptiedPutBack = "restore"
)

// EmptiedAnswer is what an answer did.
type EmptiedAnswer struct {
	// Restored is the snapshot put back here, if any.
	Restored string `json:"restored,omitempty"`
	// Fetching is how many files are to come from the other devices, having
	// been in no snapshot here.
	Fetching int `json:"fetching"`
}

// ErrNotEmptied is an answer for a game that is not held back.
var ErrNotEmptied = errors.New("that game's save is not held back: nothing was emptied, or it has been answered already")

// AnswerEmptied acts on the answer to "every save file of this game was
// deleted here — was that meant?".
func (d *Daemon) AnswerEmptied(gameID, answer string) (EmptiedAnswer, error) {
	var out EmptiedAnswer
	game, err := d.Store.GetGame(gameID)
	if err != nil {
		return out, err
	}
	if e, held := d.EmptiedSaveOf(gameID); !held || e.State != store.HoldAsking {
		return out, ErrNotEmptied
	}
	switch answer {
	case EmptiedDelete:
		if err := d.P2P.Sync.ConfirmHold(gameID); err != nil {
			if errors.Is(err, syncengine.ErrNoHold) {
				return out, ErrNotEmptied
			}
			return out, err
		}
		d.Log.Log("info", fmt.Sprintf("the deletion of %q's save files goes to your other devices, which keep a snapshot of them first", game.Name))
	case EmptiedPutBack:
		if snap, ok := d.newestWithFiles(gameID); ok {
			if _, err := d.Snapshots.Restore(gameID, snap.ID); err != nil {
				return out, fmt.Errorf("could not put the files back from the snapshot of %s: %w", snap.Timestamp, err)
			}
			out.Restored = snap.ID
			d.P2P.Sync.RecordActivity(store.ActivityEvent{GameID: gameID, Kind: store.ActivityRestored,
				Detail: fmt.Sprintf("%s|%s", snap.ID, snap.Timestamp)})
		}
		n, err := d.P2P.Sync.PutBack(gameID)
		switch {
		case errors.Is(err, syncengine.ErrNoHold) && out.Restored != "":
			// The restore brought every file back, and the watcher saw it
			// and let the hold go first: the answer is already carried out.
		case errors.Is(err, syncengine.ErrNoHold):
			return out, ErrNotEmptied
		case err != nil:
			return out, err
		}
		out.Fetching = n
		switch {
		case out.Restored != "" && n > 0:
			d.Log.Log("success", fmt.Sprintf("put %q's save files back from a snapshot; %d more come from your other devices", game.Name, n))
		case out.Restored != "":
			d.Log.Log("success", fmt.Sprintf("put %q's save files back from a snapshot", game.Name))
		default:
			d.Log.Log("success", fmt.Sprintf("%q's save files come back from your other devices", game.Name))
		}
	default:
		return out, fmt.Errorf("answer %q or %q", EmptiedDelete, EmptiedPutBack)
	}
	if d.OnGameChanged != nil {
		d.OnGameChanged(gameID)
	}
	// Either way the game syncs again now, rather than at the next pass.
	d.P2P.GoSync(func(ctx context.Context) {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		defer cancel()
		_, _ = d.P2P.SyncGame(ctx, gameID)
	})
	return out, nil
}
