package snapshot

import (
	"os"
	"testing"
	"time"

	"github.com/opensave/opensave/internal/store"
)

// The "auto-delete old backups" setting was stored, migrated from the old
// app, shown in Settings with a retention period — and read by nothing. This
// pins what it does now that it does something: automatic snapshots past the
// age go, and two things never do, whatever their age.
func TestPruneOlderThan_DeletesOldAutomaticSnapshotsOnly(t *testing.T) {
	env := setup(t)
	writeSave(t, env.saveDir, "slot1.sav", "x")
	// A generous count limit, so only age decides here.
	game, _ := env.store.GetGame("game1")
	game.MaxSnapshots = 0
	game.MaxManualSnapshots = 0
	if err := env.store.UpdateGame(game); err != nil {
		t.Fatal(err)
	}

	// A clock the test moves by hand: snapshots are stamped with it, and the
	// cutoff is computed from it.
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	env.mgr.now = func() time.Time { return now }
	at := func(daysAgo int, seq int) {
		now = time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC).Add(-time.Duration(daysAgo)*24*time.Hour + time.Duration(seq)*time.Second)
	}

	at(100, 0)
	oldAuto, _ := env.mgr.Create("game1", "before sync replaced local files", true)
	at(100, 1)
	oldManual, _ := env.mgr.Create("game1", "my deliberate save point", false)
	at(40, 0)
	middleAuto, _ := env.mgr.Create("game1", "", true)
	at(5, 0)
	recentAuto, _ := env.mgr.Create("game1", "", true)
	now = time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)

	removed, freed, touched := env.mgr.PruneOlderThan(30)
	if removed != 2 || freed <= 0 {
		t.Errorf("PruneOlderThan(30) removed %d (freed %d), want 2: the two automatic ones past 30 days", removed, freed)
	}
	if len(touched) != 1 || touched[0] != "game1" {
		t.Errorf("touched = %v, want [game1] so the dashboard is told", touched)
	}
	for _, c := range []struct {
		snap store.Snapshot
		keep bool
		why  string
	}{
		{oldAuto, false, "an automatic snapshot 100 days old is exactly what the setting removes"},
		{middleAuto, false, "40 days is past a 30-day limit"},
		{oldManual, true, "a snapshot the user took is never deleted by age"},
		{recentAuto, true, "5 days is within the limit, and it is the newest on the branch"},
	} {
		_, err := env.store.GetSnapshot(c.snap.ID)
		_, statErr := os.Stat(c.snap.ZipPath)
		if c.keep && (err != nil || statErr != nil) {
			t.Errorf("%s was deleted: %s (row err=%v zip err=%v)", c.snap.ID, c.why, err, statErr)
		}
		if !c.keep && (err == nil || statErr == nil) {
			t.Errorf("%s survived: %s (row err=%v zip err=%v)", c.snap.ID, c.why, err, statErr)
		}
	}

	// A second pass finds nothing more to do.
	if removed, _, _ := env.mgr.PruneOlderThan(30); removed != 0 {
		t.Errorf("a second sweep removed %d more", removed)
	}
}

// The newest snapshot on a branch is kept even when it is automatic and far
// past the limit: a game not played for a year still has its one copy.
func TestPruneOlderThan_KeepsTheNewestOnEachBranch(t *testing.T) {
	env := setup(t)
	writeSave(t, env.saveDir, "slot1.sav", "x")
	game, _ := env.store.GetGame("game1")
	game.MaxSnapshots = 0
	if err := env.store.UpdateGame(game); err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	clock := base
	env.mgr.now = func() time.Time { return clock }
	older, _ := env.mgr.Create("game1", "", true)
	clock = base.Add(time.Hour)
	newest, _ := env.mgr.Create("game1", "", true)
	clock = base.Add(300 * 24 * time.Hour) // ten months later

	removed, _, _ := env.mgr.PruneOlderThan(30)
	if removed != 1 {
		t.Errorf("removed %d, want 1: the older of two ancient automatic snapshots", removed)
	}
	if _, err := env.store.GetSnapshot(newest.ID); err != nil {
		t.Errorf("the branch's newest snapshot was deleted by age; the game now has no history at all")
	}
	if _, err := env.store.GetSnapshot(older.ID); err == nil {
		t.Errorf("the older ancient snapshot survived")
	}

	// And with the setting off — days <= 0 — nothing happens at all.
	if removed, _, _ := env.mgr.PruneOlderThan(0); removed != 0 {
		t.Errorf("PruneOlderThan(0) removed %d; zero must mean off", removed)
	}
}
