package e2e

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/opensave/opensave/testutil"
)

// An unpair has to reach the other device even when the goodbye does not.
//
// Unpairing sends one signed goodbye and deletes the peer's record — and with
// it the key the goodbye was signed with. The goodbye used to go out once,
// fire and forget. If the other device was offline, or the relay socket was
// mid-reconnect, it was simply lost; and the fallback for that case — a bare
// unpair-notify sent when the other device next says hello — is refused by
// any device that has seen this one authenticate, which is every current
// pairing. So the other device went on listing this one as paired for good:
// trying to sync with it, and being turned away every time.

// The relay socket is down when A unpairs, so the goodbye has nowhere to go.
func TestRelayUnpair_AGoodbyeLostWhileOfflineStillArrives(t *testing.T) {
	relayURL := startRelay(t)
	a, b, _ := pairedAndSynced(t, relayURL, "lost-goodbye")

	a.Daemon.P2P.Wan.Disconnect()
	a.API(http.MethodDelete, "/api/peers/"+b.NodeID(), nil, nil)
	if _, err := a.Daemon.Store.GetPeer(b.NodeID()); err == nil {
		t.Fatal("setup: A did not unpair B")
	}
	waitForFailedGoodbye(t, a, "Spoof-B")
	if _, err := b.Daemon.Store.GetPeer(a.NodeID()); err != nil {
		t.Fatal("setup: B heard the goodbye although A was offline, so this proves nothing")
	}

	a.Daemon.P2P.Wan.Connect()
	if !testutil.WaitFor(45*time.Second, func() bool {
		_, err := b.Daemon.Store.GetPeer(a.NodeID())
		return err != nil
	}) {
		t.Fatal("A's goodbye was lost while A was offline, and B still lists A as paired after A came back")
	}
}

// The same on a LAN: B is unreachable at the moment A unpairs it.
func TestLANUnpair_AGoodbyeLostWhileOfflineStillArrives(t *testing.T) {
	a := testutil.NewTestDaemon(t, "Lost-A")
	b := testutil.NewTestDaemon(t, "Lost-B")
	a.PairWith(b)

	// Unlike the relay case there is no "authenticated before" state to set
	// up here: both daemons are on 127.0.0.1, and the LAN routes wave loopback
	// through unchecked, as they must for the app's own dashboard. It does not
	// matter to what this shows — on a LAN a lost goodbye had no second
	// chance at all, signed or not.

	// B goes away — its app is closed, or the laptop is asleep.
	b.Server.Stop()
	a.API(http.MethodDelete, "/api/peers/"+b.NodeID(), nil, nil)
	if _, err := a.Daemon.Store.GetPeer(b.NodeID()); err == nil {
		t.Fatal("setup: A did not unpair B")
	}
	// The goodbye goes out in the background; B must not be back before it
	// has failed, or it simply arrives.
	waitForFailedGoodbye(t, a, "Lost-B")

	// B comes back on the same address and goes about its business, which
	// includes checking on the devices it believes it is paired with.
	if _, err := b.Server.Start(b.Port); err != nil {
		t.Fatalf("setup: B could not come back on port %d: %v", b.Port, err)
	}
	if _, err := b.Daemon.Store.GetPeer(a.NodeID()); err != nil {
		t.Fatal("setup: B heard the goodbye although it was offline, so this proves nothing")
	}
	if !testutil.WaitFor(45*time.Second, func() bool {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		b.Daemon.P2P.PingPairedPeers(ctx)
		cancel()
		_, err := b.Daemon.Store.GetPeer(a.NodeID())
		return err != nil
	}) {
		t.Fatal("A's goodbye was lost while B was offline, and B still lists A as paired after it came back")
	}
}

// waitForFailedGoodbye waits for a to report that its goodbye to the device
// named could not be delivered.
func waitForFailedGoodbye(t *testing.T, a *testutil.TestDaemon, name string) {
	t.Helper()
	want := `could not reach "` + name + `"`
	if !testutil.WaitFor(30*time.Second, func() bool {
		for _, e := range a.Daemon.Log.History() {
			if strings.Contains(e.Message, want) {
				return true
			}
		}
		return false
	}) {
		t.Fatalf("setup: A never reported its goodbye to %s as undelivered", name)
	}
}
