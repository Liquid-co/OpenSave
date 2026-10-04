package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
)

// Save versions: which version of a game's save this device holds. See
// migrations/0037_game_versions.sql for the columns and
// internal/p2p/syncengine/version.go for the rules. The vectors are kept as
// the JSON the sync engine writes; this layer does not interpret them.

// GameVersion is one game's version record on this device.
type GameVersion struct {
	GameID  string `db:"game_id"`
	Key     string `db:"key"`
	Counter int64  `db:"counter"`
	Vector  string `db:"vector"`
	Target  string `db:"target"`
	Pulling bool   `db:"pulling"`
	Hash    string `db:"hash"`
	// Answered is the JSON vector this device kept its save over in a
	// conflict, AnsweredAt when.
	Answered   string `db:"answered"`
	AnsweredAt int64  `db:"answered_at"`
}

// GetGameVersion returns a game's version record, creating an empty one —
// with a fresh writer key — the first time it is asked for. A game that is not
// tracked gets a record that is not stored.
func (s *Store) GetGameVersion(gameID string) (GameVersion, error) {
	var v GameVersion
	err := s.db.Get(&v, `SELECT * FROM game_versions WHERE game_id = ?`, gameID)
	if err == nil {
		return v, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return GameVersion{}, fmt.Errorf("get game version %s: %w", gameID, err)
	}
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return GameVersion{}, err
	}
	v = GameVersion{GameID: gameID, Key: hex.EncodeToString(b[:]), Vector: "{}"}
	// OR IGNORE: two callers creating it at once keep the first key.
	if _, err := s.db.Exec(`INSERT OR IGNORE INTO game_versions (game_id, key, vector)
		SELECT ?, ?, '{}' WHERE EXISTS (SELECT 1 FROM games WHERE id = ?)`,
		gameID, v.Key, gameID); err != nil {
		return GameVersion{}, fmt.Errorf("create game version %s: %w", gameID, err)
	}
	if err := s.db.Get(&v, `SELECT * FROM game_versions WHERE game_id = ?`, gameID); err != nil &&
		!errors.Is(err, sql.ErrNoRows) {
		return GameVersion{}, fmt.Errorf("get game version %s: %w", gameID, err)
	}
	return v, nil
}

// SaveGameVersion writes a game's version record. The writer key is never
// changed by this: it is the record's identity.
func (s *Store) SaveGameVersion(v GameVersion) error {
	_, err := s.db.Exec(`UPDATE game_versions SET counter = ?, vector = ?, target = ?, pulling = ?, hash = ?,
		answered = ?, answered_at = ? WHERE game_id = ?`,
		v.Counter, v.Vector, v.Target, v.Pulling, v.Hash, v.Answered, v.AnsweredAt, v.GameID)
	if err != nil {
		return fmt.Errorf("save game version %s: %w", v.GameID, err)
	}
	return nil
}
