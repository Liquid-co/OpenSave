package snapshot

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/opensave/opensave/internal/store"
)

func pin(t *testing.T, env *testEnv, snap store.Snapshot) {
	t.Helper()
	yes := true
	if _, err := env.mgr.EditSnapshot("game1", snap.ID, SnapshotEdit{Pinned: &yes}); err != nil {
		t.Fatalf("pin %s: %v", snap.ID, err)
	}
}

func stillThere(t *testing.T, env *testEnv, snap store.Snapshot) bool {
	t.Helper()
	_, rowErr := env.store.GetSnapshot(snap.ID)
	_, zipErr := os.Stat(snap.ZipPath)
	return rowErr == nil && zipErr == nil
}

// The count limits never take a pinned snapshot — of either kind — and a pin
// does not cost the unpinned ones their places: with a limit of 2, two
// unpinned snapshots stay beside the pinned one.
func TestPinnedSnapshotsSurviveTheCountLimits(t *testing.T) {
	env := setup(t)
	game, _ := env.store.GetGame("game1")
	game.MaxSnapshots = 2
	game.MaxManualSnapshots = 2
	if err := env.store.UpdateGame(game); err != nil {
		t.Fatal(err)
	}

	writeSave(t, env.saveDir, "slot1.sav", "first")
	oldAuto, _ := env.mgr.Create("game1", "", true)
	oldManual, _ := env.mgr.Create("game1", "before the boss", false)
	pin(t, env, oldAuto)
	pin(t, env, oldManual)

	var autos, manuals []store.Snapshot
	for i := 0; i < 4; i++ {
		writeSave(t, env.saveDir, "slot1.sav", "v"+string(rune('a'+i)))
		a, _ := env.mgr.Create("game1", "", true)
		m, _ := env.mgr.Create("game1", "later", false)
		autos = append(autos, a)
		manuals = append(manuals, m)
	}

	if !stillThere(t, env, oldAuto) || !stillThere(t, env, oldManual) {
		t.Fatal("a pinned snapshot was pruned by the count limit")
	}
	for i, s := range autos {
		if want := i >= 2; stillThere(t, env, s) != want {
			t.Errorf("automatic snapshot %d kept=%v, want %v: the limit of 2 applies to the unpinned ones", i, !want, want)
		}
	}
	for i, s := range manuals {
		if want := i >= 2; stillThere(t, env, s) != want {
			t.Errorf("manual snapshot %d kept=%v, want %v: the limit of 2 applies to the unpinned ones", i, !want, want)
		}
	}

	// Unpinned, it is an ordinary old snapshot again and the next prune takes it.
	no := false
	if _, err := env.mgr.EditSnapshot("game1", oldAuto.ID, SnapshotEdit{Pinned: &no}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := env.mgr.PruneAllGames(); err != nil {
		t.Fatal(err)
	}
	if stillThere(t, env, oldAuto) {
		t.Error("an unpinned snapshot beyond the limit survived the next prune")
	}
	if !stillThere(t, env, oldManual) {
		t.Error("unpinning one snapshot released another")
	}
}

// The age rule skips pinned snapshots, however old.
func TestPinnedSnapshotsSurviveTheAgeRule(t *testing.T) {
	env := setup(t)
	game, _ := env.store.GetGame("game1")
	game.MaxSnapshots = 0
	game.MaxManualSnapshots = 0
	if err := env.store.UpdateGame(game); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	env.mgr.now = func() time.Time { return now }

	writeSave(t, env.saveDir, "slot1.sav", "x")
	now = now.Add(-200 * 24 * time.Hour)
	ancientPinned, _ := env.mgr.Create("game1", "", true)
	now = now.Add(time.Second)
	ancient, _ := env.mgr.Create("game1", "", true)
	now = time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	_, _ = env.mgr.Create("game1", "", true) // the newest, kept regardless
	pin(t, env, ancientPinned)

	removed, _, _ := env.mgr.PruneOlderThan(30)
	if removed != 1 {
		t.Errorf("removed %d, want 1: only the unpinned old one", removed)
	}
	if !stillThere(t, env, ancientPinned) {
		t.Error("the age rule deleted a pinned snapshot")
	}
	if stillThere(t, env, ancient) {
		t.Error("the age rule skipped an unpinned old snapshot; the test proves nothing if the rule is off")
	}
}

// Clean-up sweeps away old conflict branches whole. One holding a pinned
// snapshot is left: sweeping the branch would take the pinned snapshot too.
func TestTheConflictBranchSweepSparesABranchWithAPin(t *testing.T) {
	env := setup(t)
	writeSave(t, env.saveDir, "slot1.sav", "state")
	for _, name := range []string{"conflict-kept", "conflict-swept"} {
		if _, err := env.mgr.CreateBranch("game1", name, true); err != nil {
			t.Fatal(err)
		}
	}
	kept, err := env.store.ListSnapshots("game1", "conflict-kept")
	if err != nil || len(kept) == 0 {
		t.Fatalf("no snapshot seeded on the branch: %v %v", kept, err)
	}
	pin(t, env, kept[0])

	if _, _, err := env.mgr.PruneAllGames(); err != nil {
		t.Fatal(err)
	}
	branches, _ := env.store.ListBranches("game1")
	joined := strings.Join(branches, ",")
	if !strings.Contains(joined, "conflict-kept") {
		t.Errorf("the branch with a pinned snapshot was swept: %v", branches)
	}
	if strings.Contains(joined, "conflict-swept") {
		t.Errorf("the other conflict branch was not swept, so the sweep never ran and this proves nothing: %v", branches)
	}
	if !stillThere(t, env, kept[0]) {
		t.Error("the pinned snapshot is gone")
	}
}

// A pin protects from the housekeeping, not from the person: deleting a
// pinned snapshot by hand works.
func TestAPinnedSnapshotCanStillBeDeletedByHand(t *testing.T) {
	env := setup(t)
	writeSave(t, env.saveDir, "slot1.sav", "x")
	snap, _ := env.mgr.Create("game1", "", false)
	pin(t, env, snap)
	if _, err := env.mgr.DeleteSnapshot("game1", snap.ID); err != nil {
		t.Fatalf("deleting a pinned snapshot by hand: %v", err)
	}
	if stillThere(t, env, snap) {
		t.Error("still there after being deleted")
	}
}

func TestEditSnapshotNotes(t *testing.T) {
	env := setup(t)
	writeSave(t, env.saveDir, "slot1.sav", "x")
	snap, _ := env.mgr.Create("game1", "Safety snapshot before restoring", true)

	note := "  good run, before the boss  "
	got, err := env.mgr.EditSnapshot("game1", snap.ID, SnapshotEdit{Note: &note})
	if err != nil {
		t.Fatal(err)
	}
	if got.Note != "good run, before the boss" {
		t.Errorf("note = %q, want it trimmed", got.Note)
	}
	if got.Comment != "Safety snapshot before restoring" {
		t.Errorf("comment = %q: a note must not overwrite the reason the snapshot was taken", got.Comment)
	}
	if got.Pinned {
		t.Error("writing a note pinned the snapshot")
	}

	// Pinning leaves the note alone, and an empty note removes it.
	yes := true
	got, _ = env.mgr.EditSnapshot("game1", snap.ID, SnapshotEdit{Pinned: &yes})
	if got.Note != "good run, before the boss" || !got.Pinned {
		t.Errorf("after pinning: %+v", got)
	}
	empty := "   "
	got, _ = env.mgr.EditSnapshot("game1", snap.ID, SnapshotEdit{Note: &empty})
	if got.Note != "" {
		t.Errorf("an empty note left %q", got.Note)
	}

	long := strings.Repeat("é", MaxNoteLength+1) // counted in characters, not bytes
	if _, err := env.mgr.EditSnapshot("game1", snap.ID, SnapshotEdit{Note: &long}); !errors.Is(err, ErrInvalidEdit) {
		t.Errorf("an over-long note: err = %v, want ErrInvalidEdit", err)
	}
	fits := strings.Repeat("é", MaxNoteLength)
	if _, err := env.mgr.EditSnapshot("game1", snap.ID, SnapshotEdit{Note: &fits}); err != nil {
		t.Errorf("a note of exactly the limit was refused: %v", err)
	}
}

// A snapshot is edited through its game: naming another game's snapshot, or
// one that does not exist, is not found — never an edit of the wrong save.
func TestEditSnapshotChecksTheGame(t *testing.T) {
	env := setup(t)
	if err := env.store.CreateGame(store.Game{ID: "game2", Name: "Game Two", SavePath: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	writeSave(t, env.saveDir, "slot1.sav", "x")
	snap, _ := env.mgr.Create("game1", "", false)
	yes := true
	if _, err := env.mgr.EditSnapshot("game2", snap.ID, SnapshotEdit{Pinned: &yes}); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("editing through the wrong game: err = %v, want not found", err)
	}
	if got, _ := env.store.GetSnapshot(snap.ID); got.Pinned {
		t.Error("the snapshot was pinned through the wrong game")
	}
	if _, err := env.mgr.EditSnapshot("game1", "snap_nope", SnapshotEdit{Pinned: &yes}); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("editing a snapshot that does not exist: err = %v, want not found", err)
	}
}
