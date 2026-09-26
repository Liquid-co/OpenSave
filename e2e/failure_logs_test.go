package e2e

import (
	"testing"

	"github.com/opensave/opensave/testutil"
)

// logOnFailure prints each device's daemon log if the test fails.
//
// A timing failure in CI arrives as one line — "no conflict raised" — with
// nothing to say which sync ran, what it decided or what failed underneath,
// and it rarely happens twice in a row. The daemon already logs every step of
// a sync; this puts that record next to the failure instead of discarding it
// with the test.
func logOnFailure(t *testing.T, devices ...*testutil.TestDaemon) {
	t.Helper()
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		for _, d := range devices {
			t.Logf("---- log of %s ----", d.Name())
			for _, e := range d.Daemon.Log.History() {
				t.Logf("%s %-7s %s", e.Timestamp, e.Level, e.Message)
			}
		}
	})
}
