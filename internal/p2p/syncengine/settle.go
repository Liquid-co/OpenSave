package syncengine

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/opensave/opensave/internal/delta"
)

// A save part-way through being changed by a sync is not a state anyone has.
//
// A sync that pulls writes files one at a time, and for as long as it runs the
// folder holds some of the new files and some of the old — a mixture that
// existed on no device and that nobody chose. Read in that window, it describes
// a save that is not there:
//
//   - A device with an empty folder, pulling a game for the first time, was
//     asked for its files by the other device a moment later. It had 35 of
//     121, every one identical to the other side's. With no history between
//     them, "both hold files and they differ" is the rule for a conflict, and
//     one was raised over a save nobody had touched.
//   - The same with history: a device part-way through taking an update holds
//     some updated files and some not. That differs from the state both last
//     agreed on, as does the other side, so it reads as both having moved.
//
// Both are timing: they need the second sync to start while the first is
// writing, which a new device's first sync crossing the other device's own —
// its watcher, its reconcile, a click — does.
//
// So the rule is structural rather than per call site: a save is read for a
// sync only while no sync is writing it, and written only while no sync is
// reading it. Writers hold Writing for as long as they change files; readers
// hold Reading for as long as they walk them. Waiting for the writes to stop
// and then reading is not enough — the first version of this did that, and a
// pull that began while the manifest was being walked was caught half-way all
// the same.
//
// The helpers that write — applyLocalDeletions, createPulledDirsIn, pullFiles,
// touchSaveMtimes — each hold Writing themselves, so new code calling them is
// covered without having to know. A sync also holds it across its whole apply,
// because the gap between deleting and pulling is as much a mixture as the
// middle of a pull. Writes nest. Two things must never happen, and nothing
// here does either: taking Reading while holding Writing for the same game (it
// would wait on itself), and holding Reading across a request to a peer (the
// peer may be waiting for this device).

// SettlingMessage is what a device answers when asked for a save it is still
// writing, and what the asking side recognises (isSettling). It deliberately
// contains no "not found", which would read as the game not being tracked.
const SettlingMessage = "This device is part-way through applying a sync of this game; ask again once it has finished"

// ServeSettleWait is how long answering a peer waits for this device to finish
// writing before saying it is still busy. Under the 30 seconds either
// transport gives a request, so the answer arrives as "busy" rather than as a
// timeout the asking side cannot tell from a dead connection. The asker's next
// request waits again, so a long pull is waited out in steps. Var so tests can
// shrink it.
var ServeSettleWait = 15 * time.Second

// ErrSettling means a save was still being written when the wait ran out.
var ErrSettling = errors.New(SettlingMessage)

// isSettling reports whether an error from a peer is its "still writing"
// answer.
func isSettling(err error) bool {
	return err != nil && strings.Contains(err.Error(), SettlingMessage)
}

// saveGate is one game's readers and writers.
type saveGate struct {
	writers, waitingWriters, readers int
	// changed is closed, and replaced, whenever any count falls: the moment a
	// waiter's condition may have come true.
	changed chan struct{}
}

func (e *Engine) gateLocked(gameID string) *saveGate {
	if e.gates == nil {
		e.gates = map[string]*saveGate{}
	}
	g := e.gates[gameID]
	if g == nil {
		g = &saveGate{changed: make(chan struct{})}
		e.gates[gameID] = g
	}
	return g
}

// wakeLocked tells every waiter on the game to look again, and forgets the
// gate once nobody is using it.
func (e *Engine) wakeLocked(gameID string, g *saveGate) {
	close(g.changed)
	g.changed = make(chan struct{})
	if g.writers == 0 && g.waitingWriters == 0 && g.readers == 0 {
		delete(e.gates, gameID)
	}
}

// Writing marks the game's save as being changed on this device until done is
// called, first waiting for any sync reading it to finish. Hold it around
// anything that changes the files a sync reads — a pull, a deletion, a branch
// switch, a snapshot restore. Calls nest.
func (e *Engine) Writing(gameID string) (done func()) {
	e.settleMu.Lock()
	g := e.gateLocked(gameID)
	if g.writers == 0 {
		// Readers are short — a walk of the folder — so this waits for them
		// without a deadline. New ones queue behind it meanwhile.
		g.waitingWriters++
		for g.readers > 0 {
			ch := g.changed
			e.settleMu.Unlock()
			<-ch
			e.settleMu.Lock()
			g = e.gateLocked(gameID)
		}
		g.waitingWriters--
	}
	g.writers++
	e.settleMu.Unlock()

	var once bool
	return func() {
		e.settleMu.Lock()
		defer e.settleMu.Unlock()
		if once {
			return
		}
		once = true
		g := e.gateLocked(gameID)
		g.writers--
		if e.writtenAt == nil {
			e.writtenAt = map[string]time.Time{}
		}
		e.writtenAt[gameID] = time.Now()
		e.wakeLocked(gameID, g)
	}
}

// writeEcho is how long after a sync stops writing a game its file events may
// still be arriving.
const writeEcho = 10 * time.Second

// BeingWritten reports whether a sync on this device is writing a game's save
// now, or was a moment ago — what the watcher asks before taking a burst it
// could not attribute (an overflow of file events) for a change of the game's.
// A pull of a save of many files overflows the event queue again and again,
// and each one read as the game saving: an automatic snapshot of a half-pulled
// save every few seconds, which pushed the real ones out of retention.
func (e *Engine) BeingWritten(gameID string) bool {
	if e.WritingNow(gameID) {
		return true
	}
	e.settleMu.Lock()
	defer e.settleMu.Unlock()
	return time.Since(e.writtenAt[gameID]) < writeEcho
}

// WritingNow reports whether a sync on this device is writing a game's save at
// this moment — what the watcher waits out before looking at a burst, so it
// never judges a save half-way between two states. Not the moments after:
// a change the game makes then is the game's, and its snapshot is not held up.
func (e *Engine) WritingNow(gameID string) bool {
	e.settleMu.Lock()
	defer e.settleMu.Unlock()
	g := e.gates[gameID]
	return g != nil && (g.writers > 0 || g.waitingWriters > 0)
}

// Reading holds the game's save still for a sync to read it: it waits until
// nothing is writing it, then keeps writers out until done is called. It
// returns ErrSettling if ctx ends first. Hold it only for the reading itself —
// never across a request to a peer, and never while holding Writing.
func (e *Engine) Reading(ctx context.Context, gameID string) (done func(), err error) {
	e.settleMu.Lock()
	for {
		g := e.gateLocked(gameID)
		if g.writers == 0 && g.waitingWriters == 0 {
			g.readers++
			break
		}
		ch := g.changed
		e.settleMu.Unlock()
		select {
		case <-ch:
		case <-ctx.Done():
			return nil, ErrSettling
		}
		e.settleMu.Lock()
	}
	e.settleMu.Unlock()

	var once bool
	return func() {
		e.settleMu.Lock()
		defer e.settleMu.Unlock()
		if once {
			return
		}
		once = true
		g := e.gateLocked(gameID)
		g.readers--
		e.wakeLocked(gameID, g)
	}, nil
}

// TryReading is Reading without the wait: it holds the save still and says so
// if nothing is writing it, and otherwise takes nothing. For a check that
// runs on every change to the folder — the watcher's — where waiting would
// hold up everything behind it, and the writer's own changes bring the check
// round again once it has finished.
func (e *Engine) TryReading(gameID string) (done func(), ok bool) {
	e.settleMu.Lock()
	g := e.gateLocked(gameID)
	if g.writers > 0 || g.waitingWriters > 0 {
		e.settleMu.Unlock()
		return func() {}, false
	}
	g.readers++
	e.settleMu.Unlock()
	var once bool
	return func() {
		e.settleMu.Lock()
		defer e.settleMu.Unlock()
		if once {
			return
		}
		once = true
		g := e.gateLocked(gameID)
		g.readers--
		e.wakeLocked(gameID, g)
	}, true
}

// ReadManifest builds the manifest of one of the game's save folders while
// holding it still (Reading) — the way every sync reads a save it is not
// itself writing.
func (e *Engine) ReadManifest(ctx context.Context, gameID, path string) (delta.Manifest, error) {
	done, err := e.Reading(ctx, gameID)
	if err != nil {
		return delta.Manifest{}, err
	}
	defer done()
	return delta.BuildManifest(path)
}

// maxBusyFollowUps bounds how many times in a row a sync re-runs because a
// peer was still writing. Each re-run waits up to ServeSettleWait on the
// peer, so this is a few minutes of waiting in all; past it the change goes
// out on the next ordinary sync, and a device that never finishes cannot keep
// this one asking forever.
const maxBusyFollowUps = 12
