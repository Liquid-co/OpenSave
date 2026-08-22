package e2e

import (
	"testing"
	"time"

	"github.com/opensave/opensave/testutil"
)

// A save file that exists only while the game is running must not manufacture
// a conflict when the game deletes it again.
//
// Reported from the field: a game keeps its save in one folder alongside a
// rotating .bak that exists only while it is open. Playing on the PC created
// the .bak, a sync copied it to the handheld, and quitting deleted it again —
// after which every sync reported "both sides modified since last sync" on a
// device that had been sitting idle for an hour. Syncing twice with the game
// closed came back clean, so nothing was stuck; the two devices simply
// disagreed about a scratch file the game throws away anyway.
//
// The mechanism is the merge-base failing to advance across the push. If the
// base still describes the state from before the file appeared, then after the
// deletion BOTH sides differ from it — one by having the file, one by having
// removed it — and a one-sided deletion reads as a two-way divergence.
//
// Nothing here is about .bak specifically. Plenty of games write temporary
// files while running, and any of them produces this shape.
func TestTransientFileDeletedOnQuitDoesNotConflict(t *testing.T) {
	if testing.Short() {
		t.Skip("relay-backed; skipped under -short")
	}

	a, b, gameID := trackBothOverRelay(t, "TransientFile", map[string]string{
		"progress.db": "start",
	})

	// The game runs: it rewrites its save and drops a rotating backup beside
	// it, the way the reported title does.
	testutil.SettleSync(t, gameID, a, b)
	a.WriteSave("progress.db", "played-for-an-hour")
	a.WriteSave("progress.bak", "rotating backup, exists only while running")

	if status, _ := syncTo(a, gameID, b.NodeID()); status == "conflict" {
		ca, _ := conflictOn(a, gameID)
		t.Fatalf("syncing while the game is running conflicted: %+v", ca.DiffFiles)
	}
	if !testutil.WaitFor(60*time.Second, func() bool { return b.ReadSave("progress.bak") != "" }) {
		// Not the point of the test, but if the peer never received it the
		// scenario below is not the reported one.
		t.Fatalf("the transient file never reached the peer, so this is not the reported setup")
	}

	// The game quits and removes its backup. Only this device changed; the
	// peer has not been touched since the sync above.
	testutil.SettleSync(t, gameID, a, b)
	a.RemoveSave("progress.bak")

	status, _ := syncTo(a, gameID, b.NodeID())
	if status == "conflict" {
		ca, _ := conflictOn(a, gameID)
		t.Fatalf("deleting a file the game created reported a conflict on a save the peer "+
			"never touched: %+v\n"+
			"  A base=%.12q pushed=%.12q\n"+
			"  B base=%.12q pushed=%.12q",
			ca.DiffFiles,
			a.Daemon.Store.GetAgreedHash(gameID, b.NodeID()),
			a.Daemon.Store.GetPushedHash(gameID, b.NodeID()),
			b.Daemon.Store.GetAgreedHash(gameID, a.NodeID()),
			b.Daemon.Store.GetPushedHash(gameID, a.NodeID()))
	}

	// And the deletion has to actually travel, rather than the peer pushing
	// the file back as something this device is "missing".
	if !testutil.WaitFor(60*time.Second, func() bool { return b.ReadSave("progress.bak") == "" }) {
		t.Error("the deletion never reached the peer — it still holds a file the game removed")
	}
	if got := b.ReadSave("progress.db"); got != "played-for-an-hour" {
		t.Errorf("the real save did not survive: peer holds %q", got)
	}
}
