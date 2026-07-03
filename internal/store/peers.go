package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// Peer is a paired remote device.
type Peer struct {
	ID          string         `db:"id" json:"id"`
	Name        string         `db:"name" json:"name"`
	DeviceType  string         `db:"device_type" json:"deviceType"`
	Address     string         `db:"address" json:"address"`
	Port        int            `db:"port" json:"port"`
	PairedAt    string         `db:"paired_at" json:"pairedAt"`
	LastSynced  sql.NullString `db:"last_synced" json:"lastSynced"`
	LastSeenMs  int64          `db:"last_seen_ms" json:"-"`
	Status      string         `db:"status" json:"status"`
}

// UpsertPeer inserts a new paired peer or updates an existing one's
// connection details (used both at pairing time and whenever a peer's
// address/status changes, e.g. LAN IP changed or WAN heartbeat received).
func (s *Store) UpsertPeer(p Peer) error {
	_, err := s.db.NamedExec(`
		INSERT INTO peers (id, name, device_type, address, port, status, last_seen_ms)
		VALUES (:id, :name, :device_type, :address, :port, :status, :last_seen_ms)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			device_type = excluded.device_type,
			address = excluded.address,
			port = excluded.port,
			status = excluded.status,
			last_seen_ms = excluded.last_seen_ms`,
		p)
	if err != nil {
		return fmt.Errorf("upsert peer %s: %w", p.ID, err)
	}
	return nil
}

// GetPeer returns a single paired peer by ID.
func (s *Store) GetPeer(id string) (Peer, error) {
	var p Peer
	err := s.db.Get(&p, `SELECT * FROM peers WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Peer{}, ErrNotFound
	}
	if err != nil {
		return Peer{}, fmt.Errorf("get peer %s: %w", id, err)
	}
	return p, nil
}

// ListPeers returns every paired peer.
func (s *Store) ListPeers() ([]Peer, error) {
	var peers []Peer
	if err := s.db.Select(&peers, `SELECT * FROM peers ORDER BY name`); err != nil {
		return nil, fmt.Errorf("list peers: %w", err)
	}
	return peers, nil
}

// SetPeerPairedAt overrides a peer's paired_at timestamp (used by the
// legacy importer to carry over the original pairing date instead of the
// insert-time default).
func (s *Store) SetPeerPairedAt(id, timestampISO8601 string) error {
	res, err := s.db.Exec(`UPDATE peers SET paired_at = ? WHERE id = ?`, timestampISO8601, id)
	if err != nil {
		return fmt.Errorf("set peer paired_at %s: %w", id, err)
	}
	return checkRowAffected(res)
}

// UpdatePeerLastSynced stamps a peer's last_synced time (called after a
// successful sync run completes with that peer).
func (s *Store) UpdatePeerLastSynced(id, timestampISO8601 string) error {
	res, err := s.db.Exec(`UPDATE peers SET last_synced = ? WHERE id = ?`, timestampISO8601, id)
	if err != nil {
		return fmt.Errorf("update peer last_synced %s: %w", id, err)
	}
	return checkRowAffected(res)
}

// UnpairPeer removes a paired peer and its per-game sync lineage state.
func (s *Store) UnpairPeer(id string) error {
	tx, err := s.db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM game_peer_sync_state WHERE peer_id = ?`, id); err != nil {
		return fmt.Errorf("delete sync state for peer %s: %w", id, err)
	}
	res, err := tx.Exec(`DELETE FROM peers WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete peer %s: %w", id, err)
	}
	if err := checkRowAffected(res); err != nil {
		return err
	}
	return tx.Commit()
}

// GamePeerSyncState is the lineage bookkeeping used by the P2P sync engine
// to compute conflict/direction without relying on wall-clock comparisons:
// the set of relative file/dir paths that were part of the last
// successful sync with a given peer for a given game.
type GamePeerSyncState struct {
	GameID          string `db:"game_id"`
	PeerID          string `db:"peer_id"`
	LastSyncedFiles string `db:"last_synced_files"`
	LastSyncedDirs  string `db:"last_synced_dirs"`
}

// GetSyncState returns the last-synced file/dir path sets for a game+peer,
// or empty sets if this is the first sync ever attempted with that peer.
func (s *Store) GetSyncState(gameID, peerID string) (files, dirs []string, err error) {
	var row GamePeerSyncState
	dbErr := s.db.Get(&row, `SELECT * FROM game_peer_sync_state WHERE game_id = ? AND peer_id = ?`, gameID, peerID)
	if errors.Is(dbErr, sql.ErrNoRows) {
		return []string{}, []string{}, nil
	}
	if dbErr != nil {
		return nil, nil, fmt.Errorf("get sync state %s/%s: %w", gameID, peerID, dbErr)
	}
	if err := json.Unmarshal([]byte(row.LastSyncedFiles), &files); err != nil {
		return nil, nil, fmt.Errorf("unmarshal last_synced_files: %w", err)
	}
	if err := json.Unmarshal([]byte(row.LastSyncedDirs), &dirs); err != nil {
		return nil, nil, fmt.Errorf("unmarshal last_synced_dirs: %w", err)
	}
	return files, dirs, nil
}

// SetSyncState replaces the lineage bookkeeping for a game+peer wholesale
// after a successful sync — matches the JS app's Set-replace-in-full
// approach rather than incremental row-level tracking.
func (s *Store) SetSyncState(gameID, peerID string, files, dirs []string) error {
	if files == nil {
		files = []string{}
	}
	if dirs == nil {
		dirs = []string{}
	}
	filesJSON, err := json.Marshal(files)
	if err != nil {
		return err
	}
	dirsJSON, err := json.Marshal(dirs)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
		INSERT INTO game_peer_sync_state (game_id, peer_id, last_synced_files, last_synced_dirs)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(game_id, peer_id) DO UPDATE SET
			last_synced_files = excluded.last_synced_files,
			last_synced_dirs = excluded.last_synced_dirs`,
		gameID, peerID, string(filesJSON), string(dirsJSON))
	if err != nil {
		return fmt.Errorf("set sync state %s/%s: %w", gameID, peerID, err)
	}
	return nil
}
