package store

import (
	"fmt"
	"time"
)

// AdoptHistory moves every snapshot of one game to another, onto branches of
// its own there, and returns the branch names it used. It is how linking two
// copies of a game keeps the history of the copy merged in: its entry is then
// deleted, and a snapshot row goes with its game's branch (ON DELETE CASCADE)
// while its archive stays on disk — history nothing listed, restored or
// cleaned up again.
//
// A branch named main becomes base; any other becomes base-<branch>. A name
// already in use gets a number. The archives stay where they are: a snapshot
// records its own path.
func (s *Store) AdoptHistory(fromID, toID, base string) ([]string, error) {
	tx, err := s.db.Beginx()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var from []string
	if err := tx.Select(&from, `SELECT name FROM branches WHERE game_id = ? ORDER BY name`, fromID); err != nil {
		return nil, fmt.Errorf("list %s's branches: %w", fromID, err)
	}
	var taken []string
	if err := tx.Select(&taken, `SELECT name FROM branches WHERE game_id = ?`, toID); err != nil {
		return nil, fmt.Errorf("list %s's branches: %w", toID, err)
	}
	used := map[string]bool{}
	for _, n := range taken {
		used[n] = true
	}

	var names []string
	for _, branch := range from {
		var count int
		if err := tx.Get(&count, `SELECT COUNT(*) FROM snapshots WHERE game_id = ? AND branch_name = ?`, fromID, branch); err != nil {
			return nil, err
		}
		if count == 0 {
			continue // an empty branch is not history
		}
		name := base
		if branch != "main" {
			name = base + "-" + branch
		}
		for n := 2; used[name]; n++ {
			name = fmt.Sprintf("%s-%d", base, n)
			if branch != "main" {
				name = fmt.Sprintf("%s-%s-%d", base, branch, n)
			}
		}
		used[name] = true
		if _, err := tx.Exec(`INSERT INTO branches (game_id, name) VALUES (?, ?)`, toID, name); err != nil {
			return nil, fmt.Errorf("create branch %s/%s: %w", toID, name, err)
		}
		if _, err := tx.Exec(`UPDATE snapshots SET game_id = ?, branch_name = ? WHERE game_id = ? AND branch_name = ?`,
			toID, name, fromID, branch); err != nil {
			return nil, fmt.Errorf("move %s/%s's snapshots: %w", fromID, branch, err)
		}
		names = append(names, name)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return names, nil
}

// AdoptCollections puts one game into every collection another is in, so a
// star or a collection is not lost when the other's entry goes.
func (s *Store) AdoptCollections(fromID, toID string) error {
	_, err := s.db.Exec(`INSERT OR IGNORE INTO collection_games (collection_id, game_id, added_ms)
		SELECT collection_id, ?, ? FROM collection_games WHERE game_id = ?`, toID, time.Now().UnixMilli(), fromID)
	if err != nil {
		return fmt.Errorf("carry %s's collections over to %s: %w", fromID, toID, err)
	}
	return nil
}
