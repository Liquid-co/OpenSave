// Package owntouch remembers which paths in save folders OpenSave itself has
// just written or removed, so the watcher can tell those changes apart from
// ones a game or a person made.
//
// Without it every change OpenSave applied — a file pulled from a peer, a
// deletion a peer asked for, the mtimes "keep mine" touches — looked to the
// watcher exactly like the game saving. A peer deleting files one request at
// a time over several minutes produced a full auto-snapshot every couple of
// seconds, each of a save that was neither the old one nor the new one, and
// those filled the retention budget and pushed out the snapshots that mattered.
package owntouch

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// window is how long a mark counts. Long enough to cover the watcher's
// debounce and a slow burst of events behind it; short enough that the same
// path changed again by a game a little later is not mistaken for ours.
const window = 2 * time.Minute

var (
	mu     sync.Mutex
	marked = map[string]mark{}
)

// mark is when OpenSave touched a path, whether it removed it, and - once
// OpenSave is done with a file (Settled) - the time and size it left it with.
type mark struct {
	at      time.Time
	removed bool
	settled bool
	mod     time.Time
	size    int64
}

func key(path string) string {
	p := filepath.Clean(path)
	if runtime.GOOS == "windows" {
		p = strings.ToLower(p)
	}
	return p
}

// Mark records that OpenSave itself just created, wrote, removed or re-dated
// path. Its parent folder is marked too: creating or removing an entry is an
// event on the folder as well.
func Mark(path string) { record(path, false) }

// MarkRemoved records that OpenSave itself is about to remove path. Only a
// path marked this way counts as OpenSave's when it is found gone: a file
// OpenSave has just written and a game or a person then deleted is their
// change, and has to sync and be snapshotted as one.
func MarkRemoved(path string) { record(path, true) }

// Settled records the time and size OpenSave has left a file it marked with,
// once it is done with it (written, renamed into place, re-dated). From then on
// the file is OpenSave's only while it still has exactly those: a game saving
// over it a moment later - inside any slack a time comparison would need - is
// the game's change.
func Settled(path string) {
	fi, err := os.Lstat(path)
	if err != nil || fi.IsDir() {
		return
	}
	k := key(path)
	mu.Lock()
	defer mu.Unlock()
	if m, ok := marked[k]; ok {
		m.settled, m.mod, m.size = true, fi.ModTime(), fi.Size()
		marked[k] = m
	}
}

func record(path string, removed bool) {
	if path == "" {
		return
	}
	now := time.Now()
	mu.Lock()
	defer mu.Unlock()
	marked[key(path)] = mark{at: now, removed: removed}
	marked[key(filepath.Dir(path))] = mark{at: now}
	if len(marked) > 50000 {
		for k, m := range marked {
			if now.Sub(m.at) > window {
				delete(marked, k)
			}
		}
	}
}

// slack is how much later than its mark a file OpenSave wrote may be dated:
// re-dating to now happens just after marking, and some filesystems keep
// times to two seconds.
const slack = 2 * time.Second

// Recent reports whether the change just seen at path is one OpenSave made:
// the path was marked within the window, and nothing has written it since.
//
// The second half matters as much as the first. A game often saves straight
// after a sync — the same file OpenSave has just put there — and a mark alone
// would take that save for the sync's for two minutes: no snapshot of it, and
// no new version to hand to anyone. Whatever OpenSave leaves behind is dated
// no later than its mark (a pulled file keeps the time it was written to its
// temporary name, or is given the peer's; re-dating sets now), so a file
// dated later was written by someone else. Once OpenSave is done with a file
// (Settled) the test is exact: the time and size it left, or not ours - the
// slack would take a game's save within two seconds of a pull for the pull's.
// A path that is gone is OpenSave's
// only if OpenSave removed it (MarkRemoved): one it wrote and someone then
// deleted is theirs. A folder carries no save of its own, so its mark is
// enough.
func Recent(path string) bool {
	mu.Lock()
	m, ok := marked[key(path)]
	mu.Unlock()
	if !ok || time.Since(m.at) > window {
		return false
	}
	fi, err := os.Lstat(path)
	if err != nil {
		return m.removed
	}
	if fi.IsDir() {
		return true
	}
	if m.settled {
		return fi.ModTime().Equal(m.mod) && fi.Size() == m.size
	}
	return !fi.ModTime().After(m.at.Add(slack))
}
