package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Provisioning hold: a game that has been recorded but is not ready to sync.
//
// The row is written in the same transaction as the game, so a crash cannot
// leave a tracked game that the next launch will sync. Existing games have
// no row and are left alone. See migrations/0038_game_provisioning_holds.sql.

// SetProvisioningReadFault makes later hold lookups fail with err while
// every other table still answers. Pass nil to clear it. Production never
// sets this; it exists so a test can prove a failed read is not "not held".
func (s *Store) SetProvisioningReadFault(err error) {
	if s == nil {
		return
	}
	s.faultMu.Lock()
	s.provisioningReadFault = err
	s.faultMu.Unlock()
}

func (s *Store) provisioningFault() error {
	if s == nil {
		return fmt.Errorf("provisioning hold could not be read")
	}
	s.faultMu.Lock()
	defer s.faultMu.Unlock()
	return s.provisioningReadFault
}

// ProvisioningBlocks reports whether gameID must not sync. An unknown id is
// not held. An alias of a held game is held. A failed read is an error, never
// a quiet false: callers must not serve, watch, or sync when this fails.
func (s *Store) ProvisioningBlocks(gameID string) (bool, error) {
	if s == nil {
		return false, fmt.Errorf("provisioning hold could not be read")
	}
	if gameID == "" {
		return false, nil
	}
	if err := s.provisioningFault(); err != nil {
		return false, err
	}
	held, err := s.ProvisioningHeld(gameID)
	if err != nil || held {
		return held, err
	}
	var canonical string
	err = s.db.Get(&canonical, `SELECT game_id FROM game_aliases WHERE alias_id = ?`, gameID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("provisioning hold alias %s: %w", gameID, err)
	}
	if canonical == "" || canonical == gameID {
		return false, nil
	}
	return s.ProvisioningHeld(canonical)
}

// ProvisioningHeld reports whether gameID is still being configured.
// An unknown id is not held. A database error is returned, not treated as false.
func (s *Store) ProvisioningHeld(gameID string) (bool, error) {
	if s == nil {
		return false, fmt.Errorf("provisioning hold could not be read")
	}
	if gameID == "" {
		return false, nil
	}
	if err := s.provisioningFault(); err != nil {
		return false, err
	}
	var one int
	err := s.db.Get(&one, `SELECT 1 FROM game_provisioning_holds WHERE game_id = ?`, gameID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("provisioning hold %s: %w", gameID, err)
	}
	return true, nil
}

// ProvisioningHeldSet is every game currently held, for callers that walk
// the library and must not query once per game.
func (s *Store) ProvisioningHeldSet() (map[string]struct{}, error) {
	if s == nil {
		return nil, fmt.Errorf("provisioning hold could not be read")
	}
	if err := s.provisioningFault(); err != nil {
		return nil, err
	}
	var ids []string
	if err := s.db.Select(&ids, `SELECT game_id FROM game_provisioning_holds`); err != nil {
		return nil, fmt.Errorf("list provisioning holds: %w", err)
	}
	out := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		out[id] = struct{}{}
	}
	return out, nil
}

// CreateHeldGame inserts a game and its provisioning hold in one transaction.
// AutoSync is forced off. The local untrack tombstone is cleared in the same
// transaction: re-tracking is explicit, but it must not become visible to a
// peer before the hold exists.
func (s *Store) CreateHeldGame(g Game) error {
	if g.ID == "" {
		return fmt.Errorf("held game requires an id")
	}
	if g.ActiveBranch == "" {
		g.ActiveBranch = "main"
	}
	if g.MaxSnapshots == 0 {
		g.MaxSnapshots = 20
	}
	g.AutoSync = false

	tx, err := s.db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.NamedExec(`
		INSERT INTO games (id, name, save_path, active_branch, auto_sync, max_snapshots, max_manual_snapshots, app_id, exe_path, cover_url)
		VALUES (:id, :name, :save_path, :active_branch, :auto_sync, :max_snapshots, :max_manual_snapshots, :app_id, :exe_path, :cover_url)`,
		g); err != nil {
		return fmt.Errorf("insert game: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO branches (game_id, name) VALUES (?, ?)`, g.ID, g.ActiveBranch); err != nil {
		return fmt.Errorf("insert default branch: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO game_provisioning_holds (game_id, held_at) VALUES (?, ?)`,
		g.ID, time.Now().UTC().Format(time.RFC3339Nano),
	); err != nil {
		return fmt.Errorf("insert provisioning hold: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM untracked_games WHERE game_id = ?`, g.ID); err != nil {
		return fmt.Errorf("clear tombstone: %w", err)
	}
	return tx.Commit()
}

// ReleaseProvisioning clears the hold and turns AutoSync on, in one
// transaction. A game that is not held is left unchanged, including a game
// whose AutoSync the user turned off on purpose. The bool is whether this
// call released a hold.
func (s *Store) ReleaseProvisioning(gameID string) (bool, error) {
	return s.ReleaseProvisioningMode(gameID, true)
}

// ReleaseProvisioningMode clears the hold. enableAutoSync false leaves
// AutoSync off, so reconcile, reconnect and the watcher do not sync the game.
// The caller then syncs it explicitly. A game that is not held is left
// unchanged either way.
func (s *Store) ReleaseProvisioningMode(gameID string, enableAutoSync bool) (bool, error) {
	tx, err := s.db.Beginx()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	res, err := tx.Exec(`DELETE FROM game_provisioning_holds WHERE game_id = ?`, gameID)
	if err != nil {
		return false, fmt.Errorf("release provisioning hold %s: %w", gameID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if n == 0 {
		return false, tx.Commit()
	}
	auto := 0
	if enableAutoSync {
		auto = 1
	}
	if _, err := tx.Exec(`UPDATE games SET auto_sync = ? WHERE id = ?`, auto, gameID); err != nil {
		return false, fmt.Errorf("set auto-sync %s: %w", gameID, err)
	}
	return true, tx.Commit()
}
