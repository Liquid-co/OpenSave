package store

import (
	"fmt"
	"strings"
)

// The registry keys a game keeps save data in. See migration 0020 for why this
// is a table rather than a column.
//
// Paths are stored exactly as their source wrote them and normalised at the
// point of use — see the migration for why rewriting on the way in causes the
// same save to be captured twice under two spellings.

// SetGameRegistryKeys replaces a game's registry keys with the set given.
//
// Replace rather than merge: the manifest is the source of truth for what a
// game stores where, and a key it has stopped listing is one the game no longer
// writes. Merging would keep capturing a key forever after the game stopped
// using it, which quietly grows every snapshot with data nothing reads.
func (s *Store) SetGameRegistryKeys(gameID string, keys []string) error {
	tx, err := s.db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM game_registry_keys WHERE game_id = ?`, gameID); err != nil {
		return fmt.Errorf("clearing registry keys: %w", err)
	}
	seen := map[string]bool{}
	for _, k := range keys {
		k = strings.TrimSpace(k)
		if k == "" || seen[strings.ToLower(k)] {
			continue
		}
		seen[strings.ToLower(k)] = true
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO game_registry_keys (game_id, key_path) VALUES (?, ?)`,
			gameID, k); err != nil {
			return fmt.Errorf("adding registry key %q: %w", k, err)
		}
	}
	return tx.Commit()
}

// GameRegistryKeys returns a game's registry keys, in a stable order so two
// devices capture the same save in the same shape.
func (s *Store) GameRegistryKeys(gameID string) ([]string, error) {
	var keys []string
	err := s.db.Select(&keys,
		`SELECT key_path FROM game_registry_keys WHERE game_id = ? ORDER BY key_path`, gameID)
	if err != nil {
		return nil, fmt.Errorf("listing registry keys: %w", err)
	}
	return keys, nil
}

// GamesWithRegistryKeys returns the ids of every game that declares one, for
// callers that need to know which games need a registry pass at all.
func (s *Store) GamesWithRegistryKeys() ([]string, error) {
	var ids []string
	err := s.db.Select(&ids,
		`SELECT DISTINCT game_id FROM game_registry_keys ORDER BY game_id`)
	if err != nil {
		return nil, fmt.Errorf("listing games with registry keys: %w", err)
	}
	return ids, nil
}
