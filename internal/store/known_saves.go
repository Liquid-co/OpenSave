package store

import (
	"fmt"
	"time"
)

// KnownSave is a save folder the background scan has seen. See
// migrations/0027_known_saves.sql.
type KnownSave struct {
	Path  string `db:"save_path"`
	Name  string `db:"name"`
	AppID string `db:"app_id"`
	// Pending marks one announced and not yet looked at.
	Pending bool `db:"pending"`
}

// StockTaken reports whether the first background scan has run.
func (s *Store) StockTaken() (bool, error) {
	var n int
	if err := s.db.Get(&n, `SELECT COUNT(*) FROM new_games_state`); err != nil {
		return false, fmt.Errorf("read new-games state: %w", err)
	}
	return n > 0, nil
}

// MarkStockTaken records that the first background scan has run.
func (s *Store) MarkStockTaken() error {
	_, err := s.db.Exec(`INSERT OR IGNORE INTO new_games_state (id, stock_taken) VALUES (1, ?)`,
		time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("mark stock taken: %w", err)
	}
	return nil
}

// IsKnownSave reports whether a folder has been seen before.
func (s *Store) IsKnownSave(path string) (bool, error) {
	var n int
	if err := s.db.Get(&n, `SELECT COUNT(*) FROM known_saves WHERE path = ?`, normalizeLocationPath(path)); err != nil {
		return false, fmt.Errorf("look up known save: %w", err)
	}
	return n > 0, nil
}

// RememberSaves records folders as seen. One already recorded is left as it
// is — its first sighting, and whether it is waiting.
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
		if _, err := tx.Exec(`INSERT OR IGNORE INTO known_saves (path, save_path, name, app_id, first_seen, pending)
			VALUES (?, ?, ?, ?, ?, ?)`, key, k.Path, k.Name, k.AppID, now, k.Pending); err != nil {
			return fmt.Errorf("remember save %s: %w", k.Path, err)
		}
	}
	return tx.Commit()
}

// PendingSaves lists the announced folders nobody has looked at yet, oldest
// first.
func (s *Store) PendingSaves() ([]KnownSave, error) {
	var out []KnownSave
	if err := s.db.Select(&out, `SELECT save_path, name, app_id, pending FROM known_saves
		WHERE pending = 1 ORDER BY first_seen, save_path`); err != nil {
		return nil, fmt.Errorf("list waiting saves: %w", err)
	}
	return out, nil
}

// ClearPendingSaves marks every announced folder as looked at.
func (s *Store) ClearPendingSaves() error {
	if _, err := s.db.Exec(`UPDATE known_saves SET pending = 0 WHERE pending = 1`); err != nil {
		return fmt.Errorf("clear waiting saves: %w", err)
	}
	return nil
}

// PathsOverlap reports whether two save folders are the same folder or one
// sits inside the other, compared the way the store compares a game's
// locations — the same answer on every operating system.
func PathsOverlap(a, b string) bool { return overlaps(a, b) }
