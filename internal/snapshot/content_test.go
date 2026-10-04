package snapshot

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The content a snapshot records and the content read back from its archive
// are named alike — which is what lets a device that only has another's
// archive, or only its file list, tell they hold the same save.
func TestContentKey_FromFilesAndFromArchiveAgree(t *testing.T) {
	env := setup(t)
	writeSave(t, env.saveDir, "slot1.sav", "one")
	writeSave(t, env.saveDir, "sub/slot2.sav", "two")
	snap, err := env.mgr.Create("game1", "", true)
	if err != nil {
		t.Fatal(err)
	}
	files, err := env.store.SnapshotFiles(snap.ID)
	if err != nil || len(files) == 0 {
		t.Fatalf("no file list recorded: %v", err)
	}
	fromArchive, err := ContentKeyOfArchive(snap.ZipPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := ContentKey(files); got != fromArchive {
		t.Errorf("from the file list %s, from the archive %s", got, fromArchive)
	}
	stored, _ := env.store.GetSnapshot(snap.ID)
	if stored.ContentHash != fromArchive {
		t.Errorf("recorded content %q, want %q", stored.ContentHash, fromArchive)
	}
}

// Steps on the way to a newer snapshot — every file of them in it, unchanged —
// go. A newer snapshot that lacks files (something was deleted) does not make
// the older one redundant; pinned and hand-taken snapshots are never removed.
func TestPruneContained(t *testing.T) {
	env := setup(t)
	game, err := env.store.GetGame("game1")
	if err != nil {
		t.Fatal(err)
	}
	game.MaxSnapshots = 50
	if err := env.store.UpdateGame(game); err != nil {
		t.Fatal(err)
	}
	snap := func(auto bool) string {
		t.Helper()
		s, err := env.mgr.Create("game1", "", auto)
		if err != nil {
			t.Fatal(err)
		}
		return s.ID
	}
	writeSave(t, env.saveDir, "a.bin", "a")
	step1 := snap(true)
	writeSave(t, env.saveDir, "b.bin", "b")
	step2 := snap(true)
	pinnedStep := snap(false) // same files as step2, by hand, then pinned below
	_ = env.store.SetSnapshotPinned(pinnedStep, true)
	writeSave(t, env.saveDir, "c.bin", "c")
	full := snap(true)
	// Then a file is deleted on purpose.
	if err := os.Remove(filepath.Join(env.saveDir, "c.bin")); err != nil {
		t.Fatal(err)
	}
	writeSave(t, env.saveDir, "a.bin", "a, played on")
	after := snap(true)

	removed, _, err := env.mgr.PruneContained("game1")
	if err != nil {
		t.Fatal(err)
	}
	gone := func(id string) bool { _, err := env.store.GetSnapshot(id); return err != nil }
	if !gone(step1) || !gone(step2) {
		t.Errorf("steps on the way to a newer snapshot kept: step1 gone=%v step2 gone=%v", gone(step1), gone(step2))
	}
	if gone(pinnedStep) {
		t.Error("a pinned snapshot was removed")
	}
	if gone(full) {
		t.Error("removed a snapshot holding a file deleted later on purpose")
	}
	if gone(after) {
		t.Error("removed the newest snapshot")
	}
	if removed != 2 {
		t.Errorf("removed %d, want 2", removed)
	}
}

// With PruneOnCreate, a new snapshot takes the place of older automatic ones it
// holds entirely, as it is taken; one that lacks a file of them does not.
func TestPruneContainedBy_ANewSnapshotReplacesItsSteps(t *testing.T) {
	env := setup(t)
	env.mgr.PruneOnCreate = true
	game, _ := env.store.GetGame("game1")
	game.MaxSnapshots = 50
	_ = env.store.UpdateGame(game)

	writeSave(t, env.saveDir, "a.bin", "a")
	first, err := env.mgr.Create("game1", "", true)
	if err != nil {
		t.Fatal(err)
	}
	writeSave(t, env.saveDir, "b.bin", "b")
	if _, err := env.mgr.Create("game1", "", true); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := env.store.GetSnapshot(first.ID); err != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if _, err := env.store.GetSnapshot(first.ID); err == nil {
		t.Fatal("the step the new snapshot holds entirely was kept")
	}

	// Now a file goes: the newer snapshot does not hold the older one.
	before, _ := env.store.ListSnapshots("game1", "main")
	if err := os.Remove(filepath.Join(env.saveDir, "b.bin")); err != nil {
		t.Fatal(err)
	}
	if _, err := env.mgr.Create("game1", "", true); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	if _, err := env.store.GetSnapshot(before[0].ID); err != nil {
		t.Error("removed the snapshot holding a file deleted afterwards")
	}
}

// A copy taken before a sync replaces a save already archived is that archive,
// not a second one.
func TestBeforeReplacing_AnArchivedSaveIsNotArchivedAgain(t *testing.T) {
	env := setup(t)
	writeSave(t, env.saveDir, "slot1.sav", "progress")
	first, err := env.mgr.Create("game1", "", true)
	if err != nil {
		t.Fatal(err)
	}
	copy, err := env.mgr.CreateBeforeReplacing("game1", "before sync")
	if err != nil {
		t.Fatal(err)
	}
	if copy.ID != first.ID {
		t.Errorf("a second archive %s of the save %s already holds", copy.ID, first.ID)
	}
	snaps, _ := env.store.ListSnapshots("game1", "main")
	if len(snaps) != 1 {
		t.Errorf("%d snapshots, want 1", len(snaps))
	}
}

// Several snapshots of the same files — one per device the save was compared
// with — are kept once: the newest unless one is pinned or taken by hand;
// pinned ones are never removed; the others' ids still lead to it.
func TestMergeDuplicates(t *testing.T) {
	env := setup(t)
	// Room for every snapshot below: this is about merging, not retention.
	game, err := env.store.GetGame("game1")
	if err != nil {
		t.Fatal(err)
	}
	game.MaxSnapshots = 50
	if err := env.store.UpdateGame(game); err != nil {
		t.Fatal(err)
	}
	writeSave(t, env.saveDir, "slot1.sav", "the save")
	var same []string
	for i := 0; i < 3; i++ {
		s, err := env.mgr.Create("game1", "", true)
		if err != nil {
			t.Fatal(err)
		}
		same = append(same, s.ID)
	}
	pinned, err := env.mgr.Create("game1", "pinned copy", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := env.store.SetSnapshotPinned(pinned.ID, true); err != nil {
		t.Fatal(err)
	}
	writeSave(t, env.saveDir, "slot1.sav", "played on")
	other, err := env.mgr.Create("game1", "", true)
	if err != nil {
		t.Fatal(err)
	}
	// As if taken before contents were recorded.
	for _, id := range append(same, pinned.ID, other.ID) {
		_ = env.store.SetSnapshotContentHash(id, "")
	}

	merged, _, err := env.mgr.MergeDuplicates("game1")
	if err != nil {
		t.Fatal(err)
	}
	if merged != 3 {
		t.Errorf("merged %d, want the 3 unpinned copies of the pinned one", merged)
	}
	if _, err := env.store.GetSnapshot(pinned.ID); err != nil {
		t.Error("the pinned snapshot was removed")
	}
	if _, err := env.store.GetSnapshot(other.ID); err != nil {
		t.Error("a snapshot of different content was removed")
	}
	for _, id := range same {
		if got, ok := env.store.ResolveSnapshotID(id); !ok || got != pinned.ID {
			t.Errorf("%s leads to %q (%v), want the snapshot kept, %s", id, got, ok, pinned.ID)
		}
	}
	// Run again: nothing more to do.
	if again, _, _ := env.mgr.MergeDuplicates("game1"); again != 0 {
		t.Errorf("a second pass merged %d more", again)
	}
}

// A session's named snapshot, then the watcher's copy of the same save a
// moment later: the copy takes its place and keeps its name.
func TestPruneContainedBy_AnIdenticalCopyKeepsTheName(t *testing.T) {
	env := setup(t)
	env.mgr.PruneOnCreate = true
	writeSave(t, env.saveDir, "slot1.sav", "after an evening")
	named, err := env.mgr.Create("game1", "After playing (1 h)", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := env.store.SetSnapshotNote(named.ID, "boss beaten"); err != nil {
		t.Fatal(err)
	}
	copied, err := env.mgr.Create("game1", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if copied.ID == named.ID {
		t.Skip("the copy was not taken: nothing to replace")
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := env.store.GetSnapshot(named.ID); err != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	kept, err := env.store.GetSnapshot(copied.ID)
	if err != nil {
		t.Fatal(err)
	}
	if kept.Comment != "After playing (1 h)" || kept.Note != "boss beaten" {
		t.Errorf("the snapshot kept is called %q (note %q), want the name and note of the one it replaced", kept.Comment, kept.Note)
	}
}
