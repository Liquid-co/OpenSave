package daemon

import (
	"strings"
	"testing"
	"time"
)

// A pause is logged in words. The Activity feed shows this line, and it read
// "syncing paused for 1h0m0s".
func TestPauseIsLoggedInWords(t *testing.T) {
	d := newTestDaemon(t)
	for dur, want := range map[time.Duration]string{
		15 * time.Minute: "syncing paused for 15 minutes —",
		time.Hour:        "syncing paused for 1 hour —",
		90 * time.Minute: "syncing paused for 1 hour 30 minutes —",
		3 * time.Hour:    "syncing paused for 3 hours —",
	} {
		d.PauseSync(dur)
		history := d.Log.History()
		if got := history[len(history)-1].Message; !strings.HasPrefix(got, want) {
			t.Errorf("pausing for %s logged %q, want it to start %q", dur, got, want)
		}
		d.ResumeSync()
	}
}
