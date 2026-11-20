package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// BackupFileMade is when this device last made a backup file, and how many
// games went into it (migrations/0047_backup_file_made.sql).
type BackupFileMade struct {
	MadeMs int64 `db:"made_ms" json:"madeMs"`
	Games  int   `db:"games" json:"games"`
}

// NoteBackupFileMade records a backup file just made.
func (s *Store) NoteBackupFileMade(madeMs int64, games int) error {
	if _, err := s.db.Exec(`INSERT INTO backup_file_made (id, made_ms, games) VALUES (1, ?, ?)
		ON CONFLICT (id) DO UPDATE SET made_ms = excluded.made_ms, games = excluded.games`, madeMs, games); err != nil {
		return fmt.Errorf("record the backup file made: %w", err)
	}
	return nil
}

// LastBackupFile returns the last backup file made, and false when none has
// been.
func (s *Store) LastBackupFile() (BackupFileMade, bool, error) {
	var b BackupFileMade
	err := s.db.Get(&b, `SELECT made_ms, games FROM backup_file_made WHERE id = 1`)
	if errors.Is(err, sql.ErrNoRows) {
		return BackupFileMade{}, false, nil
	}
	if err != nil {
		return BackupFileMade{}, false, fmt.Errorf("read the last backup file made: %w", err)
	}
	return b, true, nil
}
