package store

import (
	"fmt"
	"time"
)

// SetPeerChoosesFolders lets a paired device choose the folders of games it
// syncs to this one, or stops it. See migration 0038.
func (s *Store) SetPeerChoosesFolders(peerID string, allowed bool) error {
	if !allowed {
		if _, err := s.db.Exec(`DELETE FROM peer_folder_trust WHERE peer_id = ?`, peerID); err != nil {
			return fmt.Errorf("stop %s choosing folders: %w", peerID, err)
		}
		return nil
	}
	if _, err := s.GetPeer(peerID); err != nil {
		return fmt.Errorf("let %s choose folders: %w", peerID, err)
	}
	if _, err := s.db.Exec(`INSERT OR IGNORE INTO peer_folder_trust (peer_id, since_ms) VALUES (?, ?)`,
		peerID, time.Now().UnixMilli()); err != nil {
		return fmt.Errorf("let %s choose folders: %w", peerID, err)
	}
	return nil
}

// PeerChoosesFolders reports whether a paired device may choose the folders of
// games it syncs to this one. False on any doubt.
func (s *Store) PeerChoosesFolders(peerID string) bool {
	if peerID == "" {
		return false
	}
	var n int
	if err := s.db.Get(&n, `SELECT COUNT(*) FROM peer_folder_trust WHERE peer_id = ?`, peerID); err != nil {
		return false
	}
	return n > 0
}
