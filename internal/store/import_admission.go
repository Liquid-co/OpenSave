package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"
)

// ImportPathAdmission owns the native cross-process path claim throughout
// backup extraction and final game registration. It is not a token.
type ImportPathAdmission struct {
	store   *Store
	path    string
	gameID  string
	release func()
}

// ErrPathClaimBusy means admission timed out or another import holds the
// claim. It is not evidence that any save location overlaps another game.
var ErrPathClaimBusy = errors.New("save-location changes are busy; try again in a moment")

const pathClaimWait = 30 * time.Second
const pathClaimPoll = 100 * time.Millisecond

// Ordinary registrations wait for short-lived work, just as native topology
// edits do. The claim covers preparation AND commit, without holding a SQLite
// transaction while waiting or resolving filesystem paths.
func (s *Store) claimLock() (func(), error) {
	return s.claimLockContext(context.Background())
}

func (s *Store) claimLockContext(parent context.Context) (func(), error) {
	limit := s.claimWaitFor
	if limit <= 0 {
		limit = pathClaimWait
	}
	ctx, cancel := context.WithTimeout(parent, limit)
	defer cancel()
	return s.waitClaimLock(ctx)
}

func (s *Store) waitClaimLock(ctx context.Context) (func(), error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrPathClaimBusy, err)
		}
		release, err := s.tryClaimLock()
		if err == nil {
			// Cancellation racing acquisition must not admit a new mutation.
			if err := ctx.Err(); err != nil {
				release()
				return nil, fmt.Errorf("%w: %w", ErrPathClaimBusy, err)
			}
			return release, nil
		}
		if !errors.Is(err, ErrPathClaimBusy) {
			return nil, err
		}
		timer := time.NewTimer(pathClaimPoll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, fmt.Errorf("%w: %w", ErrPathClaimBusy, ctx.Err())
		case <-timer.C:
		}
	}
}

// Imports retain immediate refusal. Resolving symlinks makes aliases share the
// same sidecar on Unix; HOME and cwd never determine the lock location.
func (s *Store) tryClaimLock() (func(), error) {
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
	release, err := s.tryClaimLock()
	if err != nil {
		return nil, err
	}
	prepared, err := s.prepareNativePaths(path)
	if err != nil {
		release()
		return nil, err
	}
	tx, err := s.db.Beginx()
	if err != nil {
		release()
		return nil, err
	}
	if err := checkGamePathOwnershipTx(tx, prepared, incomingGameID); err != nil {
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
