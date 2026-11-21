package store

import "fmt"

// PeerBranch is the branch a paired device was on at the last sync of a game
// with it (migrations/0048_peer_branches.sql).
type PeerBranch struct {
	Branch string `db:"branch" json:"branch"`
	SeenMs int64  `db:"seen_ms" json:"seenMs"`
}

// NotePeerBranch records the branch a device answered a sync with.
func (s *Store) NotePeerBranch(gameID, peerID, branch string, seenMs int64) error {
	if branch == "" {
		return nil
	}
	if _, err := s.db.Exec(`INSERT INTO peer_branches (game_id, peer_id, branch, seen_ms) VALUES (?, ?, ?, ?)
		ON CONFLICT (game_id, peer_id) DO UPDATE SET branch = excluded.branch, seen_ms = excluded.seen_ms`,
		gameID, peerID, branch, seenMs); err != nil {
		return fmt.Errorf("record %s's branch of %s: %w", peerID, gameID, err)
	}
	return nil
}

// PeerBranches returns, for one game, each paired device's branch as of its
// last sync: peer id to branch.
func (s *Store) PeerBranches(gameID string) (map[string]PeerBranch, error) {
	var rows []struct {
		PeerID string `db:"peer_id"`
		PeerBranch
	}
	if err := s.db.Select(&rows, `SELECT peer_id, branch, seen_ms FROM peer_branches WHERE game_id = ?`, gameID); err != nil {
		return nil, fmt.Errorf("peer branches of %s: %w", gameID, err)
	}
	out := make(map[string]PeerBranch, len(rows))
	for _, r := range rows {
		out[r.PeerID] = r.PeerBranch
	}
	return out, nil
}
