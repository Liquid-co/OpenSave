package e2e

import (
	"os"
	"testing"
	"time"

	"github.com/opensave/opensave/testutil"
)

// TestPresence_GoneDeviceShowsOffline measures how long a device that went
// away keeps being shown online. Opt-in measurement:
// OPENSAVE_PRESENCE_TIMING=1 go test ./e2e -run TestPresence_ -v
func TestPresence_GoneDeviceShowsOffline(t *testing.T) {
	if os.Getenv("OPENSAVE_PRESENCE_TIMING") == "" {
		t.Skip("set OPENSAVE_PRESENCE_TIMING to run")
	}
	a := testutil.NewTestDaemon(t, "Presence-A")
	b := testutil.NewTestDaemon(t, "Presence-B")
	a.PairWith(b)
	bID := b.NodeID()

	status := func() string {
		p, err := a.Daemon.Store.GetPeer(bID)
		if err != nil {
			return "?"
		}
		return p.Status
	}
	if !testutil.WaitFor(60*time.Second, func() bool { return status() == "online" }) {
		t.Fatalf("B never showed online on A: %q", status())
	}

	gone := time.Now()
	b.Server.Stop()
	b.Daemon.Stop()

	last := status()
	for time.Since(gone) < 10*time.Minute {
		if s := status(); s != last {
			t.Logf("%6.1fs after B stopped: %s -> %s", time.Since(gone).Seconds(), last, s)
			last = s
		}
		if last == "offline" {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Logf("B shown online for %.1fs after it stopped", time.Since(gone).Seconds())
	if last != "offline" {
		t.Fatalf("B still %q 10 minutes after it stopped", last)
	}
}
