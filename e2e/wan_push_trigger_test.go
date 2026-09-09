package e2e

import (
	"net/http"
	"testing"
	"time"

	"github.com/opensave/opensave/testutil"
)

// A push over the relay has to land when it is pushed, not when the receiving
// device next happens to look.
//
// The trigger that tells a peer "I have newer content, come and get it" is the
// one message in the sync protocol that wants no reply, and so it was built by
// hand instead of going through the code that seals and signs a request. Once
// request authentication shipped, the receiving side correctly began refusing
// unsigned requests from a peer that had authenticated before — and this
// trigger was unsigned. It was dropped on arrival, every time.
//
// Nothing was lost, which is why it was nearly invisible: the receiver's own
// periodic reconcile picked the change up on its next pass. What the user saw
// was a save that took up to a minute to appear on the other machine, with no
// error anywhere. Two tests failed on timing and none of them said why.
//
// So this asserts on latency, because latency was the only symptom.
func TestWanPushTriggerLandsPromptly(t *testing.T) {
	relayURL := startRelay(t)
	a := testutil.NewTestDaemon(t, "Trigger-A")
	b := testutil.NewTestDaemon(t, "Trigger-B")
	pairOverRelay(t, a, b, relayURL, "trigger-room")

	a.WriteSave("slot1.sav", "first")
	gameID := a.TrackGame("Trigger Game")
	b.API(http.MethodPost, "/api/games",
		map[string]string{"name": "Trigger Game", "savePath": b.SaveDir}, nil)
	syncTo(a, gameID, b.NodeID())
	if !testutil.WaitFor(60*time.Second, func() bool { return b.ReadSave("slot1.sav") == "first" }) {
		t.Fatal("setup: the first sync never landed")
	}

	// The refusal only starts once the peer has authenticated at least once,
	// which the sync above guarantees. Testing before that point would pass
	// against the broken build, because an unsigned request from a peer with
	// no latch is accepted.
	if !testutil.WaitFor(30*time.Second, func() bool {
		peer, err := b.Daemon.Store.GetPeer(a.NodeID())
		return err == nil && peer.AuthVerifiedMs > 0
	}) {
		t.Fatal("setup: the peer never authenticated, so the refusal being tested is not active")
	}

	// A one-sided change on A, pushed. B holds nothing newer, so this is a
	// pure push and its arrival depends entirely on the trigger.
	time.Sleep(syncSettleWindow)
	a.WriteSave("slot1.sav", "second")
	start := time.Now()
	syncTo(a, gameID, b.NodeID())

	// Comfortably inside the reconcile backstop, so passing cannot mean the
	// backstop covered for a dropped trigger. That interval is a minute; if it
	// is ever shortened, this bound has to come down with it or the test stops
	// distinguishing the two.
	const bound = 25 * time.Second
	if !testutil.WaitFor(bound, func() bool { return b.ReadSave("slot1.sav") == "second" }) {
		t.Fatalf("a pushed save had not reached the peer after %s — the push trigger is being "+
			"dropped, and the change will only appear when the peer's periodic reconcile "+
			"next runs", bound)
	}
	t.Logf("push landed in %s", time.Since(start).Round(time.Millisecond))
}
