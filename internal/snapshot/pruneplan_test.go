package snapshot

import (
	"sort"
	"strings"
	"testing"
	"time"
)

// The plan is what the clean-up then does: every snapshot it lists is
// deleted, and nothing it does not list is. Built with every rule in play at
// once — count limits of both kinds, pins, an abandoned conflict branch, one
// kept by a pin, and the age rule — since the rules overlap, and an overlap
// is where a plan and the real thing would part.
func TestPrunePlanIsWhatCleanUpDeletes(t *testing.T) {
	env := setup(t)
	game, _ := env.store.GetGame("game1")
	game.MaxSnapshots = 2
	game.MaxManualSnapshots = 1
	if err := env.store.UpdateGame(game); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	env.mgr.now = func() time.Time { return now }

	// Old automatic snapshots on main — some past the count limit, some past
	// the age rule — manual ones past the manual limit, and one pinned.
	writeSave(t, env.saveDir, "slot1.sav", "a")
	for i := 0; i < 5; i++ {
		now = now.Add(-time.Duration(60-i) * 24 * time.Hour)
		snap, _ := env.mgr.Create("game1", "", i%2 == 0)
		if i == 1 {
			pin(t, env, snap)
		}
		now = time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	}
	// Two conflict branches, one kept by a pin.
	for _, name := range []string{"conflict-old", "conflict-kept"} {
		if _, err := env.mgr.CreateBranch("game1", name, true); err != nil {
			t.Fatal(err)
		}
	}
	kept, _ := env.store.ListSnapshots("game1", "conflict-kept")
	pin(t, env, kept[0])
	// And the age rule on.
	settings, _ := env.store.GetSettings()
	settings.AutoDeleteBackups = true
	settings.AutoDeleteDays = 30
	if err := env.store.UpdateSettings(settings); err != nil {
		t.Fatal(err)
	}

	plan, err := env.mgr.PrunePlan()
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) == 0 {
		t.Fatal("the plan is empty, so this test proves nothing")
	}
	before := allSnapshotIDs(t, env)

	if _, _, err := env.mgr.PruneAllGames(); err != nil {
		t.Fatal(err)
	}
	after := map[string]bool{}
	for _, id := range allSnapshotIDs(t, env) {
		after[id] = true
	}
	var deleted, planned []string
	for _, id := range before {
		if !after[id] {
			deleted = append(deleted, id)
		}
	}
	for _, s := range plan {
		planned = append(planned, s.ID)
	}
	sort.Strings(deleted)
	sort.Strings(planned)
	if strings.Join(deleted, ",") != strings.Join(planned, ",") {
		t.Errorf("clean-up deleted %v\nthe plan said        %v", deleted, planned)
	}

	// And after it, nothing is left to plan.
	if again, _ := env.mgr.PrunePlan(); len(again) != 0 {
		t.Errorf("after a clean-up the plan still lists %d snapshot(s)", len(again))
	}
}

func allSnapshotIDs(t *testing.T, env *testEnv) []string {
	t.Helper()
	branches, err := env.store.ListBranches("game1")
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, b := range branches {
		snaps, _ := env.store.ListSnapshots("game1", b)
		for _, s := range snaps {
			ids = append(ids, s.ID)
		}
	}
	return ids
}

// A game with no limits and nothing old has nothing to clean up.
func TestPrunePlanOfATidyLibraryIsEmpty(t *testing.T) {
	env := setup(t)
	game, _ := env.store.GetGame("game1")
	game.MaxSnapshots = 0
	game.MaxManualSnapshots = 0
	if err := env.store.UpdateGame(game); err != nil {
		t.Fatal(err)
	}
	writeSave(t, env.saveDir, "slot1.sav", "a")
	for i := 0; i < 3; i++ {
		env.mgr.Create("game1", "", true)
	}
	if plan, _ := env.mgr.PrunePlan(); len(plan) != 0 {
		t.Errorf("plan = %d snapshot(s), want none", len(plan))
	}
}
