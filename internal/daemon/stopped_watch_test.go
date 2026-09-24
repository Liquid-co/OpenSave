package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opensave/opensave/internal/store"
)

// `opensave add` tracks a game on a daemon of its own that is gone by the time
// the watch would start — and tells the running daemon to take the game on.
// Nothing failed, so nothing should say "could not watch".
func TestTrackingOnAStoppingDaemonLogsNoWatchFailure(t *testing.T) {
	d := newTestDaemon(t)
	save := t.TempDir()
	if err := os.WriteFile(filepath.Join(save, "slot.sav"), []byte("x"), 0o666); err != nil {
		t.Fatal(err)
	}
	d.Watcher.Stop()
	if _, err := d.TrackGame(store.Game{Name: "Quick Add", SavePath: save}); err != nil {
		t.Fatal(err)
	}
	d.WaitForTracking()
	for _, e := range d.Log.History() {
		if strings.Contains(e.Message, "could not watch") {
			t.Errorf("logged %q for a watch that was never going to start", e.Message)
		}
	}
}
