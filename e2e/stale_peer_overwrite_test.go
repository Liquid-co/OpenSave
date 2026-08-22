package e2e

import (
	"testing"
	"time"

	"github.com/opensave/opensave/testutil"
)

// A device that has not been played on must never overwrite one that has.
//
// Reported: a main machine's save was replaced by the save from a second
// machine, with no conflict raised, and the original could not be found
// afterwards. That is not a conflict being decided wrongly — it is a pull
// running in the direction nothing should have pulled.
//
// The base-repair added in 2.3.1 advances the merge-base far more often than
// before, and the merge-base is what decides who is behind whom. A base that
// advanced wrongly would make a device look unchanged when it is not, and an
// unchanged device is one the engine will happily overwrite. That is the
// mechanism to rule out.
func TestIdlePeerDoesNotOverwriteAPlayedSave(t *testing.T) {
	if testing.Short() {
		t.Skip("relay-backed; skipped under -short")
	}

	a, b, gameID := trackBothOverRelay(t, "StalePeer", map[string]string{
		"slot1.sav": "shared start",
	})

	// Establish the repair's precondition: A pushes, B ends up holding
	// exactly what it was handed.
	testutil.SettleSync(t, gameID, a, b)
	a.WriteSave("slot1.sav", "run 1")
	if status, _ := syncTo(a, gameID, b.NodeID()); status == "conflict" {
		t.Fatalf("a one-sided edit conflicted before the scenario began")
	}
	if !testutil.WaitFor(60*time.Second, func() bool { return b.ReadSave("slot1.sav") == "run 1" }) {
		t.Fatal("the push never landed, so the precondition was never set up")
	}

	// Only A is played on. B is not touched again — it is the machine left
	// sitting idle, holding what it was given.
	testutil.SettleSync(t, gameID, a, b)
	a.WriteSave("slot1.sav", "run 2, hours of progress")

	// Sync from BOTH directions. The reported loss happened without anyone
	// choosing anything, so whichever side initiates, the older copy must
	// not win.
	if status, _ := syncTo(a, gameID, b.NodeID()); status == "conflict" {
		t.Fatalf("A syncing its own newer save reported a conflict against an idle peer")
	}
	if got := a.ReadSave("slot1.sav"); got != "run 2, hours of progress" {
		t.Fatalf("A's newer save was replaced by syncing to an idle peer: A now holds %q", got)
	}

	testutil.SettleSync(t, gameID, a, b)
	if status, _ := syncTo(b, gameID, a.NodeID()); status == "conflict" {
		t.Fatalf("the idle peer initiating a sync reported a conflict it has no business raising")
	}
	if got := a.ReadSave("slot1.sav"); got != "run 2, hours of progress" {
		t.Fatalf("the idle peer overwrote the played save when IT initiated: A now holds %q\n"+
			"  A base=%.12q pushed=%.12q\n  B base=%.12q pushed=%.12q",
			got,
			a.Daemon.Store.GetAgreedHash(gameID, b.NodeID()),
			a.Daemon.Store.GetPushedHash(gameID, b.NodeID()),
			b.Daemon.Store.GetAgreedHash(gameID, a.NodeID()),
			b.Daemon.Store.GetPushedHash(gameID, a.NodeID()))
	}

	// And the newer save should have travelled the other way instead.
	if !testutil.WaitFor(60*time.Second, func() bool {
		return b.ReadSave("slot1.sav") == "run 2, hours of progress"
	}) {
		t.Errorf("the played save never reached the idle peer: B holds %q", b.ReadSave("slot1.sav"))
	}
}
