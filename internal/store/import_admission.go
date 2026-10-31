package store

import (
	"fmt"
	"path/filepath"
)

// ImportPathAdmission owns the native cross-process path claim throughout
// backup extraction and final game registration. It is not a token.
type ImportPathAdmission struct {
	store   *Store
	path    string
	gameID  string
	release func()
}

// claimLock protects native registrations of primary/named save roots and
// untracked backup extraction. The lock is the database file itself, so a
// different HOME, cwd, or hardlink spelling still contends.
func (s *Store) claimLock() (func(), error) {
	canonical, err := filepath.Abs(s.path)
	if err != nil {
		return nil, err
	}
	canonical, err = filepath.EvalSymlinks(canonical)
	if err != nil {
		return nil, err
	}
	release, err := claimNativeDB(canonical)
	if err != nil {
		return nil, fmt.Errorf("acquire save-path claim lock: %w", err)
	}
	return release, nil
}

// BeginImportPath reserves ownership through every filesystem effect and the
// eventual game registration. A read failure always refuses admission.
func (s *Store) BeginImportPath(path, incomingGameID string) (*ImportPathAdmission, error) {
	release, err := s.claimLock()
	if err != nil {
		return nil, err
	}
	tx, err := s.db.Beginx()
	if err != nil {
		release()
		return nil, err
	}
	if err := checkGamePathOwnershipTx(tx, path, incomingGameID); err != nil {
		_ = tx.Rollback()
		release()
		return nil, err
	}
	var found int
	if err := tx.Get(&found, "SELECT COUNT(*) FROM games WHERE id=?", incomingGameID); err != nil || found != 0 {
		_ = tx.Rollback()
		release()
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("%w: imported game already exists", ErrGamePathOwned)
	}
	if err := tx.Rollback(); err != nil {
		release()
		return nil, err
	}
	return &ImportPathAdmission{store: s, path: path, gameID: incomingGameID, release: release}, nil
}

// CreateGameAdmitted skips the claim re-acquire only for the exact target
// and ID that were checked, but still rechecks #131 ownership in SQLite.
func (s *Store) CreateGameAdmitted(g Game, a *ImportPathAdmission) error {
	if a == nil || a.store != s || a.release == nil || a.path != g.SavePath || a.gameID != g.ID {
		return ErrGamePathOwned
	}
	return s.createGameWithoutClaimLock(g)
}

func (a *ImportPathAdmission) Release() {
	if a != nil && a.release != nil {
		a.release()
		a.release = nil
	}
}
