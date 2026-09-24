package store

import (
	"fmt"
	"strings"
)

// UnpairedPeer is a device this one unpaired and may still owe a goodbye.
// See migration 0028 for why it is kept.
type UnpairedPeer struct {
	ID         string `db:"id"`
	Name       string `db:"name"`
	Address    string `db:"address"`
	Port       int    `db:"port"`
	PublicKey  string `db:"public_key"`
	UnpairedMs int64  `db:"unpaired_ms"`
}

// RememberUnpaired keeps what is needed to sign a goodbye to a device after
// its peer row is gone. One with no pinned key is not kept: nothing could be
// signed for it, and it never authenticated, so it accepts an unsigned
// goodbye anyway.
func (s *Store) RememberUnpaired(p UnpairedPeer) error {
	if strings.TrimSpace(p.PublicKey) == "" {
		return nil
	}
	if _, err := s.db.Exec(`INSERT INTO unpaired_peers (id, name, address, port, public_key, unpaired_ms)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET name = excluded.name, address = excluded.address,
			port = excluded.port, public_key = excluded.public_key, unpaired_ms = excluded.unpaired_ms`,
		p.ID, p.Name, p.Address, p.Port, p.PublicKey, p.UnpairedMs); err != nil {
		return fmt.Errorf("remember unpaired device %s: %w", p.ID, err)
	}
	return nil
}

// UnpairedPeers returns every device still owed a goodbye.
func (s *Store) UnpairedPeers() ([]UnpairedPeer, error) {
	var out []UnpairedPeer
	if err := s.db.Select(&out, `SELECT id, name, address, port, public_key, unpaired_ms
		FROM unpaired_peers ORDER BY unpaired_ms`); err != nil {
		return nil, fmt.Errorf("list unpaired devices: %w", err)
	}
	return out, nil
}

// ForgetUnpaired drops the record once the goodbye has landed, or once the
// device is paired again.
func (s *Store) ForgetUnpaired(id string) error {
	if _, err := s.db.Exec(`DELETE FROM unpaired_peers WHERE id = ?`, id); err != nil {
		return fmt.Errorf("forget unpaired device %s: %w", id, err)
	}
	return nil
}

// ForgetUnpairedBefore drops records older than cutoffMs and reports how
// many went.
func (s *Store) ForgetUnpairedBefore(cutoffMs int64) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM unpaired_peers WHERE unpaired_ms < ?`, cutoffMs)
	if err != nil {
		return 0, fmt.Errorf("expire unpaired devices: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}
