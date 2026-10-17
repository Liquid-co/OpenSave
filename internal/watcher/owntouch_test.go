package watcher

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/opensave/opensave/internal/owntouch"
)

// Files OpenSave writes or deletes itself — a pull, a peer's deletions — are
// not a new save. Only the recorded hash moves; no auto-snapshot, and no
// OnChanged echoing the sync back out.
func TestOwnChanges_AreNotSnapshotted(t *testing.T) {
	saveDir := t.TempDir()
	col := newCollector()
	eng := New(col.callbacks())
	defer eng.Stop()
	if err := eng.Watch("game1", saveDir); err != nil {
		t.Fatal(err)
	}

	// A peer's deletions arriving one at a time, as they do, plus pulled files.
	for i := 0; i < 5; i++ {
		p := filepath.Join(saveDir, fmt.Sprintf("map_%d.bin", i))
		owntouch.Mark(p)
		if err := os.WriteFile(p, []byte(fmt.Sprint(i)), 0o666); err != nil {
			t.Fatal(err)
		}
		time.Sleep(300 * time.Millisecond)
	}
	// Past the debounce, with room to spare.
	if !waitFor(t, 8*time.Second, func() bool {
		col.mu.Lock()
		defer col.mu.Unlock()
		return col.manifestHashes["game1"] != ""
	}) {
		t.Fatal("the recorded hash was not moved to the new content")
	}
	time.Sleep(3 * time.Second)
	if n := col.snapshotCount(); n != 0 {
		t.Errorf("%d auto-snapshot(s) for changes OpenSave made itself, want 0", n)
	}
	if n := col.changedCount(); n != 0 {
		t.Errorf("OnChanged fired %d time(s) for changes OpenSave made itself", n)
	}

	// The game then saves: that is snapshotted, as always.
	if err := os.WriteFile(filepath.Join(saveDir, "slot1.sav"), []byte("played"), 0o666); err != nil {
		t.Fatal(err)
	}
	if !waitFor(t, 10*time.Second, func() bool { return col.snapshotCount() >= 1 }) {
		t.Fatal("a change the game made was not snapshotted")
	}
}

// A pull that creates folders and writes into them before the watcher has put
// them under watch is still the pull: the rescan that follows registering
// them is not someone else's change.
func TestOwnNewFolders_AreNotSnapshotted(t *testing.T) {
	saveDir := t.TempDir()
	col := newCollector()
	eng := New(col.callbacks())
	defer eng.Stop()
	if err := eng.Watch("game1", saveDir); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		dir := filepath.Join(saveDir, fmt.Sprintf("slot%02d", i))
		owntouch.Mark(dir)
		if err := os.MkdirAll(dir, 0o777); err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(dir, "data.sav")
		owntouch.Mark(p)
		if err := os.WriteFile(p, []byte(fmt.Sprint(i)), 0o666); err != nil {
			t.Fatal(err)
		}
	}
	if !waitFor(t, 10*time.Second, func() bool {
		col.mu.Lock()
		defer col.mu.Unlock()
		return col.manifestHashes["game1"] != ""
	}) {
		t.Fatal("the recorded hash was not moved to the new content")
	}
	time.Sleep(4 * time.Second)
	if n := col.snapshotCount(); n != 0 {
		t.Errorf("%d auto-snapshot(s) for folders and files a pull created", n)
	}

	// A folder the game creates is a change, as before.
	if err := os.MkdirAll(filepath.Join(saveDir, "newslot"), 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(saveDir, "newslot", "data.sav"), []byte("played"), 0o666); err != nil {
		t.Fatal(err)
	}
	if !waitFor(t, 10*time.Second, func() bool { return col.snapshotCount() >= 1 }) {
		t.Fatal("a folder the game created was not snapshotted")
	}
}

// While a sync is writing the game, the watcher looks at what changed only
// once it has finished — never at a save half-way between two states — and a
// change of the game's is still snapshotted then.
func TestWhileASyncWrites_TheWatcherWaits(t *testing.T) {
	saveDir := t.TempDir()
	col := newCollector()
	var writing atomic.Bool
	cb := col.callbacks()
	cb.SyncWriting = func(string) bool { return writing.Load() }
	cb.SyncWritingNow = cb.SyncWriting
	eng := New(cb)
	defer eng.Stop()
	if err := eng.Watch("game1", saveDir); err != nil {
		t.Fatal(err)
	}
	writing.Store(true)
	if err := os.WriteFile(filepath.Join(saveDir, "slot1.sav"), []byte("played"), 0o666); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Second)
	if n := col.snapshotCount(); n != 0 {
		t.Fatalf("%d snapshot(s) while a sync was writing the game", n)
	}
	writing.Store(false)
	if !waitFor(t, 10*time.Second, func() bool { return col.snapshotCount() >= 1 }) {
		t.Fatal("the game's change was not snapshotted once the sync had finished")
	}
}

// A burst that mixes OpenSave's own changes with one from the game is a real
// change and is snapshotted.
func TestMixedChanges_AreSnapshotted(t *testing.T) {
	saveDir := t.TempDir()
	col := newCollector()
	eng := New(col.callbacks())
	defer eng.Stop()
	if err := eng.Watch("game1", saveDir); err != nil {
		t.Fatal(err)
	}
	own := filepath.Join(saveDir, "pulled.bin")
	owntouch.Mark(own)
	if err := os.WriteFile(own, []byte("from a peer"), 0o666); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(saveDir, "slot1.sav"), []byte("played"), 0o666); err != nil {
		t.Fatal(err)
	}
	if !waitFor(t, 10*time.Second, func() bool { return col.snapshotCount() >= 1 }) {
		t.Fatal("a burst including the game's own change was not snapshotted")
	}
}
