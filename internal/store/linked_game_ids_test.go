package store

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func linkStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "opensave.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func joined(ids []string) string { return strings.Join(ids, ",") }

// game_aliases.game_id is a foreign key into games, so a link can only ever
// point at a game that exists — worth exercising rather than side-stepping,
// since it is part of what stops a stray id being accepted.
func mustGame(t *testing.T, s *Store, ids ...string) {
	t.Helper()
	for _, id := range ids {
		if err := s.CreateGame(Game{ID: id, Name: id, SavePath: t.TempDir()}); err != nil {
			t.Fatalf("CreateGame %s: %v", id, err)
		}
	}
}

// A game with no links must still match its own backups.
func TestLinkedGameIDs_UnlinkedGameMatchesOnlyItself(t *testing.T) {
	s := linkStore(t)
	got, err := s.LinkedGameIDs("elden-ring")
	if err != nil {
		t.Fatal(err)
	}
	if joined(got) != "elden-ring" {
		t.Errorf("LinkedGameIDs = %v, want just the id itself", got)
	}
}

// The case from the report: the same title tracked under two names on two
// devices, linked by the user. Either id must reach the same set, because
// either device may be the one asking.
func TestLinkedGameIDs_BothEndsOfALinkAgree(t *testing.T) {
	s := linkStore(t)
	mustGame(t, s, "elden-ring")
	if err := s.AddGameAlias("elden-ring-1", "elden-ring"); err != nil {
		t.Fatal(err)
	}

	fromCanonical, err := s.LinkedGameIDs("elden-ring")
	if err != nil {
		t.Fatal(err)
	}
	if joined(fromCanonical) != "elden-ring,elden-ring-1" {
		t.Errorf("from canonical = %v", fromCanonical)
	}

	fromAlias, err := s.LinkedGameIDs("elden-ring-1")
	if err != nil {
		t.Fatal(err)
	}
	if joined(fromAlias) != joined(fromCanonical) {
		t.Errorf("asking from the alias gave %v, from the canonical %v — they must agree",
			fromAlias, fromCanonical)
	}
}

// Widening what a game accepts is the whole risk here, so an unrelated game
// must never appear in the set.
func TestLinkedGameIDs_UnrelatedGamesAreNotIncluded(t *testing.T) {
	s := linkStore(t)
	mustGame(t, s, "elden-ring", "stardew-valley")
	if err := s.AddGameAlias("elden-ring-1", "elden-ring"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddGameAlias("stardew-2", "stardew-valley"); err != nil {
		t.Fatal(err)
	}

	got, err := s.LinkedGameIDs("elden-ring")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range got {
		if strings.HasPrefix(id, "stardew") {
			t.Fatalf("an unrelated game leaked into the set: %v", got)
		}
	}
	if joined(got) != "elden-ring,elden-ring-1" {
		t.Errorf("LinkedGameIDs = %v", got)
	}
}

// Several devices linked into one title all resolve together.
func TestLinkedGameIDs_ManyAliasesOnOneGame(t *testing.T) {
	s := linkStore(t)
	mustGame(t, s, "elden-ring")
	for _, a := range []string{"eldenring", "elden-ring-pc", "elden-ring-1"} {
		if err := s.AddGameAlias(a, "elden-ring"); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.LinkedGameIDs("eldenring")
	if err != nil {
		t.Fatal(err)
	}
	if joined(got) != "elden-ring,elden-ring-1,elden-ring-pc,eldenring" {
		t.Errorf("LinkedGameIDs = %v, want the canonical first then the aliases sorted", got)
	}
}

// The schema permits a -> b -> c. Resolution must reach the end rather than
// stopping one short, or half the set would be missed.
func TestLinkedGameIDs_FollowsAChain(t *testing.T) {
	s := linkStore(t)
	mustGame(t, s, "b", "c")
	if err := s.AddGameAlias("a", "b"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddGameAlias("b", "c"); err != nil {
		t.Fatal(err)
	}
	got, err := s.LinkedGameIDs("a")
	if err != nil {
		t.Fatal(err)
	}
	if got[0] != "c" {
		t.Errorf("LinkedGameIDs(a) = %v, want the chain resolved to c", got)
	}
}

// Nothing forbids a cycle in the schema, and a daemon that hangs on one would
// be a far worse bug than the one being fixed. This must terminate.
func TestLinkedGameIDs_TerminatesOnACycle(t *testing.T) {
	s := linkStore(t)
	mustGame(t, s, "a", "b")
	if err := s.AddGameAlias("a", "b"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddGameAlias("b", "a"); err != nil {
		t.Fatal(err)
	}

	done := make(chan []string, 1)
	go func() {
		got, err := s.LinkedGameIDs("a")
		if err != nil {
			t.Errorf("LinkedGameIDs: %v", err)
		}
		done <- got
	}()
	select {
	case got := <-done:
		if len(got) == 0 {
			t.Error("a cycle produced an empty set")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("LinkedGameIDs did not terminate on a cycle")
	}
}

func TestLinkedGameIDs_RejectsEmptyID(t *testing.T) {
	s := linkStore(t)
	if _, err := s.LinkedGameIDs("   "); err == nil {
		t.Error("expected an error for a blank game id")
	}
}
