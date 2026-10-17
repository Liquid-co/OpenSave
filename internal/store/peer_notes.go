package store

import "fmt"

// Peer notes: a choice made on this device that a peer has not acted on yet
// (migrations/0041_peer_notes.sql, syncengine/choices.go).

const (
	// NoteDeclined is the peer's save "Keep mine" declined, by manifest hash.
	NoteDeclined = "declined"
	// NoteLeftBranch is the branch this device was switched away from by hand.
	NoteLeftBranch = "left_branch"
)

// PeerNote returns a note, or "" when there is none.
func (s *Store) PeerNote(gameID, peerID, kind string) string {
	var v string
	if err := s.db.Get(&v, `SELECT value FROM peer_notes WHERE game_id = ? AND peer_id = ? AND kind = ?`,
		gameID, peerID, kind); err != nil {
		return ""
	}
	return v
}

// SetPeerNote records a note, replacing one of the same kind.
func (s *Store) SetPeerNote(gameID, peerID, kind, value string) error {
	if _, err := s.db.Exec(`INSERT INTO peer_notes (game_id, peer_id, kind, value) VALUES (?, ?, ?, ?)
		ON CONFLICT (game_id, peer_id, kind) DO UPDATE SET value = excluded.value`,
		gameID, peerID, kind, value); err != nil {
		return fmt.Errorf("record %s note for %s/%s: %w", kind, gameID, peerID, err)
	}
	return nil
}

// ClearPeerNote removes one note.
func (s *Store) ClearPeerNote(gameID, peerID, kind string) {
	_, _ = s.db.Exec(`DELETE FROM peer_notes WHERE game_id = ? AND peer_id = ? AND kind = ?`, gameID, peerID, kind)
}

// ClearGameNotes removes every note of a kind for a game, whichever peer it
// is about.
func (s *Store) ClearGameNotes(gameID, kind string) {
	_, _ = s.db.Exec(`DELETE FROM peer_notes WHERE game_id = ? AND kind = ?`, gameID, kind)
}
