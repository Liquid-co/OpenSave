package store

import (
	"path/filepath"
	"testing"
)

func regTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.CreateGame(Game{ID: "g1", Name: "Rounds", SavePath: t.TempDir()}); err != nil {
		t.Fatalf("create game: %v", err)
	}
	return s
}

func TestRegistryKeysRoundTrip(t *testing.T) {
	s := regTestStore(t)
	want := []string{
		"HKEY_CURRENT_USER/Software/Landfall Games/Rounds",
		"HKEY_CURRENT_USER/Software/Landfall Games/Rounds Extra",
	}
	if err := s.SetGameRegistryKeys("g1", want); err != nil {
		t.Fatal(err)
	}
	got, err := s.GameRegistryKeys("g1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != want[0] {
		t.Errorf("GameRegistryKeys = %v, want %v", got, want)
	}
}

// The path is stored exactly as its source wrote it. Rewriting on the way in
// would make a key learned from a peer compare unequal to the same key read
// from the manifest, and the game would capture one save twice.
func TestRegistryKeysAreStoredVerbatim(t *testing.T) {
	s := regTestStore(t)
	raw := "HKEY_CURRENT_USER/Software/Landfall Games/Rounds"
	if err := s.SetGameRegistryKeys("g1", []string{raw}); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GameRegistryKeys("g1")
	if len(got) != 1 || got[0] != raw {
		t.Errorf("stored %v, want the original spelling %q", got, raw)
	}
}

// The manifest is the source of truth: a key it has stopped listing is one the
// game no longer writes, and merging would capture it forever.
func TestSettingKeysReplacesRatherThanMerges(t *testing.T) {
	s := regTestStore(t)
	if err := s.SetGameRegistryKeys("g1", []string{"HKCU/Software/A", "HKCU/Software/B"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetGameRegistryKeys("g1", []string{"HKCU/Software/A"}); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GameRegistryKeys("g1")
	if len(got) != 1 || got[0] != "HKCU/Software/A" {
		t.Errorf("GameRegistryKeys = %v, want only the key still listed", got)
	}
}

func TestDuplicateKeysCollapse(t *testing.T) {
	s := regTestStore(t)
	if err := s.SetGameRegistryKeys("g1", []string{
		"HKCU/Software/A", "hkcu/software/a", "  ", "HKCU/Software/A",
	}); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GameRegistryKeys("g1"); len(got) != 1 {
		t.Errorf("GameRegistryKeys = %v, want one row", got)
	}
}

func TestGamesWithRegistryKeys(t *testing.T) {
	s := regTestStore(t)
	if err := s.CreateGame(Game{ID: "g2", Name: "Other", SavePath: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetGameRegistryKeys("g1", []string{"HKCU/Software/A"}); err != nil {
		t.Fatal(err)
	}
	ids, err := s.GamesWithRegistryKeys()
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != "g1" {
		t.Errorf("GamesWithRegistryKeys = %v, want [g1]", ids)
	}
}
