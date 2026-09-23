package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// CloudHead is where one game's save stands on this device, as the cloud
// mirror sees it. See migrations/0026_cloud_heads.sql.
type CloudHead struct {
	GameID    string   `db:"game_id"`
	Snapshot  string   `db:"snapshot"`
	Since     string   `db:"since"`
	File      string   `db:"file"`
	Chain     []string `db:"-"`
	ChainJSON string   `db:"chain"`
	Published string   `db:"published"`
	Dismissed string   `db:"dismissed"`
}

// GetCloudHead returns the game's record, and false when it has none yet.
func (s *Store) GetCloudHead(gameID string) (CloudHead, bool, error) {
	var h CloudHead
	err := s.db.Get(&h, `SELECT * FROM cloud_heads WHERE game_id = ?`, gameID)
	if errors.Is(err, sql.ErrNoRows) {
		return CloudHead{GameID: gameID}, false, nil
	}
	if err != nil {
		return CloudHead{}, false, fmt.Errorf("get cloud head %s: %w", gameID, err)
	}
	if h.ChainJSON != "" {
		if err := json.Unmarshal([]byte(h.ChainJSON), &h.Chain); err != nil {
			return CloudHead{}, false, fmt.Errorf("cloud head %s: chain: %w", gameID, err)
		}
	}
	return h, true, nil
}

// SetCloudHead writes the game's record.
//
// Refused, silently, for a game that is not tracked. These writes come from
// snapshots and uploads finishing in the background, and one landing just
// after an untrack would otherwise bring back a record from the game's
// previous life — the shape of the bug that once cost a paired device a file.
func (s *Store) SetCloudHead(h CloudHead) error {
	chain := h.Chain
	if chain == nil {
		chain = []string{}
	}
	b, err := json.Marshal(chain)
	if err != nil {
		return err
	}
	h.ChainJSON = string(b)
	_, err = s.db.NamedExec(`
		INSERT INTO cloud_heads (game_id, snapshot, since, file, chain, published, dismissed)
		SELECT :game_id, :snapshot, :since, :file, :chain, :published, :dismissed
		WHERE EXISTS (SELECT 1 FROM games WHERE id = :game_id)
		ON CONFLICT(game_id) DO UPDATE SET
			snapshot  = excluded.snapshot,
			since     = excluded.since,
			file      = excluded.file,
			chain     = excluded.chain,
			published = excluded.published,
			dismissed = excluded.dismissed`, h)
	if err != nil {
		return fmt.Errorf("set cloud head %s: %w", h.GameID, err)
	}
	return nil
}
