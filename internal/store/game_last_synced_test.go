package store

import "testing"

// The per-game stamp answers "is my Deck up to date with THIS save", which
// the per-device stamp cannot: one device syncing three games writes one
// peers.last_synced, and the game that failed reads as fresh as the two that
// did not.
func TestGameLastSynced_PerGamePerPeer(t *testing.T) {
	s := openTestStore(t)

	got, err := s.GameLastSynced("game")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("a game that never synced reports %v, want nothing", got)
	}

	// Stamped before any lineage exists for the pair: the row has to be
	// created, not silently skipped.
	if err := s.UpdateGamePeerLastSynced("game", "deck", "2026-09-21T10:00:00.000Z"); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateGamePeerLastSynced("game", "laptop", "2026-09-21T11:00:00.000Z"); err != nil {
		t.Fatal(err)
	}
	// A different game with the same peer is a different answer.
	if err := s.UpdateGamePeerLastSynced("other", "deck", "2026-09-21T12:00:00.000Z"); err != nil {
		t.Fatal(err)
	}

	got, err = s.GameLastSynced("game")
	if err != nil {
		t.Fatal(err)
	}
	if got["deck"] != "2026-09-21T10:00:00.000Z" || got["laptop"] != "2026-09-21T11:00:00.000Z" || len(got) != 2 {
		t.Errorf("GameLastSynced = %v", got)
	}

	// A later sync moves the stamp and leaves the lineage alone.
	if err := s.SetSyncState("game", "deck", []string{"a.sav"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateGamePeerLastSynced("game", "deck", "2026-09-21T13:00:00.000Z"); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GameLastSynced("game")
	if got["deck"] != "2026-09-21T13:00:00.000Z" {
		t.Errorf("restamp: deck = %q", got["deck"])
	}
	files, _, err := s.GetSyncState("game", "deck")
	if err != nil || len(files) != 1 || files[0] != "a.sav" {
		t.Errorf("stamping touched the lineage: files=%v err=%v", files, err)
	}

	// Unpairing takes the rows with it, so nobody is shown a device that is
	// no longer paired as having synced.
	if err := s.UpsertPeer(Peer{ID: "deck", Name: "Deck", Address: "relay"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UnpairPeer("deck"); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GameLastSynced("game")
	if _, still := got["deck"]; still {
		t.Errorf("an unpaired device still has a last-synced stamp: %v", got)
	}
	if got["laptop"] == "" {
		t.Errorf("unpairing one device lost another's stamp: %v", got)
	}
}
