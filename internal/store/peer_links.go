package store

import "fmt"

// PeerLink is how fast this device has measured its connection to a peer.
// See migrations/0038_peer_links.sql.
type PeerLink struct {
	PeerID      string  `db:"peer_id"`
	BytesPerSec float64 `db:"bytes_per_sec"`
	MeasuredMs  int64   `db:"measured_ms"`
	ProbedMs    int64   `db:"probed_ms"`
}

// PeerLinks returns every recorded link, by peer.
func (s *Store) PeerLinks() (map[string]PeerLink, error) {
	var rows []PeerLink
	if err := s.db.Select(&rows, `SELECT * FROM peer_links`); err != nil {
		return nil, fmt.Errorf("list peer links: %w", err)
	}
	out := make(map[string]PeerLink, len(rows))
	for _, r := range rows {
		out[r.PeerID] = r
	}
	return out, nil
}

// SavePeerLink records a peer's link, replacing what was there.
func (s *Store) SavePeerLink(l PeerLink) error {
	_, err := s.db.Exec(`INSERT INTO peer_links (peer_id, bytes_per_sec, measured_ms, probed_ms) VALUES (?, ?, ?, ?)
		ON CONFLICT(peer_id) DO UPDATE SET bytes_per_sec = excluded.bytes_per_sec,
			measured_ms = excluded.measured_ms, probed_ms = excluded.probed_ms`,
		l.PeerID, l.BytesPerSec, l.MeasuredMs, l.ProbedMs)
	if err != nil {
		return fmt.Errorf("save peer link %s: %w", l.PeerID, err)
	}
	return nil
}
