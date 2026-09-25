package syncengine

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/opensave/opensave/internal/delta"
	"github.com/opensave/opensave/internal/snapshot"
	"github.com/opensave/opensave/internal/store"
)

// Holding back a save that was emptied.
//
// A folder whose every save file went at once — an uninstaller, a game
// resetting its saves, someone clearing the wrong folder — reads to a sync
// as "every file deleted", and a sync passes deletions on: the other devices
// would delete their copies too, and the save would be gone everywhere at
// once. So a game with a save location that is empty here, which held files
// before, is held back: this device starts no sync of it (SyncGame answers
// ErrHeld) and gives no other device its manifest (the serving side answers
// HeldMessage, and the asking side skips it as peer_holding). Nothing is
// deleted anywhere until someone says what was meant, in the app or with
// `opensave emptied`:
//
//   - "Delete them there too" (ConfirmHold): the game syncs as it is; the
//     other devices delete their copies, keeping a snapshot first as they
//     always do.
//   - "Put them back" (PutBack): the newest snapshot with the files is
//     restored here, and anything it lacks is fetched from the other devices.
//     Until every file is here the other devices still see nothing of this
//     one: they judge deletions by their own record of what the two had in
//     common, which nothing here can change, and a folder still missing a
//     file would read to them as that file deleted.
//
// What the location "held before" is the files the other devices are
// recorded as having in common with this one, together with the files in
// this device's newest snapshot that has any. Not the first alone: that
// record lags — a file pushed to another device joins it only once that
// device has said it has the file — while the other device's own record,
// which is what it acts on, already has it. The snapshot is there before the
// deletion happens, so it is also there before any sync can see the empty
// folder, however quickly one comes.
//
// A hold lets go by itself when every file it covers is back — restored from
// the Recycle Bin, say — since nothing would be deleted any more. Not merely
// when the folder has something in it again: a game that wiped its saves and
// started a new one still has a question waiting about the old ones.
//
// Only an empty location counts. A few files deleted at once is how a game
// tidies up after itself; all of them is not.

// HeldMessage is what this device says to a manifest request for a game it
// is holding back. Shared by the serving side (internal/p2p) and isHeld here,
// as AwaitingFolderMessage is, and free of "not found" for the same reason.
const HeldMessage = "This device is holding this game back: its save files were all deleted there, and it is waiting to be told whether that was meant"

// ErrHeld is SyncGame declining to sync a game that is held back.
var ErrHeld = errors.New("its save files were all deleted here; it is not synced until you say whether that was meant")

// ErrNoHold is an answer to a question nobody asked: the game is not held.
var ErrNoHold = errors.New("that game is not held back")

func isHeld(err error) bool {
	return err != nil && strings.Contains(err.Error(), HeldMessage)
}

// emptiness is what a look at a game's save found.
type emptiness struct {
	// held is, for each save location that holds no files here, what it held
	// before; locations that hold files, or held none before, are absent.
	held map[string][]string
	// present is what each location holds here, and presentDirs its folders,
	// by location ("" is the main folder).
	present, presentDirs map[string]map[string]struct{}
}

// back says whether every file of a hold's is here again.
func (x emptiness) back(held map[string][]string) bool {
	for name, files := range held {
		for _, f := range files {
			if _, ok := x.present[name][f]; !ok {
				return false
			}
		}
	}
	return true
}

// emptiedLocations looks at a game's save for what a hold is about.
// Excluded files do not count on either side. A location that cannot be read
// — a folder not there at all — is not empty; it is missing, which the daemon
// deals with separately. A game that has never synced with another device has
// nothing to hold back from.
func (e *Engine) emptiedLocations(game store.Game) (x emptiness, err error) {
	x.held = map[string][]string{}
	extra, err := e.Store.GameRootPaths(game.ID)
	if err != nil {
		extra = nil
	}
	m, failures, err := delta.BuildMultiManifest(game.SavePath, extra)
	if err != nil {
		return x, err
	}
	rules := e.rulesFor(game.ID)
	m = filterManifest(m, rules)
	x.present = map[string]map[string]struct{}{delta.PrimaryRoot: keysOf(m.Files)}
	x.presentDirs = map[string]map[string]struct{}{delta.PrimaryRoot: setOf(m.Dirs)}
	for name, root := range m.Extra {
		x.present[name] = keysOf(root.Files)
		x.presentDirs[name] = setOf(root.Dirs)
	}

	empty := false
	for name, files := range x.present {
		if len(files) == 0 && failures[name] == nil {
			empty = true
		}
	}
	if !empty || !e.Store.HasSyncState(game.ID) {
		return x, nil // the usual case, answered without reading anything more
	}

	before, err := e.Store.SharedFiles(game.ID)
	if err != nil {
		return x, err
	}
	for name, files := range e.lastSavedFiles(game) {
		if before[name] == nil {
			before[name] = map[string]struct{}{}
		}
		for f := range files {
			before[name][f] = struct{}{}
		}
	}
	for name, had := range before {
		had = filterLineage(had, rules)
		here, known := x.present[name]
		if len(had) == 0 || !known || len(here) > 0 || failures[name] != nil {
			continue
		}
		list := make([]string, 0, len(had))
		for f := range had {
			list = append(list, f)
		}
		sort.Strings(list)
		x.held[name] = list
	}
	return x, nil
}

// lastSavedFiles is what the game's newest snapshot with any files held, by
// save location: what its save was just before it was emptied.
func (e *Engine) lastSavedFiles(game store.Game) map[string]map[string]struct{} {
	snaps, err := e.Store.ListSnapshots(game.ID, game.ActiveBranch) // newest first
	if err != nil {
		return nil
	}
	for _, s := range snaps {
		out := map[string]map[string]struct{}{}
		add := func(root, path string) {
			if path == "" || strings.HasSuffix(path, "/") || dotted(path) {
				return
			}
			if out[root] == nil {
				out[root] = map[string]struct{}{}
			}
			out[root][path] = struct{}{}
		}
		if files, err := e.Store.SnapshotFiles(s.ID); err == nil && len(files) > 0 {
			for _, f := range files {
				add(f.Root, f.Path)
			}
		} else if entries, err := snapshot.ArchiveEntries(s.ZipPath); err == nil {
			// Taken before snapshots recorded their files.
			for _, en := range entries {
				root, _ := snapshot.RootOfArchiveEntry(en.Name)
				add(root, snapshot.ArchiveEntryRelPath(en.Name))
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return nil
}

// dotted says whether a path is inside a dot-folder or is a dot-file, which
// a manifest leaves out.
func dotted(path string) bool {
	for _, seg := range strings.Split(path, "/") {
		if strings.HasPrefix(seg, ".") {
			return true
		}
	}
	return false
}

func setOf(list []string) map[string]struct{} {
	out := make(map[string]struct{}, len(list))
	for _, v := range list {
		out[v] = struct{}{}
	}
	return out
}

func keysOf[V any](m map[string]V) map[string]struct{} {
	out := make(map[string]struct{}, len(m))
	for k := range m {
		out[k] = struct{}{}
	}
	return out
}

// CheckHold says whether a game is held back now, noticing a new hold and
// letting go of one that no longer applies. serving is true when another
// device is asking for this game's manifest, false when this device is about
// to start a sync of it — the two differ once the answer was "put them
// back", while the files are still being fetched.
func (e *Engine) CheckHold(gameID string, serving bool) (held bool, err error) {
	e.holdMu.Lock()
	defer e.holdMu.Unlock()
	game, err := e.Store.GetGame(gameID)
	if err != nil {
		return false, err
	}
	hold, has, err := e.Store.GetDeletionHold(gameID)
	if err != nil {
		return false, err
	}
	x, err := e.emptiedLocations(game)
	if err != nil {
		// Cannot tell: a hold already waiting keeps waiting; otherwise this
		// is left to the sync, which fails on the same unreadable folder.
		return has && hold.State != store.HoldConfirmed && (serving || hold.State == store.HoldAsking), nil
	}

	switch {
	case has && hold.State == store.HoldConfirmed:
		// The deletion was meant. Once there are files again the next
		// emptying is a new question.
		if len(x.held) == 0 {
			_ = e.Store.ClearDeletionHold(gameID)
		}
		return false, nil

	case has:
		if x.back(hold.Files()) {
			_ = e.Store.ClearDeletionHold(gameID)
			if hold.State == store.HoldAsking {
				e.Log("success", fmt.Sprintf("every save file of %q is back, so nothing is held back any more", game.Name))
			} else {
				e.Log("success", fmt.Sprintf("every save file of %q is back; syncing it with your other devices again", game.Name))
			}
			e.holdChanged(gameID)
			return false, nil
		}
		return serving || hold.State == store.HoldAsking, nil

	case len(x.held) > 0:
		if err := e.Store.SetDeletionHold(gameID, store.HoldAsking, time.Now().UnixMilli(), x.held); err != nil {
			return false, err
		}
		names := make([]string, 0, len(x.held))
		for name := range x.held {
			names = append(names, name)
		}
		sort.Strings(names)
		e.Log("warn", fmt.Sprintf("every save file of %q was deleted on this device (%s) — it is not synced until you say whether that was meant, "+
			"so your other devices keep their copies meanwhile", game.Name, describeLocations(names)))
		files := 0
		for _, f := range x.held {
			files += len(f)
		}
		e.RecordActivity(store.ActivityEvent{GameID: gameID, Kind: store.ActivityEmptied, Files: files})
		e.holdChanged(gameID)
		return true, nil
	}
	return false, nil
}

// ConfirmHold records the answer "delete them on the other devices too".
func (e *Engine) ConfirmHold(gameID string) error {
	e.holdMu.Lock()
	defer e.holdMu.Unlock()
	hold, has, err := e.Store.GetDeletionHold(gameID)
	if err != nil {
		return err
	}
	if !has || hold.State != store.HoldAsking {
		return ErrNoHold
	}
	if err := e.Store.SetDeletionHoldState(gameID, store.HoldConfirmed); err != nil {
		return err
	}
	e.holdChanged(gameID)
	return nil
}

// PutBack records the answer "put them back", after the newest snapshot with
// the files has been restored here, if there was one. Whatever that did not
// bring back is still on the other devices, and still recorded here as shared
// and as deleted — which is what a sync from this side reads as "delete it
// there". For those files both records go, so the other device's copy reads
// as one this device has not got, and the next sync fetches it; until they
// are all here, the other devices are still shown nothing (see CheckHold).
// Returns how many files are to be fetched.
func (e *Engine) PutBack(gameID string) (fetching int, err error) {
	e.holdMu.Lock()
	defer e.holdMu.Unlock()
	hold, has, err := e.Store.GetDeletionHold(gameID)
	if err != nil {
		return 0, err
	}
	if !has || hold.State != store.HoldAsking {
		return 0, ErrNoHold
	}
	game, err := e.Store.GetGame(gameID)
	if err != nil {
		return 0, err
	}
	x, err := e.emptiedLocations(game)
	if err != nil {
		return 0, err
	}
	for root, files := range hold.Files() {
		var missing []string
		for _, f := range files {
			_ = e.Store.ClearDeletedFile(gameID, root, f)
			if _, ok := x.present[root][f]; !ok {
				missing = append(missing, f)
			}
		}
		fetching += len(missing)
		if err := e.Store.ForgetSharedPaths(gameID, root, missing, x.presentDirs[root]); err != nil {
			return 0, err
		}
	}
	if fetching == 0 {
		err = e.Store.ClearDeletionHold(gameID)
	} else {
		err = e.Store.SetDeletionHoldState(gameID, store.HoldFetching)
	}
	if err != nil {
		return 0, err
	}
	e.holdChanged(gameID)
	return fetching, nil
}

// noteEmptiedByPeer runs after this device deleted files because another
// device did. If that left a location empty, the deletion was decided over
// there — by someone who confirmed it, or on a device that predates holding
// back — and this device must not ask again and hold up the devices that have
// not heard yet. started is when the deletion began: a hold noticed since
// (the watcher seeing the files go) was noticing this.
func (e *Engine) noteEmptiedByPeer(gameID string, started time.Time) {
	e.holdMu.Lock()
	defer e.holdMu.Unlock()
	hold, has, err := e.Store.GetDeletionHold(gameID)
	if err != nil || (has && hold.State != store.HoldConfirmed && hold.SinceMs < started.UnixMilli()) {
		return // a question asked before, about this device's own deletion
	}
	game, err := e.Store.GetGame(gameID)
	if err != nil {
		return
	}
	x, err := e.emptiedLocations(game)
	if err != nil || len(x.held) == 0 {
		return
	}
	_ = e.Store.SetDeletionHold(gameID, store.HoldConfirmed, time.Now().UnixMilli(), x.held)
	if has && hold.State != store.HoldConfirmed {
		e.holdChanged(gameID)
	}
}

// emptiedUnconfirmed is the other half of holding back, on the device that
// would do the deleting: a peer's save location that is empty, where this
// sync would delete files here, and that the peer has not said was emptied on
// purpose. The emptied device holds itself back when it knows what the
// location held; this catches the moment it cannot know yet — a file written
// and the folder emptied before a snapshot or the record of what the two
// share had caught up with it — and a device from before holding back
// existed, which sends every empty folder as it is.
func emptiedUnconfirmed(remoteFiles map[string]delta.FileEntry, d Decision, confirmed bool) bool {
	return len(remoteFiles) == 0 && len(d.FilesToDeleteLocally) > 0 && !confirmed
}

// handOverEmptying sends a confirmed emptying on as one step. The ordinary
// way a deletion leaves this device is a request per file, and the other
// device's own syncs carry on meanwhile: one that lands between two requests
// sees its save half deleted and this one wholly, reads that as both sides
// having changed, and stops on a conflict nobody caused. So for a location
// emptied on purpose, the requests are not sent; the other device is asked to
// sync instead, and deletes every file in one pass of its own, which nothing
// can land in the middle of. Reports whether it did that.
func (e *Engine) handOverEmptying(gameID string, peer Peer, localFiles map[string]delta.FileEntry, d *Decision) bool {
	if len(localFiles) > 0 || len(d.FilesToDeleteOnPeer) == 0 || !e.DeletionConfirmed(gameID) {
		return false
	}
	d.FilesToDeleteOnPeer = nil
	d.DirsToDeleteOnPeer = nil
	e.Transport.TriggerPeerPull(peer, gameID)
	return true
}

// DeletionConfirmed says whether an empty save of this game was emptied on
// purpose, for the manifest this device serves (ManifestResponse).
func (e *Engine) DeletionConfirmed(gameID string) bool {
	hold, has, err := e.Store.GetDeletionHold(gameID)
	return err == nil && has && hold.State == store.HoldConfirmed
}

// NoteEmptiedByPeer is noteEmptiedByPeer for the serving side, which deletes
// files another device asked it to — one request per file. So it runs a
// moment after the last of a batch, once, and never inside a request: the
// other device sends its deletions one after another, and every moment added
// to each is a moment for a sync from this side to land between two of them
// and see the save half deleted.
//
// It also keeps the batch in the activity history as one event: device is
// the device that asked.
func (e *Engine) NoteEmptiedByPeer(gameID, device string, started time.Time) {
	e.noteMu.Lock()
	defer e.noteMu.Unlock()
	if e.noting == nil {
		e.noting = map[string]*peerDeletions{}
	}
	if batch, pending := e.noting[gameID]; pending {
		if started.Before(batch.first) {
			batch.first = started
		}
		batch.files++
		return
	}
	e.noting[gameID] = &peerDeletions{first: started, device: device, files: 1}
	time.AfterFunc(noteDelay, func() {
		e.noteMu.Lock()
		batch := e.noting[gameID]
		delete(e.noting, gameID)
		e.noteMu.Unlock()
		e.RecordActivity(store.ActivityEvent{GameID: gameID, Kind: store.ActivityDeleted, Device: batch.device, Files: batch.files})
		e.noteEmptiedByPeer(gameID, batch.first)
	})
}

// peerDeletions is a batch of deletions another device asked for.
type peerDeletions struct {
	first  time.Time
	device string
	files  int
}

// noteDelay is how long NoteEmptiedByPeer waits for the rest of a batch.
const noteDelay = 300 * time.Millisecond

func (e *Engine) holdChanged(gameID string) {
	if e.OnHoldChanged != nil {
		e.OnHoldChanged(gameID)
	}
}

// describeLocations names emptied locations for a log line.
func describeLocations(names []string) string {
	parts := make([]string, 0, len(names))
	for _, n := range names {
		if n == delta.PrimaryRoot {
			parts = append(parts, "its save folder")
		} else {
			parts = append(parts, fmt.Sprintf("its %q location", n))
		}
	}
	return strings.Join(parts, " and ")
}
