package watcher

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

// overflowedWatcher returns a real fsnotify watcher whose reader is waiting to
// hand over an overflow error nobody has taken: a folder written faster than
// it was read, then its events taken and its errors not — the state a burst
// leaves behind once the run loop has been cancelled.
func overflowedWatcher(t *testing.T) *fsnotify.Watcher {
	t.Helper()
	// Not t.TempDir: a watcher this test abandons still holds the folder.
	dir, err := os.MkdirTemp("", "overflow")
	if err != nil {
		t.Fatal(err)
	}
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	if err := fsw.AddWith(dir, fsnotify.WithBufferSize(watchBufferBytes)); err != nil {
		t.Fatal(err)
	}
	// Nothing reads while these are written, so the reader stalls on the first
	// event and everything after it overflows the folder's buffer.
	for i := 0; i < 600; i++ {
		name := filepath.Join(dir, fmt.Sprintf("slot-with-a-longer-name-%04d.sav", i))
		if err := os.WriteFile(name, []byte("x"), 0o666); err != nil {
			t.Fatal(err)
		}
	}
	for quiet := false; !quiet; {
		select {
		case <-fsw.Events:
		case <-time.After(300 * time.Millisecond):
			quiet = true
		}
	}
	return fsw
}

// Stopping a watch returns promptly after a burst. It could wait forever:
// fsnotify's Close was lost to the overflow error its reader was holding, and
// stop() called Close before its bounded wait, so the bound never applied.
// Found as a 45-minute timeout in TestBurst_ManyFilesAtOnceEndsWithAnAccurateBaseline
// on a full suite run; in the app it is a quit that never finishes.
func TestStopReturnsAfterAnUnreadOverflow(t *testing.T) {
	// The premise, checked: a plain Close in this state does hang. If fsnotify
	// stops losing it, this test is no longer testing anything.
	plain := overflowedWatcher(t)
	closed := make(chan struct{})
	go func() {
		plain.Close()
		close(closed)
	}()
	select {
	case <-closed:
		t.Skip("fsnotify's Close no longer hangs after an unread overflow; the premise is gone")
	case <-time.After(2 * time.Second):
	}

	_, cancel := context.WithCancel(context.Background())
	var warned []string
	gw := &gameWatch{
		gameID: "burst",
		fsw:    overflowedWatcher(t),
		cancel: cancel,
		done:   make(chan struct{}),
		log:    func(_, msg string) { warned = append(warned, msg) },
	}
	// The run loop has already gone, as it does as soon as it is cancelled.
	close(gw.done)

	returned := make(chan time.Duration, 1)
	go func() {
		start := time.Now()
		gw.stop()
		returned <- time.Since(start)
	}()
	select {
	case took := <-returned:
		// Prompt, not merely bounded: giving up at the bound would leave the
		// watcher's goroutine and folder handle behind on every such stop.
		if took > 2*time.Second || len(warned) > 0 {
			t.Errorf("stop() took %s and warned %q; the close should not have been lost", took, warned)
		}
	case <-time.After(watchStopTimeout + 10*time.Second):
		t.Fatal("stop() did not return: closing the watcher hung")
	}
}
