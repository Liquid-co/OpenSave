package watcher

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// Putting a folder under watch must never wait on the loop that drains the
// watcher's events.
//
// fsnotify's Windows backend serves Add from the same goroutine that delivers
// events, and only between deliveries. An Add issued while nobody is taking
// events therefore waits for a reader that is itself waiting to hand over an
// event — forever. Both places this package registers folders did exactly
// that: the event loop, on a new subfolder's Create, and Watch, which walked
// the whole tree before the loop that drains events had started.

// A save folder that is busy while its watch starts — the game running when
// OpenSave launches — must not hang Watch. The walk registers the root first,
// so its events are already being produced while the rest of the tree is
// still being added.
func TestWatch_ReturnsWhileTheFolderIsBusyDuringRegistration(t *testing.T) {
	dir := t.TempDir()
	// Enough subfolders that registering them takes a while, which is the
	// window the busy writer below needs.
	for i := 0; i < 1500; i++ {
		if err := os.MkdirAll(filepath.Join(dir, fmt.Sprintf("area%04d", i)), 0o777); err != nil {
			t.Fatal(err)
		}
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			_ = os.WriteFile(filepath.Join(dir, fmt.Sprintf("autosave%02d.sav", i%20)), []byte(fmt.Sprint(i)), 0o666)
		}
	}()
	defer func() { close(stop); wg.Wait() }()

	col := newCollector()
	eng := New(col.callbacks())
	t.Cleanup(eng.Stop)

	done := make(chan error, 1)
	go func() { done <- eng.Watch("busy", dir) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Watch never returned: registering the tree waited on events nobody was draining")
	}
}

// After a folder is registered off the event loop, a change made inside it
// must still be noticed — the registration happening later than the Create
// event must not leave a window in which the folder's first change is missed.
func TestNewSubfolder_FirstChangeInsideIsNoticed(t *testing.T) {
	dir := t.TempDir()
	col := newCollector()
	eng := New(col.callbacks())
	t.Cleanup(eng.Stop)

	if err := os.WriteFile(filepath.Join(dir, "slot0.sav"), []byte("start"), 0o666); err != nil {
		t.Fatal(err)
	}
	manifest, _, err := buildForTest(dir)
	if err != nil {
		t.Fatal(err)
	}
	col.mu.Lock()
	col.manifestHashes["newdir"] = manifest
	col.mu.Unlock()
	if err := eng.Watch("newdir", dir); err != nil {
		t.Fatal(err)
	}

	sub := filepath.Join(dir, "profile1")
	if err := os.MkdirAll(sub, 0o777); err != nil {
		t.Fatal(err)
	}
	if !waitUntil(15*time.Second, func() bool { return finalStateRecorded(t, col, "newdir", dir) }) {
		t.Fatal("creating an empty folder was never recorded")
	}

	// The folder exists and has been rescanned. Anything written into it
	// from here on can only be seen through its own watch.
	if err := os.WriteFile(filepath.Join(sub, "data.sav"), []byte("first save in the new profile"), 0o666); err != nil {
		t.Fatal(err)
	}
	if !waitUntil(15*time.Second, func() bool { return finalStateRecorded(t, col, "newdir", dir) }) {
		t.Error("a file written into a newly created folder was never noticed — the folder is not being watched")
	}
}
