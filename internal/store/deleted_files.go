package store

import (
	"fmt"
	"strings"
	"time"
)

// DeletedFile is a deletion this device made, kept so that propagating it does
// not depend on inferring it later from a set that can change underneath.
//
// See migrations/0022_deleted_files.sql for why this exists and why Hash is
// the part that makes acting on it safe.
type DeletedFile struct {
	GameID string `db:"game_id"`
	Root   string `db:"root"`
	Path   string `db:"path"`
	// Hash is the content at the moment of deletion. A peer whose copy hashes
	// differently has edited it since; their bytes win and this record must
	// not be used to remove them.
	Hash        string `db:"hash"`
	DeletedAtMs int64  `db:"deleted_at_ms"`
}

// DeletedFileRetention is how long a record is kept.
//
// Long enough that a device switched off for a holiday still learns about the
// deletion when it comes back; short enough that the table does not grow for
// the life of the install. Syncthing's own bug tracker has a case of stale
// deletion records resurrecting files years later, which is the failure this
// bound exists to avoid.
const DeletedFileRetention = 90 * 24 * time.Hour

// RecordDeletedFiles writes deletions observed for one game. Existing records
// for the same path are replaced: the newest deletion is the one that
// describes what is on disk now.
func (s *Store) RecordDeletedFiles(gameID string, files []DeletedFile) error {
	if strings.TrimSpace(gameID) == "" || len(files) == 0 {
		return nil
	}
	tx, err := s.db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Preparex(`
		INSERT INTO deleted_files (game_id, root, path, hash, deleted_at_ms)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(game_id, root, path) DO UPDATE SET
			hash          = excluded.hash,
			deleted_at_ms = excluded.deleted_at_ms`)
	if err != nil {
		return fmt.Errorf("prepare deleted-file insert: %w", err)
	}
	defer stmt.Close()

	now := time.Now().UnixMilli()
	for _, f := range files {
		if strings.TrimSpace(f.Path) == "" || strings.TrimSpace(f.Hash) == "" {
			// A record with no path cannot be matched, and one with no hash
			// cannot be checked against the peer's copy — which is the only
			// thing that makes propagating it safe. Skipped rather than
			// stored: a record that cannot be used safely is worse than none.
			continue
		}
		at := f.DeletedAtMs
		if at == 0 {
			at = now
		}
		if _, err := stmt.Exec(gameID, f.Root, f.Path, f.Hash, at); err != nil {
			return fmt.Errorf("record deletion of %s: %w", f.Path, err)
		}
	}
	return tx.Commit()
}

// DeletedFiles returns the recorded deletions for one game root, keyed by
// relative path.
func (s *Store) DeletedFiles(gameID, root string) (map[string]DeletedFile, error) {
	var rows []DeletedFile
	if err := s.db.Select(&rows, `
		SELECT game_id, root, path, hash, deleted_at_ms
		FROM deleted_files WHERE game_id = ? AND root = ?`, gameID, root); err != nil {
		return nil, fmt.Errorf("read deletions for %s: %w", gameID, err)
	}
	out := make(map[string]DeletedFile, len(rows))
	for _, r := range rows {
		out[r.Path] = r
	}
	return out, nil
}

// ClearDeletedFile forgets one recorded deletion.
//
// Called when the path exists locally again — the file was restored, or the
// game wrote it back — because at that point the record describes something
// that is no longer true, and leaving it would let a later sync delete the
// peer's copy of a file this device now has.
func (s *Store) ClearDeletedFile(gameID, root, path string) error {
	_, err := s.db.Exec(
		`DELETE FROM deleted_files WHERE game_id = ? AND root = ? AND path = ?`,
		gameID, root, path)
	if err != nil {
		return fmt.Errorf("clear deletion record for %s: %w", path, err)
	}
	return nil
}

// ClearDeletedFilesForGame forgets every deletion recorded for a game, for use
// when it is untracked — the records describe a folder this device no longer
// has an opinion about.
func (s *Store) ClearDeletedFilesForGame(gameID string) error {
	_, err := s.db.Exec(`DELETE FROM deleted_files WHERE game_id = ?`, gameID)
	if err != nil {
		return fmt.Errorf("clear deletion records for %s: %w", gameID, err)
	}
	return nil
}

// PruneDeletedFiles drops records older than DeletedFileRetention.
func (s *Store) PruneDeletedFiles() error {
	cutoff := time.Now().Add(-DeletedFileRetention).UnixMilli()
	_, err := s.db.Exec(`DELETE FROM deleted_files WHERE deleted_at_ms < ?`, cutoff)
	if err != nil {
		return fmt.Errorf("prune deletion records: %w", err)
	}
	return nil
}
