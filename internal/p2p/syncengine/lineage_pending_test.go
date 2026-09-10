package syncengine

import (
	"sort"
	"testing"

	"github.com/opensave/opensave/internal/delta"
)

// A lineage rebuild must not erase a deletion that is still in flight.
//
// The lineage is the record of what both devices have held, and "in the
// lineage but missing on one side" is the only evidence the next sync has
// that the missing side deleted it. It is rebuilt from a fresh walk by three
// asynchronous paths after every transfer, and each one computed the plain
// intersection — so a file deleted right after it arrived was reliably erased
// from the record before any sync could act on it, and the next sync pulled
// it back. Five runs in six on Linux.
//
// These pin the rule at the function that writes the record.

func mf(paths ...string) delta.Manifest {
	m := delta.Manifest{Files: map[string]delta.FileEntry{}}
	for _, p := range paths {
		m.Files[p] = delta.FileEntry{Hash: "h-" + p}
	}
	return m
}

func lineageOf(t *testing.T, env *engineEnv) []string {
	t.Helper()
	files, _, err := env.store.GetSyncState("game1", env.peer.ID)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	return files
}

func TestPersistLineage_KeepsAFileTheOtherSideStillHolds(t *testing.T) {
	env := setupEngine(t)
	e := env.engine

	// Both sides hold two files; the lineage records both.
	e.persistLineage("game1", env.peer.ID, mf("keep.sav", "kill.sav"), mf("keep.sav", "kill.sav"))
	if got := lineageOf(t, env); len(got) != 2 {
		t.Fatalf("setup: lineage = %v", got)
	}

	// This side deleted kill.sav; the peer still has it. A rebuild lands
	// before the deletion has propagated — exactly the timing that erased it.
	e.persistLineage("game1", env.peer.ID, mf("keep.sav"), mf("keep.sav", "kill.sav"))

	got := lineageOf(t, env)
	if !contains(got, "kill.sav") {
		t.Errorf("a file this side deleted and the peer still holds was dropped from the lineage "+
			"(%v); the next sync would read the peer's copy as new and pull it back", got)
	}
}

func TestPersistLineage_KeepsAFileThisSideStillHolds(t *testing.T) {
	env := setupEngine(t)
	e := env.engine

	e.persistLineage("game1", env.peer.ID, mf("keep.sav", "kill.sav"), mf("keep.sav", "kill.sav"))

	// The mirror: the peer deleted it and this side still has it.
	e.persistLineage("game1", env.peer.ID, mf("keep.sav", "kill.sav"), mf("keep.sav"))

	if got := lineageOf(t, env); !contains(got, "kill.sav") {
		t.Errorf("a file the peer deleted and this side still holds was dropped (%v); the "+
			"next sync would push it back to the peer instead of deleting it here", got)
	}
}

func TestPersistLineage_DropsAFileNeitherSideHas(t *testing.T) {
	env := setupEngine(t)
	e := env.engine

	e.persistLineage("game1", env.peer.ID, mf("keep.sav", "kill.sav"), mf("keep.sav", "kill.sav"))

	// The deletion has propagated: gone from both. Now, and only now, it may
	// leave the record — keeping it forever would make a later file created
	// under the same name look like a deletion to propagate.
	e.persistLineage("game1", env.peer.ID, mf("keep.sav"), mf("keep.sav"))

	if got := lineageOf(t, env); contains(got, "kill.sav") {
		t.Errorf("a file neither side holds any more is still in the lineage (%v)", got)
	}
}

// The constraint the rule must not loosen: a path never confirmed on both
// sides must not get into the lineage by way of it. That is how a user's file
// was once deleted — an unconfirmed push recorded as shared, then read as
// "the peer deleted it" when the push had failed.
func TestPersistLineage_DoesNotAdmitAnUnconfirmedPath(t *testing.T) {
	env := setupEngine(t)
	e := env.engine

	e.persistLineage("game1", env.peer.ID, mf("keep.sav"), mf("keep.sav"))

	// A local-only file that has never been on the peer.
	e.persistLineage("game1", env.peer.ID, mf("keep.sav", "unpushed.sav"), mf("keep.sav"))

	if got := lineageOf(t, env); contains(got, "unpushed.sav") {
		t.Errorf("a file only this side has ever held entered the lineage (%v); if the push "+
			"failed, the next sync would delete the local original as 'removed by the peer'", got)
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// With lineage entries now surviving until a deletion propagates, the guard
// against destroying a re-created file has to hold on its own.
//
// This device deleted a file and wrote down its hash. Before that propagated,
// the peer wrote a NEW file under the same name. The lineage still says the
// path was shared; the record says what was deleted, and the peer's copy is
// not it. The record wins: their bytes are the newer fact.
func TestDecision_ARecordedDeletionWithADifferentHashPullsInsteadOfDeleting(t *testing.T) {
	local := mf("keep.sav")
	remote := delta.Manifest{Files: map[string]delta.FileEntry{
		"keep.sav":      {Hash: "h-keep.sav"},
		"recreated.sav": {Hash: "brand-new-content"},
	}}
	lineage := map[string]struct{}{"keep.sav": {}, "recreated.sav": {}}
	deleted := map[string]DeletedRecord{"recreated.sav": {Hash: "what-was-deleted"}}

	d := ComputeWithDeletions(local, remote, lineage, nil, "", deleted)

	if contains(d.FilesToDeleteOnPeer, "recreated.sav") {
		t.Fatal("a file the peer re-created with new content would be deleted on the peer; " +
			"the lineage was trusted over the deletion record that shows the content differs")
	}
	if !contains(d.FilesToPull, "recreated.sav") {
		t.Errorf("the peer's newer file was not pulled: %+v", d)
	}
}

// And when the record's hash DOES match, the lineage and the record agree and
// the deletion propagates as before.
func TestDecision_ARecordedDeletionWithAMatchingHashStillPropagates(t *testing.T) {
	local := mf("keep.sav")
	remote := mf("keep.sav", "kill.sav")
	lineage := map[string]struct{}{"keep.sav": {}, "kill.sav": {}}
	deleted := map[string]DeletedRecord{"kill.sav": {Hash: "h-kill.sav"}}

	d := ComputeWithDeletions(local, remote, lineage, nil, "", deleted)

	if !contains(d.FilesToDeleteOnPeer, "kill.sav") {
		t.Errorf("a deletion the record and the peer's copy agree on was not propagated: %+v", d)
	}
}
