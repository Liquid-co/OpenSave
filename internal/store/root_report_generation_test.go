package store

import (
	"path/filepath"
	"testing"
)

func TestMappedRootReportGenerationSurvivesRemoveReadd(t *testing.T) {
	s := openTestStore(t)
	if err := s.CreateGame(Game{ID: "generation", Name: "Generation", SavePath: filepath.Join(t.TempDir(), "main")}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddGameRoot("generation", "config", filepath.Join(t.TempDir(), "one")); err != nil {
		t.Fatal(err)
	}
	old, err := s.GameRootsMappedSince("generation")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveGameRoot("generation", "config"); err != nil {
		t.Fatal(err)
	}
	missing, err := s.GameRootsMappedSince("generation")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := missing["config"]; ok {
		t.Fatal("removed root still shown mapped")
	}
	if err := s.AddGameRoot("generation", "config", filepath.Join(t.TempDir(), "two")); err != nil {
		t.Fatal(err)
	}
	now, err := s.GameRootsMappedSince("generation")
	if err != nil {
		t.Fatal(err)
	}
	if now["config"] <= old["config"] {
		t.Fatalf("reused generation: %v then %v", old, now)
	}
	ok, err := s.SetSyncStateForRootIfMapped("generation", "peer", "config", old["config"], []string{"old.sav"}, nil)
	if err != nil || ok {
		t.Fatalf("accepted stale report: %v %v", ok, err)
	}
	ok, err = s.SetSyncStateForRootIfMapped("generation", "peer", "config", now["config"], []string{"new.sav"}, nil)
	if err != nil || !ok {
		t.Fatalf("rejected current report: %v %v", ok, err)
	}
	files, _, err := s.GetSyncStateForRoot("generation", "peer", "config")
	if err != nil || len(files) != 1 || files[0] != "new.sav" {
		t.Fatalf("wrong lineage: %v %v", files, err)
	}
}

// An asynchronous RefreshLineage must not write either files or an agreed
// base after RemoveRoot/AddRoot changes the mapping during manifest reads.
func TestRefreshedRootLineageRejectsOldGeneration(t *testing.T) {
	s := openTestStore(t)
	gameID := "refresh-generation"
	if err := s.CreateGame(Game{ID: gameID, Name: "Refresh", SavePath: filepath.Join(t.TempDir(), "main")}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddGameRoot(gameID, "config", filepath.Join(t.TempDir(), "first")); err != nil {
		t.Fatal(err)
	}
	before, err := s.GameRootsMappedSince(gameID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveGameRoot(gameID, "config"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddGameRoot(gameID, "config", filepath.Join(t.TempDir(), "second")); err != nil {
		t.Fatal(err)
	}
	committed, err := s.CommitRefreshedRootLineage(gameID, "peer", "config", before["config"], []string{"obsolete.sav"}, nil, "old-agreed")
	if err != nil || committed {
		t.Fatalf("old refresh committed: %v, %v", committed, err)
	}
	if files, _, err := s.GetSyncStateForRoot(gameID, "peer", "config"); err != nil || len(files) != 0 {
		t.Fatalf("stale refresh changed root lineage: %v, %v", files, err)
	}
	if hash := s.GetAgreedHashForRoot(gameID, "peer", "config"); hash != "" {
		t.Fatalf("stale refresh manufactured agreed hash: %q", hash)
	}
	current, err := s.GameRootsMappedSince(gameID)
	if err != nil {
		t.Fatal(err)
	}
	committed, err = s.CommitRefreshedRootLineage(gameID, "peer", "config", current["config"], []string{"new.sav"}, nil, "current-agreed")
	if err != nil || !committed {
		t.Fatalf("valid refresh was rejected: %v, %v", committed, err)
	}
	if hash := s.GetAgreedHashForRoot(gameID, "peer", "config"); hash != "current-agreed" {
		t.Fatalf("valid refresh did not advance agreed base: %q", hash)
	}
}

func TestRefreshedRootLineageLegacyZeroGeneration(t *testing.T) {
	s := openTestStore(t)
	gameID := "legacy-refresh"
	if err := s.CreateGame(Game{ID: gameID, Name: "Legacy", SavePath: filepath.Join(t.TempDir(), "main")}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddGameRoot(gameID, "config", filepath.Join(t.TempDir(), "legacy")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`DELETE FROM game_root_mapped WHERE game_id = ? AND name = ?`, gameID, "config"); err != nil {
		t.Fatal(err)
	}
	committed, err := s.CommitRefreshedRootLineage(gameID, "peer", "config", 0, []string{"legacy.sav"}, nil, "")
	if err != nil || !committed {
		t.Fatalf("legacy mapped root cannot refresh: %v, %v", committed, err)
	}
	if err := s.RemoveGameRoot(gameID, "config"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddGameRoot(gameID, "config", filepath.Join(t.TempDir(), "replacement")); err != nil {
		t.Fatal(err)
	}
	committed, err = s.CommitRefreshedRootLineage(gameID, "peer", "config", 0, []string{"stale.sav"}, nil, "")
	if err != nil || committed {
		t.Fatalf("legacy refresh contaminated re-added root: %v, %v", committed, err)
	}
}
