package e2e

import (
	"net/http"
	"testing"
	"time"

	"github.com/opensave/opensave/internal/delta"
	"github.com/opensave/opensave/testutil"
)

// A merge-base that falls behind both devices does not merely delay
// convergence — it manufactures conflicts. The check asks whether BOTH sides
// moved off the base; once the base is behind both of them the answer is yes
// forever, so every later one-sided edit reads as a two-way divergence and
// prompts on a save the peer never touched.
//
// The base falls behind on the pushing side by design: the peer is the one
// that ends up holding the new state, so convergence is recorded when it
// reports back over the network. That report can be lost — a dropped relay
// frame, a peer that quits mid-pull — and nothing afterwards repaired it.
//
// This test injects that state directly rather than trying to lose a message,
// because the harness delivers reliably: the earlier version of this test
// synced normally and passed with the fix reverted, proving nothing at all.
//
// Exactly that state, and nothing else in motion. The injection used to set
// the base alone, which also clears the record of what was pushed — the very
// record a lost report leaves behind, and the one the repair works from. The
// test then passed only when some background sync or late report happened to
// re-record the base before the edit, and failed when none did: 3 runs in
// 110 under load.
func TestStaleBase_OneSidedEditDoesNotConflict(t *testing.T) {
	a, b, gameID := pairAndTrack(t, "StaleBase", map[string]string{"slot1.sav": "shared"})

	// Nothing may run between the injection and the edit: a sync or a report
	// while the two still agree repairs the base by another route and proves
	// nothing about this one. So both stop syncing on their own, and the
	// test waits until they have finished and recorded agreeing.
	for _, d := range []*testutil.TestDaemon{a, b} {
		d.API(http.MethodPatch, "/api/games/"+gameID, map[string]any{"autoSync": false}, nil)
	}
	shared, err := delta.BuildManifest(a.SaveDir)
	if err != nil {
		t.Fatal(err)
	}
	if !testutil.WaitFor(45*time.Second, func() bool {
		return a.Daemon.Store.GetAgreedHash(gameID, b.NodeID()) == shared.ManifestHash()
	}) {
		t.Fatal("setup: A never recorded agreeing with B")
	}
	testutil.SettleSync(t, gameID, a, b)

	// Freeze the base at a state neither one holds, with the push that the
	// lost report was about still on record: B holds exactly what A sent.
	if err := a.Daemon.Store.SetAgreedHash(gameID, b.NodeID(), "stale-base-neither-side-holds"); err != nil {
		t.Fatalf("SetAgreedHash error = %v", err)
	}
	if err := a.Daemon.Store.SetPushedHash(gameID, b.NodeID(), shared.ManifestHash()); err != nil {
		t.Fatalf("SetPushedHash error = %v", err)
	}

	// One device edits. The other has not been touched.
	time.Sleep(syncSettleWindow)
	a.WriteSave("slot1.sav", "edited-by-A-only")

	status, _ := syncTo(a, gameID, b.NodeID())
	if status == "conflict" {
		t.Fatalf("a one-sided edit was reported as a conflict (%q) — the merge-base is behind "+
			"both devices, so both read as having moved off it even though only one changed", status)
	}

	if !testutil.WaitFor(45*time.Second, func() bool {
		return b.ReadSave("slot1.sav") == "edited-by-A-only"
	}) {
		t.Errorf("the edit never propagated: b slot1=%q", b.ReadSave("slot1.sav"))
	}
}

// Suppressing the prompt is not enough: if the base is left stale the same
// trap re-arms on the very next edit. It has to actually advance.
func TestStaleBase_IsRepairedNotJustTolerated(t *testing.T) {
	a, b, gameID := pairAndTrack(t, "StaleBaseRepair", map[string]string{"slot1.sav": "shared"})

	if err := a.Daemon.Store.SetAgreedHash(gameID, b.NodeID(), "stale-base-neither-side-holds"); err != nil {
		t.Fatalf("SetAgreedHash error = %v", err)
	}

	// A sync while both sides agree is the moment convergence is provable
	// without being told. It must be banked.
	syncTo(a, gameID, b.NodeID())

	if got := a.Daemon.Store.GetAgreedHash(gameID, b.NodeID()); got == "stale-base-neither-side-holds" {
		t.Fatal("the stale merge-base survived a sync where both devices agreed — " +
			"the next one-sided edit will conflict all over again")
	}

	// Prove it: two edits in a row, each on one side only, neither a conflict.
	time.Sleep(syncSettleWindow)
	a.WriteSave("slot1.sav", "first-edit")
	if status, _ := syncTo(a, gameID, b.NodeID()); status == "conflict" {
		t.Fatalf("first one-sided edit conflicted: %q", status)
	}
	if !testutil.WaitFor(45*time.Second, func() bool {
		return b.ReadSave("slot1.sav") == "first-edit"
	}) {
		t.Fatal("first edit never propagated")
	}

	time.Sleep(syncSettleWindow)
	a.WriteSave("slot1.sav", "second-edit")
	if status, _ := syncTo(a, gameID, b.NodeID()); status == "conflict" {
		t.Fatalf("second one-sided edit conflicted: %q — the base is still not advancing", status)
	}
}

// The guard: repairing a stale base must not paper over a real divergence.
// When the two devices genuinely hold different content, that is still a
// conflict no matter what the base says.
func TestStaleBase_RealDivergenceStillConflicts(t *testing.T) {
	a, b, gameID := pairAndTrack(t, "StaleBaseReal", map[string]string{"slot1.sav": "shared"})

	if err := a.Daemon.Store.SetAgreedHash(gameID, b.NodeID(), "stale-base-neither-side-holds"); err != nil {
		t.Fatalf("SetAgreedHash error = %v", err)
	}

	time.Sleep(syncSettleWindow)
	a.WriteSave("slot1.sav", "A-diverged")
	b.WriteSave("slot1.sav", "B-diverged")

	if status, _ := syncTo(a, gameID, b.NodeID()); status != "conflict" {
		t.Fatalf("both devices edited the same file but sync reported %q — the stale-base repair "+
			"is swallowing a real conflict and one side's edit is about to be lost", status)
	}
}
