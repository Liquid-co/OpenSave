package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

// Deletion holds: a game whose save files all went at once on this device,
// held back from syncing until someone says whether that was meant. See
// internal/p2p/syncengine/hold.go for the rules; this is where they are kept.

// Hold states.
const (
	// HoldAsking waits for an answer; the game does not sync meanwhile.
	HoldAsking = "held"
	// HoldFetching is an answer of "put them back" while files are still to
	// come from the other devices: this device syncs to fetch them, and still
	// shows the other devices nothing, since its empty folder would read to
	// them as every file deleted.
	HoldFetching = "fetching"
	// HoldConfirmed is an answer of "delete them there too": the empty folder
	// syncs, and is not held again until it has had files once more.
	HoldConfirmed = "confirmed"
)

// DeletionHold is one game's hold.
type DeletionHold struct {
	GameID  string `db:"game_id" json:"gameId"`
	SinceMs int64  `db:"since_ms" json:"sinceMs"`
	State   string `db:"state" json:"state"`
	// Held is what the emptied locations held before, as JSON; Files decodes
	// it.
	Held string `db:"held" json:"-"`
}

// Files is what the emptied locations held before, by location ("" is the
// main save folder).
func (h DeletionHold) Files() map[string][]string {
	out := map[string][]string{}
	_ = json.Unmarshal([]byte(h.Held), &out)
	return out
}

// FileCount is how many files that is, in every location together.
func (h DeletionHold) FileCount() int {
	n := 0
	for _, files := range h.Files() {
		n += len(files)
	}
	return n
}

// LocationNames is the emptied locations, sorted; "" is the main folder.
func (h DeletionHold) LocationNames() []string {
	names := []string{}
	for name := range h.Files() {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// GetDeletionHold returns a game's hold; ok is false when it has none.
func (s *Store) GetDeletionHold(gameID string) (h DeletionHold, ok bool, err error) {
	err = s.db.Get(&h, `SELECT * FROM deletion_holds WHERE game_id = ?`, gameID)
	if errors.Is(err, sql.ErrNoRows) {
		return DeletionHold{}, false, nil
	}
	if err != nil {
		return DeletionHold{}, false, fmt.Errorf("get deletion hold %s: %w", gameID, err)
	}
	return h, true, nil
}

// SetDeletionHold records a game's hold, replacing any it had.
func (s *Store) SetDeletionHold(gameID, state string, sinceMs int64, held map[string][]string) error {
	if held == nil {
		held = map[string][]string{}
	}
	raw, err := json.Marshal(held)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO deletion_holds (game_id, since_ms, state, held) VALUES (?, ?, ?, ?)
		ON CONFLICT(game_id) DO UPDATE SET since_ms = excluded.since_ms, state = excluded.state, held = excluded.held`,
		gameID, sinceMs, state, string(raw))
	if err != nil {
		return fmt.Errorf("set deletion hold %s: %w", gameID, err)
	}
	return nil
}

// SetDeletionHoldState changes a game's hold's state and nothing else.
func (s *Store) SetDeletionHoldState(gameID, state string) error {
	if _, err := s.db.Exec(`UPDATE deletion_holds SET state = ? WHERE game_id = ?`, state, gameID); err != nil {
		return fmt.Errorf("set deletion hold state %s: %w", gameID, err)
	}
	return nil
}

// ClearDeletionHold removes a game's hold, whatever its state.
func (s *Store) ClearDeletionHold(gameID string) error {
	if _, err := s.db.Exec(`DELETE FROM deletion_holds WHERE game_id = ?`, gameID); err != nil {
		return fmt.Errorf("clear deletion hold %s: %w", gameID, err)
	}
	return nil
}

// ListDeletionHolds returns every game's hold.
func (s *Store) ListDeletionHolds() ([]DeletionHold, error) {
	var out []DeletionHold
	if err := s.db.Select(&out, `SELECT * FROM deletion_holds ORDER BY since_ms`); err != nil {
		return nil, fmt.Errorf("list deletion holds: %w", err)
	}
	return out, nil
}

// HasSyncState says whether a game has synced with any paired device: the
// only games an emptied folder can cost anything elsewhere.
func (s *Store) HasSyncState(gameID string) bool {
	var one int
	if s.db.Get(&one, `SELECT 1 FROM game_peer_sync_state WHERE game_id = ? LIMIT 1`, gameID) == nil {
		return true
	}
	return s.db.Get(&one, `SELECT 1 FROM game_root_sync_state WHERE game_id = ? LIMIT 1`, gameID) == nil
}

// SharedFiles is every file a game is recorded as having in common with any
// paired device, by save location ("" is the main folder): what an emptied
// location would delete on those devices if its emptiness were synced.
func (s *Store) SharedFiles(gameID string) (map[string]map[string]struct{}, error) {
	out := map[string]map[string]struct{}{}
	add := func(root, raw string) error {
		var files []string
		if err := json.Unmarshal([]byte(raw), &files); err != nil {
			return fmt.Errorf("unmarshal shared files of %s: %w", gameID, err)
		}
		for _, f := range files {
			if out[root] == nil {
				out[root] = map[string]struct{}{}
			}
			out[root][f] = struct{}{}
		}
		return nil
	}
	var primary []string
	if err := s.db.Select(&primary, `SELECT last_synced_files FROM game_peer_sync_state WHERE game_id = ?`, gameID); err != nil {
		return nil, fmt.Errorf("shared files of %s: %w", gameID, err)
	}
	for _, raw := range primary {
		if err := add("", raw); err != nil {
			return nil, err
		}
	}
	var roots []struct {
		Root  string `db:"root"`
		Files string `db:"last_synced_files"`
	}
	if err := s.db.Select(&roots, `SELECT root, last_synced_files FROM game_root_sync_state WHERE game_id = ?`, gameID); err != nil {
		return nil, fmt.Errorf("shared files of %s: %w", gameID, err)
	}
	for _, r := range roots {
		if err := add(r.Root, r.Files); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// ForgetSharedPaths takes files out of what a game is recorded as having in
// common with every paired device, in one save location, along with every
// recorded folder that is not in keepDirs. For "put them back" on an emptied
// save (syncengine.Engine.PutBack): a file recorded as shared and missing
// here reads as deleted here, and forgetting it turns the other device's
// copy into one to fetch.
func (s *Store) ForgetSharedPaths(gameID, root string, files []string, keepDirs map[string]struct{}) error {
	drop := map[string]bool{}
	for _, f := range files {
		drop[f] = true
	}
	trim := func(raw string, dirs bool) (string, error) {
		var list []string
		if err := json.Unmarshal([]byte(raw), &list); err != nil {
			return "", err
		}
		kept := make([]string, 0, len(list))
		for _, p := range list {
			if dirs {
				if _, ok := keepDirs[p]; !ok {
					continue
				}
			} else if drop[p] {
				continue
			}
			kept = append(kept, p)
		}
		out, err := json.Marshal(kept)
		return string(out), err
	}

	tx, err := s.db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	type row struct {
		PeerID string `db:"peer_id"`
		Files  string `db:"last_synced_files"`
		Dirs   string `db:"last_synced_dirs"`
	}
	var rows []row
	if root == "" {
		err = tx.Select(&rows, `SELECT peer_id, last_synced_files, last_synced_dirs FROM game_peer_sync_state WHERE game_id = ?`, gameID)
	} else {
		err = tx.Select(&rows, `SELECT peer_id, last_synced_files, last_synced_dirs FROM game_root_sync_state WHERE game_id = ? AND root = ?`, gameID, root)
	}
	if err != nil {
		return fmt.Errorf("forget shared paths of %s: %w", gameID, err)
	}
	for _, r := range rows {
		files, err := trim(r.Files, false)
		if err != nil {
			return err
		}
		dirs, err := trim(r.Dirs, true)
		if err != nil {
			return err
		}
		if root == "" {
			_, err = tx.Exec(`UPDATE game_peer_sync_state SET last_synced_files = ?, last_synced_dirs = ? WHERE game_id = ? AND peer_id = ?`,
				files, dirs, gameID, r.PeerID)
		} else {
			_, err = tx.Exec(`UPDATE game_root_sync_state SET last_synced_files = ?, last_synced_dirs = ? WHERE game_id = ? AND peer_id = ? AND root = ?`,
				files, dirs, gameID, r.PeerID, root)
		}
		if err != nil {
			return fmt.Errorf("forget shared paths of %s: %w", gameID, err)
		}
	}
	return tx.Commit()
}
