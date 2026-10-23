package store

import (
	"fmt"
	"time"
)

// Verdicts on a file in a game's save folder (migrations/0042_settings_files.sql).
const (
	VerdictSettings = "settings"
	VerdictSave     = "save"
)

// SettingsFile is a verdict on one file of a game's main save folder.
type SettingsFile struct {
	GameID  string `db:"game_id" json:"-"`
	Path    string `db:"path" json:"path"`
	Verdict string `db:"verdict" json:"verdict"`
	Reason  string `db:"reason" json:"reason"`
	AtMs    int64  `db:"at_ms" json:"atMs"`
}

// SettingsFiles returns every verdict recorded for a game.
func (s *Store) SettingsFiles(gameID string) ([]SettingsFile, error) {
	var out []SettingsFile
	if err := s.db.Select(&out, `SELECT * FROM settings_files WHERE game_id = ? ORDER BY path`, gameID); err != nil {
		return nil, fmt.Errorf("list settings files of %s: %w", gameID, err)
	}
	return out, nil
}

// SetSettingsFile records a verdict, replacing any earlier one.
func (s *Store) SetSettingsFile(f SettingsFile) error {
	if f.AtMs == 0 {
		f.AtMs = time.Now().UnixMilli()
	}
	if _, err := s.db.NamedExec(`INSERT INTO settings_files (game_id, path, verdict, reason, at_ms)
		VALUES (:game_id, :path, :verdict, :reason, :at_ms)
		ON CONFLICT(game_id, path) DO UPDATE SET verdict = excluded.verdict, reason = excluded.reason, at_ms = excluded.at_ms`, f); err != nil {
		return fmt.Errorf("set settings file %s/%s: %w", f.GameID, f.Path, err)
	}
	return nil
}

// SettingsObservation is what one device's copy of an excluded settings file
// has done so far.
type SettingsObservation struct {
	GameID   string `db:"game_id"`
	PeerID   string `db:"peer_id"`
	Path     string `db:"path"`
	LastHash string `db:"last_hash"`
	Moved    int    `db:"moved"`
	Still    int    `db:"still"`
}

// SettingsObservations returns what every device's copy of path has done.
func (s *Store) SettingsObservations(gameID, path string) ([]SettingsObservation, error) {
	var out []SettingsObservation
	if err := s.db.Select(&out, `SELECT * FROM settings_observations WHERE game_id = ? AND path = ?`, gameID, path); err != nil {
		return nil, fmt.Errorf("list settings observations: %w", err)
	}
	return out, nil
}

// GetSettingsObservation returns one device's record ({} when none).
func (s *Store) GetSettingsObservation(gameID, peerID, path string) SettingsObservation {
	var o SettingsObservation
	if err := s.db.Get(&o, `SELECT * FROM settings_observations WHERE game_id = ? AND peer_id = ? AND path = ?`, gameID, peerID, path); err != nil {
		return SettingsObservation{GameID: gameID, PeerID: peerID, Path: path}
	}
	return o
}

// SaveSettingsObservation writes one device's record.
func (s *Store) SaveSettingsObservation(o SettingsObservation) error {
	if _, err := s.db.NamedExec(`INSERT INTO settings_observations (game_id, peer_id, path, last_hash, moved, still)
		VALUES (:game_id, :peer_id, :path, :last_hash, :moved, :still)
		ON CONFLICT(game_id, peer_id, path) DO UPDATE SET last_hash = excluded.last_hash,
			moved = excluded.moved, still = excluded.still`, o); err != nil {
		return fmt.Errorf("save settings observation: %w", err)
	}
	return nil
}
