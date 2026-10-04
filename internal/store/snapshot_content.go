package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// Snapshot content: which snapshots hold the same files, so a save is
// archived once however many times — and on however many devices — it is
// snapshotted. See migrations/0039_snapshot_content.sql.

// SetSnapshotContentHash records what a snapshot holds.
func (s *Store) SetSnapshotContentHash(id, hash string) error {
	if _, err := s.db.Exec(`UPDATE snapshots SET content_hash = ? WHERE id = ?`, hash, id); err != nil {
		return fmt.Errorf("set content hash of %s: %w", id, err)
	}
	return nil
}

// SnapshotByContent finds a snapshot of a game's branch holding exactly the
// content hash names; ok is false when there is none. The newest, so a copy
// kept in its place is the one retention keeps longest.
func (s *Store) SnapshotByContent(gameID, branch, hash string) (snap Snapshot, ok bool, err error) {
	if hash == "" {
		return Snapshot{}, false, nil
	}
	err = s.db.Get(&snap, `SELECT * FROM snapshots WHERE game_id = ? AND branch_name = ? AND content_hash = ?
		ORDER BY timestamp DESC LIMIT 1`, gameID, branch, hash)
	if errors.Is(err, sql.ErrNoRows) {
		return Snapshot{}, false, nil
	}
	if err != nil {
		return Snapshot{}, false, fmt.Errorf("find snapshot by content: %w", err)
	}
	return snap, true, nil
}

// AddSnapshotAlias records that alias (another device's snapshot id) names the
// same content as snapshotID here.
func (s *Store) AddSnapshotAlias(alias, snapshotID string) error {
	if alias == "" || alias == snapshotID {
		return nil
	}
	if _, err := s.db.Exec(`INSERT INTO snapshot_aliases (alias_id, snapshot_id) VALUES (?, ?)
		ON CONFLICT(alias_id) DO UPDATE SET snapshot_id = excluded.snapshot_id`, alias, snapshotID); err != nil {
		return fmt.Errorf("add snapshot alias %s: %w", alias, err)
	}
	return nil
}

// RepointSnapshotAliases moves every alias of from onto to, before from is
// removed in favour of to.
func (s *Store) RepointSnapshotAliases(from, to string) error {
	if _, err := s.db.Exec(`UPDATE snapshot_aliases SET snapshot_id = ? WHERE snapshot_id = ?`, to, from); err != nil {
		return fmt.Errorf("repoint aliases of %s: %w", from, err)
	}
	return nil
}

// ResolveSnapshotID says which snapshot here an id names: itself when it is
// one, the snapshot it is an alias of when it is that; ok false when neither.
func (s *Store) ResolveSnapshotID(id string) (string, bool) {
	var found string
	if err := s.db.Get(&found, `SELECT id FROM snapshots WHERE id = ?`, id); err == nil {
		return found, true
	}
	if err := s.db.Get(&found, `SELECT snapshot_id FROM snapshot_aliases WHERE alias_id = ?`, id); err == nil {
		return found, true
	}
	return "", false
}

// SnapshotsWithoutContentHash lists a game's snapshots whose content has not
// been computed yet.
func (s *Store) SnapshotsWithoutContentHash(gameID string) ([]Snapshot, error) {
	var out []Snapshot
	if err := s.db.Select(&out, `SELECT * FROM snapshots WHERE game_id = ? AND content_hash = '' ORDER BY timestamp`, gameID); err != nil {
		return nil, fmt.Errorf("list snapshots without content hash: %w", err)
	}
	return out, nil
}

// AllSnapshotsOfGame lists every snapshot of a game, on every branch, oldest
// first.
func (s *Store) AllSnapshotsOfGame(gameID string) ([]Snapshot, error) {
	var out []Snapshot
	if err := s.db.Select(&out, `SELECT * FROM snapshots WHERE game_id = ? ORDER BY timestamp`, gameID); err != nil {
		return nil, fmt.Errorf("list snapshots of %s: %w", gameID, err)
	}
	return out, nil
}
