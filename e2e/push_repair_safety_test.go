package e2e

import (
	"testing"
	"time"

	"github.com/opensave/opensave/testutil"
)

// Advancing the merge-base from a push record must never swallow a real
// conflict.
//
// 2.3.1 corrected what that record holds: it used to name this device's save
// as it stood at the end of a sync, which the peer had never been offered, so
// the repair that consumes it almost never fired. Now it names what the peer
// was actually handed, and the repair fires often. Anything unsound about it
// therefore went from nearly unreachable to routine.
//
// The soundness argument is that the peer holding exactly what we pushed
// proves both sides held that state, which is what a merge-base is. This test
// is the argument checked rather than asserted, on the case that would hurt:
// establish the repair's precondition, then have both devices genuinely
// diverge. That must still be a conflict, because the alternative is one
// person's save quietly replacing another's.
func TestPushRepairDoesNotSwallowARealConflict(t *testing.T) {
	if testing.Short() {
		t.Skip("relay-backed; skipped under -short")
	}

	a, b, gameID := trackBothOverRelay(t, "PushRepair", map[string]string{
		"slot1.sav": "shared start",
	})

	// 1. A plays and pushes. B ends up holding exactly what A handed it,
	//    which is the precondition the repair looks for.
	testutil.SettleSync(t, gameID, a, b)
	a.WriteSave("slot1.sav", "A's progress")
	if status, _ := syncTo(a, gameID, b.NodeID()); status == "conflict" {
		t.Fatalf("a one-sided edit conflicted before the scenario even started")
	}
	if !testutil.WaitFor(60*time.Second, func() bool { return b.ReadSave("slot1.sav") == "A's progress" }) {
		t.Fatal("the push never reached B, so the repair's precondition was never set up")
	}

	// 2. Now both devices edit the same file, independently. Neither has
	//    seen the other's change. This is a genuine divergence and the only
	//    correct answer is to ask.
	testutil.SettleSync(t, gameID, a, b)
	a.WriteSave("slot1.sav", "A played on after the sync")
	b.WriteSave("slot1.sav", "B played too, on the other machine")

	status, _ := syncTo(a, gameID, b.NodeID())
	if status != "conflict" {
		t.Fatalf("two devices edited the same save independently and the sync reported %q "+
			"instead of a conflict — one of those saves is being overwritten without asking.\n"+
			"  A base=%.12q pushed=%.12q\n"+
			"  B base=%.12q pushed=%.12q\n"+
			"  A holds %q\n  B holds %q",
			status,
			a.Daemon.Store.GetAgreedHash(gameID, b.NodeID()),
			a.Daemon.Store.GetPushedHash(gameID, b.NodeID()),
			b.Daemon.Store.GetAgreedHash(gameID, a.NodeID()),
			b.Daemon.Store.GetPushedHash(gameID, a.NodeID()),
			a.ReadSave("slot1.sav"), b.ReadSave("slot1.sav"))
	}

	// 3. And neither side may have been altered while the question is open.
	//    A conflict that has already overwritten something is not a conflict.
	if got := a.ReadSave("slot1.sav"); got != "A played on after the sync" {
		t.Errorf("A's save changed while a conflict was pending: %q", got)
	}
	if got := b.ReadSave("slot1.sav"); got != "B played too, on the other machine" {
		t.Errorf("B's save changed while a conflict was pending: %q", got)
	}
}
