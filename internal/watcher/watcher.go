// Package watcher monitors tracked games' save locations and triggers
// auto-snapshots (and then sync) when they change, porting
// src/daemon/watcher.js with the resilience fixes from its author's
// walkthrough notes:
//
//   - Single-file saves are watched via their PARENT directory with events
//     filtered to the target file, so safe-write games (write temp file,
//     delete original, rename temp over it) don't break the watch.
//   - A changed save is only snapshotted once its files stop being locked
//     ("gameplay guard": poll every 5s while the game is still writing).
//   - Locked means an OS sharing violation only — read-only/permission
//     errors do not hang the guard loop.
//   - Before snapshotting, the current manifest hash is compared against
//     the hash recorded at the previous auto-snapshot, so restoring a
//     snapshot or receiving a sync doesn't feed back into another
//     snapshot of identical content.
package watcher

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/opensave/opensave/internal/delta"
	"github.com/opensave/opensave/internal/ignore"
)

const (
	// watchBufferBytes is the per-directory event buffer fsnotify allocates.
	//
	// Its default is 64 KB, and the allocation is per WATCHED DIRECTORY, not
	// per game — addRecursive registers every subfolder of every save
	// location. Someone tracking 350 games across a few folders each was
	// therefore holding hundreds of megabytes in buffers alone before a
	// single save was read, which is most of what "OpenSave is using 760 MB
	// while idle" turned out to be.
	//
	// 8 KB is still around a hundred events in flight for one folder. Save
	// folders are not high-event places: a game writes a handful of files
	// when it saves, not thousands per second. The cost of getting this wrong
	// is a dropped-event overflow, which is now noticed and recovered from
	// rather than ignored — see ErrEventOverflow below.
	watchBufferBytes = 8 * 1024

	debounceDelay      = 2 * time.Second
	guardPollInterval  = 5 * time.Second
	snapshotMaxRetries = 5
	snapshotRetryDelay = 1500 * time.Millisecond
)

// reconcileInterval is how often every watched game is checked against the
// state the watcher believes it is in.
//
// A watch reports changes; it does not guarantee it saw them all. Events are
// dropped when a burst overruns the buffer, a folder created during one can
// end up watched by nobody, and a change landing between the last event and
// the debounce firing raises nothing further. All of those leave a live watch
// whose baseline no longer describes the folder — and because every correction
// was itself driven by an event, nothing will ever fix it. Measured at four
// minutes with no sign of recovering; the only reason it was not four hours is
// that the test gave up.
//
// That baseline decides whether a save holds changes a pull might overwrite,
// so a stale one is wrong in the direction that costs someone a save.
//
// The cost is real and worth stating plainly, because it is the same cost this
// package spent a release removing. Every pass walks each watched tree; with
// the hash cache that walk stats files rather than reading them, but the cache
// also forces a genuine re-read of anything it has held for cacheMaxAge (an
// hour), so a reconcile running forever guarantees one full read of every
// tracked save per hour. Before this, an idle game with no events was never
// walked at all.
//
// Fifteen minutes is the compromise. It turns "wrong until something else
// happens to touch this folder" into "wrong for at most a quarter of an hour",
// which is the difference that matters, while keeping the walk rare enough not
// to reinstate the constant disk activity people reported. Shorten it and the
// walking becomes noticeable on a large library; lengthen it and a dropped
// event costs more time holding a baseline that is wrong.
//
// A variable rather than a constant so tests can drive it; nothing else writes
// to it.
var reconcileInterval = 15 * time.Minute

// Callbacks connect the watcher to the rest of the daemon without import
// cycles. All are required.
type Callbacks struct {
	// IgnoreRules returns a game's exclusion list (the .gitignore-style text).
	// The recorded hash covers everything EXCEPT excluded paths, because the
	// sync engine compares against the same value to decide whether the save
	// holds changes a pull might overwrite — and a pull can never overwrite a
	// file it is not syncing. Counting excluded files here would make that
	// check fire on every pull, for a game whose config is simply newer than
	// its last snapshot.
	//
	// The cost is that editing ONLY an excluded file does not itself trigger
	// an automatic snapshot; it is still captured by the next snapshot taken
	// for any other reason. Optional: nil means nothing is excluded.
	IgnoreRules func(gameID string) string
	// GetLastManifestHash returns the hash recorded at the previous
	// auto-snapshot (empty string if none).
	GetLastManifestHash func(gameID string) (string, error)
	// SetLastManifestHash records the hash for the snapshot just taken.
	SetLastManifestHash func(gameID, hash string) error
	// CreateSnapshot takes the auto-snapshot. Retried on failure.
	CreateSnapshot func(gameID string) error
	// OnChanged fires after a successful auto-snapshot (the daemon uses it
	// to kick off P2P sync). Optional-in-behavior: may be nil.
	OnChanged func(gameID string)
	// Log receives human-readable watcher activity. May be nil.
	Log func(level, msg string)
}

// Engine owns one watch goroutine per tracked game.
type Engine struct {
	cb Callbacks

	mu     sync.Mutex
	games  map[string]*gameWatch
	closed bool

	// catchUp carries games that have just come under watch and need checking
	// against their last recorded snapshot. See catchUpWorker.
	catchUp    chan catchUpJob
	catchUpCtx context.Context
	stopCatch  context.CancelFunc

	// reconcileEvery is this engine's copy of reconcileInterval, read once
	// when it is built.
	//
	// A field rather than the package variable read from inside the worker,
	// because that read would happen on the new goroutine while whoever set
	// the variable carries on — a test restoring the real interval after
	// starting an engine writes it concurrently with that read. Starting a
	// goroutine orders the writes that came BEFORE it and nothing after, so
	// that is a data race, and one only the race detector would ever show.
	reconcileEvery time.Duration
}

// catchUpJob is one game to check after its watch starts.
type catchUpJob struct {
	gameID   string
	savePath string
	extra    map[string]string
	isFile   bool
}

type gameWatch struct {
	gameID   string
	savePath string
	// extra maps a save location's name to its path on this device, for the
	// games whose save is split across more than one folder. Watched the same
	// way the main folder is: a change in any of them is a change to the game,
	// and without this a settings folder would only ever sync when the user
	// pressed the button.
	extra  map[string]string
	isFile bool
	fsw    *fsnotify.Watcher
	cancel context.CancelFunc
	done   chan struct{}
	// log is the engine's logger, held here so stop() can report a watch
	// that refused to exit. Nil in tests that build a gameWatch directly.
	log func(level, msg string)

	// rewatch records that some folder under this game is known NOT to be
	// watched, so the next pass re-registers everything.
	//
	// Registering a new subfolder happens once, on its Create event, and
	// nothing repeated it: if that Add failed — a permission, a race with
	// shutdown, a limit — the folder stayed unwatched for the life of the
	// process, silently, because the error was discarded. A watch that is
	// missing raises no events to tell you it is missing, which is what makes
	// this class of failure invisible.
	//
	// Only ever written from the watch's own goroutine, so no lock is needed.
	rewatch bool
}

// New creates a watcher Engine.
func New(cb Callbacks) *Engine {
	ctx, cancel := context.WithCancel(context.Background())
	e := &Engine{
		cb:    cb,
		games: map[string]*gameWatch{},
		// Buffered so starting a large library never blocks on the worker.
		// Dropping a catch-up when the queue is full is safe: the next change
		// to that game snapshots it anyway, and the periodic watch reconcile
		// re-queues it.
		catchUp:    make(chan catchUpJob, 256),
		catchUpCtx: ctx,
		stopCatch:  cancel,
		// Read here, on the caller's goroutine, not inside the worker.
		reconcileEvery: reconcileInterval,
	}
	go e.catchUpWorker()
	go e.reconcileWorker()
	return e
}

// reconcileWorker re-checks every watched game on a timer.
//
// It queues the same job a starting watch queues, so everything catchUpOne is
// careful about applies unchanged: games with no baseline are left alone, the
// checks run one at a time rather than reading every save at once, and a game
// whose folder still matches its baseline produces nothing at all. The only
// difference is what prompts it.
func (e *Engine) reconcileWorker() {
	ticker := time.NewTicker(e.reconcileEvery)
	defer ticker.Stop()
	for {
		select {
		case <-e.catchUpCtx.Done():
			return
		case <-ticker.C:
			for _, job := range e.watchedJobs() {
				e.queueCatchUp(job)
			}
		}
	}
}

// watchedJobs describes every live watch, for the reconcile.
//
// Collected under the lock and queued outside it: queueCatchUp never blocks,
// but holding the engine lock across a loop over every game is how Watch and
// Unwatch end up waiting on unrelated work.
func (e *Engine) watchedJobs() []catchUpJob {
	e.mu.Lock()
	defer e.mu.Unlock()
	jobs := make([]catchUpJob, 0, len(e.games))
	for _, gw := range e.games {
		jobs = append(jobs, catchUpJob{
			gameID:   gw.gameID,
			savePath: gw.savePath,
			extra:    gw.extra,
			isFile:   gw.isFile,
		})
	}
	return jobs
}

// catchUpWorker snapshots games that changed while nobody was watching.
//
// Starting a watch is not retroactive: it reports what happens next, and says
// nothing about what happened while it was not running. A game saved while
// OpenSave was closed, or while its watch was down after a failed start, was
// therefore left with no snapshot of that state — the change sat on disk, and
// the local history skipped it until the game happened to save again. The
// peer reconcile would still carry the files to another device, so this was
// never lost data; what was missing was the snapshot you would restore from,
// which is the thing people reach for when something goes wrong.
//
// The check is the same one a change triggers: build the manifest, compare it
// to the hash recorded at the last auto-snapshot, and take one if they differ.
// Unchanged games cost a walk and nothing else.
//
// Deliberately one at a time. Three hundred games catching up at once would
// read every save at once, on a machine that has just started — exactly the
// disk storm this program has been trying to stop making.
func (e *Engine) catchUpWorker() {
	for {
		select {
		case <-e.catchUpCtx.Done():
			return
		case job := <-e.catchUp:
			e.catchUpOne(job)
		}
	}
}

// catchUpOne checks one game against the hash recorded at its last
// auto-snapshot, and snapshots it if the folder has moved on.
//
// Only for games that already have a recorded hash. Writing a baseline for one
// that has none looks harmless and is not: the write happens on this worker,
// concurrently with whatever the user is doing, so a save made in that instant
// would be recorded as the baseline and the change it represents would never
// be snapshotted. Suppressing a real snapshot to gain a nominal one is the
// wrong trade in a program whose job is keeping copies — so a game with no
// baseline is left alone, and gets one from its first ordinary change.
func (e *Engine) catchUpOne(job catchUpJob) {
	if e.cb.GetLastManifestHash == nil {
		return
	}
	last, err := e.cb.GetLastManifestHash(job.gameID)
	if err != nil || last == "" {
		return
	}
	// A baseline exists, so the ordinary change path answers the question: it
	// compares against that hash and snapshots only if they differ. Worst case
	// it races a real change and takes one snapshot too many, which is the
	// harmless direction.
	e.handleChange(e.catchUpCtx, &gameWatch{
		gameID:   job.gameID,
		savePath: job.savePath,
		extra:    job.extra,
		isFile:   job.isFile,
		log:      e.cb.Log,
	})
}

// queueCatchUp asks for a game to be checked, without blocking.
func (e *Engine) queueCatchUp(job catchUpJob) {
	select {
	case e.catchUp <- job:
	default:
		// Queue full: skipped rather than waited on. The next change to this
		// game snapshots it, and the reconcile will offer it again.
	}
}

// Watch starts (or restarts) watching a game's save location.
//
// The expensive part — the recursive walk that registers every subfolder —
// happens BEFORE taking the engine lock: a big tree must never block
// Watch/Unwatch calls for other games (a wedged engine can otherwise only
// be fixed by restarting the app).
func (e *Engine) Watch(gameID, savePath string) error {
	return e.WatchWithLocations(gameID, savePath, nil)
}

// WatchWithLocations watches a game's main save folder plus any extra save
// locations it has, by name.
//
// One fsnotify watcher covers all of them: an event in any location means the
// game changed, and what follows — snapshot, then sync — is the same work
// whichever folder it came from.
func (e *Engine) WatchWithLocations(gameID, savePath string, extra map[string]string) error {
	isFile, err := delta.ResolveLocalSaveFilePath(savePath)
	if err != nil {
		return fmt.Errorf("inspect save path: %w", err)
	}

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create fs watcher: %w", err)
	}

	// Single-file saves: watch the parent directory (survives the file
	// being unlinked+recreated by safe-write); directory saves: watch the
	// tree recursively (fsnotify is non-recursive by itself).
	if isFile {
		if err := fsw.Add(filepath.Dir(savePath)); err != nil {
			fsw.Close()
			return fmt.Errorf("watch parent dir: %w", err)
		}
	} else {
		if err := os.MkdirAll(savePath, 0o777); err != nil {
			fsw.Close()
			return fmt.Errorf("create save dir: %w", err)
		}
		if err := addRecursive(context.Background(), fsw, savePath); err != nil {
			fsw.Close()
			return fmt.Errorf("watch save dir tree: %w", err)
		}
	}

	// A location that cannot be watched is logged and skipped rather than
	// failing the game: losing automatic snapshots of a mods folder is not a
	// reason to stop watching the save.
	watched := map[string]string{}
	for name, path := range extra {
		if path == "" {
			continue
		}
		if err := os.MkdirAll(path, 0o777); err != nil {
			e.log("warn", fmt.Sprintf("cannot watch the %q save location of %s: %v", name, gameID, err))
			continue
		}
		if err := addRecursive(context.Background(), fsw, path); err != nil {
			e.log("warn", fmt.Sprintf("cannot watch the %q save location of %s: %v", name, gameID, err))
			continue
		}
		watched[name] = path
	}

	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		fsw.Close()
		return fmt.Errorf("watcher engine is stopped")
	}
	if existing, ok := e.games[gameID]; ok {
		delete(e.games, gameID)
		e.mu.Unlock()
		existing.stop() // may wait on the old run goroutine — not under the lock
		e.mu.Lock()
	}

	ctx, cancel := context.WithCancel(context.Background())
	gw := &gameWatch{
		gameID:   gameID,
		savePath: savePath,
		extra:    watched,
		isFile:   isFile,
		fsw:      fsw,
		cancel:   cancel,
		done:     make(chan struct{}),
		log:      e.log,
	}
	e.games[gameID] = gw
	e.mu.Unlock()
	go e.run(ctx, gw)

	e.log("info", fmt.Sprintf("watching %q (%s mode)", savePath, map[bool]string{true: "single-file", false: "directory"}[isFile]))

	// A watch reports what happens next, not what already happened. Check
	// whether this folder changed while nobody was watching it — after a
	// restart, or after a watch that failed to start earlier and has just
	// been retried — and snapshot it if so.
	e.queueCatchUp(catchUpJob{gameID: gameID, savePath: savePath, extra: watched, isFile: isFile})
	return nil
}

// Watching reports whether a game is currently being watched, and at which
// path. Reconciling the watch set against the database needs this: Watch
// replaces an existing watch outright — stopping its goroutine and re-adding
// every fsnotify registration — so re-watching everything to pick up one new
// game would tear down and rebuild all the others for nothing.
func (e *Engine) Watching(gameID string) (savePath string, ok bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	gw, ok := e.games[gameID]
	if !ok {
		return "", false
	}
	return gw.savePath, true
}

// WatchedGames returns the ids currently being watched.
func (e *Engine) WatchedGames() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	ids := make([]string, 0, len(e.games))
	for id := range e.games {
		ids = append(ids, id)
	}
	return ids
}

// Unwatch stops watching a game. The (possibly slow) wait for the watch
// goroutine happens outside the engine lock so other games' operations
// are never blocked.
func (e *Engine) Unwatch(gameID string) {
	e.mu.Lock()
	gw, ok := e.games[gameID]
	if ok {
		delete(e.games, gameID)
	}
	e.mu.Unlock()
	if ok {
		gw.stop()
	}
}

// Stop shuts down every watch goroutine.
func (e *Engine) Stop() {
	if e.stopCatch != nil {
		e.stopCatch()
	}
	e.mu.Lock()
	e.closed = true
	stopping := make([]*gameWatch, 0, len(e.games))
	for id, gw := range e.games {
		stopping = append(stopping, gw)
		delete(e.games, id)
	}
	e.mu.Unlock()
	for _, gw := range stopping {
		gw.stop()
	}
}

// watchStopTimeout bounds how long stopping one watch may wait for its run
// loop to exit. Generous next to the work the loop does between select turns,
// and short enough that a user quitting the app does not sit looking at a
// window that will not close.
const watchStopTimeout = 5 * time.Second

func (gw *gameWatch) stop() {
	// Cancel BEFORE closing, so the guard in addRecursive sees the cancelled
	// context and stops issuing new Adds. Closing is what actually wakes the
	// loop — it closes the Events channel, which the select treats as "the
	// watcher is gone, return" — so it cannot simply be dropped: without it
	// the loop keeps grinding through its event backlog and takes far longer
	// to notice it should stop.
	gw.cancel()
	gw.fsw.Close()

	// Bounded regardless. The ordering above removes the known way to wedge
	// this, but it cannot make the window vanish: cancel() can still land
	// between addRecursive's check and the Add it guards. stop() is on the
	// path that quits the application, where a wait that never ends is a
	// window that never closes — so this guarantee is worth holding on its
	// own, independently of the bug that prompted it.
	//
	// Giving up leaks a goroutine and a watcher handle. Against a process that
	// never exits that is the right trade: the leak lasts only as long as the
	// process, and stopping is nearly always the last thing it does.
	select {
	case <-gw.done:
	case <-time.After(watchStopTimeout):
		if gw.log != nil {
			gw.log("warn", fmt.Sprintf(
				"the watcher for %s did not stop within %s and was abandoned — "+
					"its goroutine is left running; this is a bug, but shutting down "+
					"matters more than waiting for it", gw.gameID, watchStopTimeout))
		}
	}
}

// run is the per-game event loop: filter -> debounce -> guard -> snapshot.
func (e *Engine) run(ctx context.Context, gw *gameWatch) {
	defer close(gw.done)

	var debounce *time.Timer
	var debounceC <-chan time.Time

	for {
		select {
		case <-ctx.Done():
			return

		case event, ok := <-gw.fsw.Events:
			if !ok {
				return
			}
			if !gw.eventRelevant(event) {
				continue
			}
			// New subdirectory in directory mode: extend the watch.
			if !gw.isFile && event.Has(fsnotify.Create) {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					if addErr := addRecursive(ctx, gw.fsw, event.Name); addErr != nil && ctx.Err() == nil {
						// Not fatal, and not ignorable either: this folder is
						// now invisible to the watcher. Remember to re-register
						// on the next pass rather than discovering it never
						// again.
						gw.rewatch = true
						e.log("warn", fmt.Sprintf(
							"could not watch new folder %q for %q (%v) — will retry",
							event.Name, gw.gameID, addErr))
					}
				}
			}
			if debounce == nil {
				debounce = time.NewTimer(debounceDelay)
				debounceC = debounce.C
			} else {
				if !debounce.Stop() {
					select {
					case <-debounce.C:
					default:
					}
				}
				debounce.Reset(debounceDelay)
			}

		case err, ok := <-gw.fsw.Errors:
			if !ok {
				return
			}
			// Errors were being discarded entirely, which mattered most for
			// the one that means "events were dropped": after an overflow we
			// know something changed and not what, so ignoring it loses the
			// change silently. Treat it as a change and let the debounced
			// handler rescan — the manifest comparison then finds whatever
			// the missed events would have told us.
			if errors.Is(err, fsnotify.ErrEventOverflow) {
				e.log("warn", fmt.Sprintf(
					"file events overflowed for %q — rescanning to find what changed", gw.gameID))
				// Re-register the folders as well as rescanning them.
				//
				// Almost everything the loop does with an event is to use it
				// as a trigger — handleChange reads the folder from disk, so
				// which event arrived does not matter. The exception is above:
				// a Create for a new subdirectory is what puts that
				// subdirectory under watch. Lose that one event and the folder
				// is never watched, so nothing inside it ever raises another —
				// a rescan would find today's contents and then go quiet
				// again. Re-adding is idempotent: fsnotify keeps one watch per
				// directory and allocates no second buffer for one it already
				// has.
				gw.rewatch = true
			} else {
				e.log("warn", fmt.Sprintf("watching %q: %v", gw.gameID, err))
				continue
			}
			if debounce == nil {
				debounce = time.NewTimer(debounceDelay)
				debounceC = debounce.C
			} else {
				if !debounce.Stop() {
					select {
					case <-debounce.C:
					default:
					}
				}
				debounce.Reset(debounceDelay)
			}

		case <-debounceC:
			debounce = nil
			debounceC = nil
			// Re-register before reading, so a folder that went unwatched is
			// both found now and watched from here on. Doing only one of those
			// leaves it correct today and silent tomorrow.
			if gw.rewatch {
				gw.rewatch = false
				if err := addRecursive(ctx, gw.fsw, gw.savePath); err != nil && ctx.Err() == nil {
					gw.rewatch = true
					e.log("warn", fmt.Sprintf("re-watching %q: %v", gw.gameID, err))
				}
				for _, extra := range gw.extra {
					if err := addRecursive(ctx, gw.fsw, extra); err != nil && ctx.Err() == nil {
						gw.rewatch = true
					}
				}
			}
			// The filesystem said these folders changed, which is better
			// evidence than any stamp comparison — so drop their cached
			// hashes and let handleChange read what is actually on disk.
			//
			// The whole root, not just the paths named in the events: a
			// rename moves a subtree, and a delete-then-recreate can land on
			// the same size and modification time.
			//
			// Here rather than on each event, because one burst of writes
			// fires many events and each invalidation scans the cache. Once
			// per burst is the same guarantee for a fraction of the work, and
			// it still lands before anything reads. Idle folders produce no
			// events at all, so a quiet game keeps its cache.
			delta.InvalidateRoot(gw.savePath)
			for _, path := range gw.extra {
				delta.InvalidateRoot(path)
			}
			e.handleChange(ctx, gw)
		}
	}
}

// eventRelevant filters raw fs events: dotfiles are ignored everywhere; in
// single-file mode only events on the save file itself count (compared
// case-insensitively on Windows via EqualFold — save paths there are
// case-preserving but insensitive).
func (gw *gameWatch) eventRelevant(event fsnotify.Event) bool {
	name := filepath.Base(event.Name)
	if strings.HasPrefix(name, ".") {
		return false
	}
	if strings.HasSuffix(name, ".opensave.tmp") {
		return false
	}
	if gw.isFile {
		return strings.EqualFold(
			filepath.Clean(event.Name),
			filepath.Clean(gw.savePath),
		)
	}
	return true
}

// handleChange runs after the debounce window: wait out any file locks
// (gameplay guard), skip if content is unchanged since the last
// auto-snapshot, then snapshot with retries and notify.
func (e *Engine) handleChange(ctx context.Context, gw *gameWatch) {
	// Gameplay guard: the game may still be mid-write.
	for anyFileLocked(gw.savePath) {
		e.log("info", fmt.Sprintf("save files for %q are in use; waiting (gameplay guard)", gw.gameID))
		select {
		case <-ctx.Done():
			return
		case <-time.After(guardPollInterval):
		}
	}

	manifest, failures, err := delta.BuildMultiManifest(gw.savePath, gw.extra)
	if err != nil {
		e.log("warn", fmt.Sprintf("manifest build failed for %q: %v", gw.gameID, err))
		return
	}
	for name, failure := range failures {
		e.log("warn", fmt.Sprintf("cannot read the %q save location of %s: %v", name, gw.gameID, failure))
	}
	// ContentHash, not ManifestHash: this asks "did anything about this game
	// change", which has to cover every one of its folders. For a game with
	// one folder the two are the same value, so nothing already recorded is
	// invalidated by the upgrade.
	if e.cb.IgnoreRules != nil {
		if rules := ignore.Parse(e.cb.IgnoreRules(gw.gameID)); !rules.Empty() {
			manifest = filterForHash(manifest, rules)
		}
	}
	currentHash := manifest.ContentHash()

	lastHash, err := e.cb.GetLastManifestHash(gw.gameID)
	if err == nil && lastHash == currentHash {
		e.log("info", fmt.Sprintf("no content change for %q; skipping auto-snapshot", gw.gameID))
		return
	}

	for attempt := 1; attempt <= snapshotMaxRetries; attempt++ {
		err = e.cb.CreateSnapshot(gw.gameID)
		if err == nil {
			break
		}
		e.log("warn", fmt.Sprintf("auto-snapshot failed for %q (attempt %d/%d): %v", gw.gameID, attempt, snapshotMaxRetries, err))
		select {
		case <-ctx.Done():
			return
		case <-time.After(snapshotRetryDelay):
		}
	}
	if err != nil {
		e.log("error", fmt.Sprintf("auto-snapshot permanently failed for %q: %v", gw.gameID, err))
		return
	}

	if err := e.cb.SetLastManifestHash(gw.gameID, currentHash); err != nil {
		e.log("warn", fmt.Sprintf("failed to record manifest hash for %q: %v", gw.gameID, err))
	}
	e.log("success", fmt.Sprintf("auto-snapshot created for %q", gw.gameID))

	if e.cb.OnChanged != nil {
		e.cb.OnChanged(gw.gameID)
	}
}

// anyFileLocked walks the save location and reports whether any file in it
// is currently held with an incompatible sharing mode.
func anyFileLocked(savePath string) bool {
	info, err := os.Stat(savePath)
	if err != nil {
		return false
	}
	if !info.IsDir() {
		return isFileLocked(savePath)
	}
	locked := false
	filepath.Walk(savePath, func(path string, walkInfo os.FileInfo, walkErr error) error {
		if walkErr != nil || walkInfo.IsDir() {
			return nil
		}
		if isFileLocked(path) {
			locked = true
			return filepath.SkipAll
		}
		return nil
	})
	return locked
}

// addRecursive registers root and every subdirectory with the fs watcher.
//
// ctx is the watch's own context, and the walk abandons itself once that
// context is cancelled, so a shutdown does not keep queueing new work.
//
// This is defence in depth, not the fix for the shutdown hang — stating that
// plainly because the comment is otherwise easy to trust too far. It narrows
// the window in which an Add can be issued after Close (fsnotify's Windows
// backend serves Add from a goroutine Close tears down, and an Add that
// arrives afterwards waits for a reply that never comes). But cancel() can
// still land between this check and the Add it guards, and removing this guard
// could not be shown to change the outcome under realistic load. What actually
// guarantees shutdown is the bounded wait in stop().
func addRecursive(ctx context.Context, fsw *fsnotify.Watcher, root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if ctx.Err() != nil {
			return filepath.SkipAll
		}
		if err != nil {
			return nil // unreadable subdir: skip, don't fail the whole watch
		}
		if info.IsDir() {
			if strings.HasPrefix(filepath.Base(path), ".") && path != root {
				return filepath.SkipDir
			}
			// AddWith rather than Add, to size the per-directory buffer.
			return fsw.AddWith(path, fsnotify.WithBufferSize(watchBufferBytes))
		}
		return nil
	})
}

func (e *Engine) log(level, msg string) {
	if e.cb.Log != nil {
		e.cb.Log(level, msg)
	}
}

// filterForHash drops excluded paths before the content hash is taken, so the
// value recorded here means the same thing the sync engine means by it.
func filterForHash(m delta.Manifest, rules ignore.Rules) delta.Manifest {
	out := delta.Manifest{Files: make(map[string]delta.FileEntry, len(m.Files))}
	for p, entry := range m.Files {
		if !rules.Match(p) {
			out.Files[p] = entry
		}
	}
	for _, d := range m.Dirs {
		if !rules.Match(d) {
			out.Dirs = append(out.Dirs, d)
		}
	}
	if len(m.Extra) > 0 {
		out.Extra = make(map[string]delta.RootManifest, len(m.Extra))
		for name, root := range m.Extra {
			sub := delta.RootManifest{Files: make(map[string]delta.FileEntry, len(root.Files))}
			for p, entry := range root.Files {
				if !rules.Match(p) {
					sub.Files[p] = entry
				}
			}
			for _, d := range root.Dirs {
				if !rules.Match(d) {
					sub.Dirs = append(sub.Dirs, d)
				}
			}
			out.Extra[name] = sub
		}
	}
	return out
}
