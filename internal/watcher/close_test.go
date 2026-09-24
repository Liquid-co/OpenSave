package watcher

import (
	"context"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

// If fsnotify's close is lost anyway — see closeWatcher for how, and
// TestStopReturnsAfterAnUnreadOverflow for the case it now survives — stopping
// the watch still returns, at the bound, and says it gave up. stop() used to
// call Close before its bounded wait, so a Close that never returned hung it.
func TestStopReturnsWhenCloseNeverDoes(t *testing.T) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	t.Cleanup(func() {
		close(release)
		fsw.Close()
	})

	_, cancel := context.WithCancel(context.Background())
	var warned []string
	gw := &gameWatch{
		gameID:  "stuck",
		fsw:     fsw,
		cancel:  cancel,
		done:    make(chan struct{}),
		log:     func(_, msg string) { warned = append(warned, msg) },
		closeFS: func() error { <-release; return nil },
	}
	close(gw.done)

	returned := make(chan time.Duration, 1)
	go func() {
		start := time.Now()
		gw.stop()
		returned <- time.Since(start)
	}()
	select {
	case took := <-returned:
		if len(warned) == 0 {
			t.Errorf("stop() returned in %s without reporting the watcher it abandoned", took)
		}
	case <-time.After(watchStopTimeout + 10*time.Second):
		t.Fatal("stop() did not return: a close that never finishes still hangs it")
	}
}
