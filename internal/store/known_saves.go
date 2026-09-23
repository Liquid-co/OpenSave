package store

import (
	"fmt"
	"time"
)

// KnownSave is a save folder the background scan has seen. See
// migrations/0027_known_saves.sql.
type KnownSave struct {
	Path string
	Name string
}

// HasKnownSaves reports whether the background scan has ever recorded
// anything — false means the next scan is the first, which only takes stock.
func (s *Store) HasKnownSaves() (bool, error) {
	var n int
	if err := s.db.Get(&n, `SELECT COUNT(*) FROM known_saves`); err != nil {
		return false, fmt.Errorf("count known saves: %w", err)
	}
	return n > 0, nil
}

// IsKnownSave reports whether a folder has been seen before.
func (s *Store) IsKnownSave(path string) (bool, error) {
	var n int
	if err := s.db.Get(&n, `SELECT COUNT(*) FROM known_saves WHERE path = ?`, normalizeLocationPath(path)); err != nil {
		return false, fmt.Errorf("look up known save: %w", err)
	}
	return n > 0, nil
}

// RememberSaves records folders as seen. One already recorded keeps the
// moment it was first seen.
func (s *Store) RememberSaves(saves []KnownSave) error {
	if len(saves) == 0 {
		return nil
	}
	tx, err := s.db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339)
	for _, k := range saves {
		key := normalizeLocationPath(k.Path)
		if key == "" {
			continue
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO known_saves (path, name, first_seen) VALUES (?, ?, ?)`,
			key, k.Name, now); err != nil {
			return fmt.Errorf("remember save %s: %w", k.Path, err)
		}
	}
	return tx.Commit()
}

// PathsOverlap reports whether two save folders are the same folder or one
// sits inside the other, compared the way the store compares a game's
// locations — the same answer on every operating system.
func PathsOverlap(a, b string) bool { return overlaps(a, b) }
