package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jmoiron/sqlx"
)

// ErrGamePathOwned distinguishes a committed native ownership collision from
// a failed database inspection. It applies to ordinary and peer-owned games.
var ErrGamePathOwned = errors.New("save location overlaps an existing tracked game")

type nativePathClaim struct {
	GameID string `db:"game_id"`
	Name   string `db:"game_name"`
	Path   string `db:"path"`
}

// pathsShareFiles retains OpenSave's cross-platform textual path policy and
// also detects existing filesystem aliases (including nested symlink paths on
// Unix). Nonexistent remote folders still get the textual overlap check.
func pathsShareFiles(a, b string) bool {
	if strings.TrimSpace(a) == "" || strings.TrimSpace(b) == "" {
		return false
	}
	if PathsOverlap(a, b) {
		return true
	}
	// An unmapped peer root or prospective named root can have a missing
	// final leaf. Resolve the deepest EXISTING parent before comparing the
	// remaining literal child components, otherwise a parent symlink hides a
	// real overlap until its last directory is created.
	ra, ea := canonicalExistingAncestor(a)
	rb, eb := canonicalExistingAncestor(b)
	if ea && eb && PathsOverlap(ra, rb) {
		return true
	}
	ai, ae := os.Stat(a)
	bi, be := os.Stat(b)
	return ae == nil && be == nil && os.SameFile(ai, bi)
}

// canonicalExistingAncestor resolves existing parent aliases without creating
// anything. Only filesystem-absolute paths are eligible; Windows-style paths
// on a Unix host still use the independent cross-platform textual comparison.
// A dangling symlink or inaccessible ancestor yields no extra ownership proof.
func canonicalExistingAncestor(path string) (string, bool) {
	if !filepath.IsAbs(path) {
		return "", false
	}
	current := filepath.Clean(path)
	missing := make([]string, 0, 4)
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return filepath.Clean(resolved), true
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", false
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", false
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

// checkGamePathOwnershipTx is the authoritative decision for a new/relocated
// game primary or named root. It MUST run on the same SQLite transaction as
// the write. All native writers using it observe the same serialized SQLite
// ownership decision; a competing writer/SQLITE_BUSY must fail, not retry on
// an old preflight verdict.
//
// A valid game can own multiple disjoint paths. The excluded owner is checked
// separately by the caller when adding or moving one of its named roots.
func checkGamePathOwnershipTx(tx *sqlx.Tx, path, exceptGameID string) error {
	if strings.TrimSpace(path) == "" {
		return nil // peer-created game can start without a mapped local folder
	}
	var claims []nativePathClaim
	err := tx.Select(&claims, `
		SELECT id AS game_id, name AS game_name, save_path AS path
		  FROM games WHERE id <> ? AND TRIM(save_path) <> ''
		UNION ALL
		SELECT r.game_id AS game_id, g.name AS game_name, r.path AS path
		  FROM game_roots r JOIN games g ON g.id = r.game_id
		 WHERE r.game_id <> ? AND TRIM(r.path) <> ''`,
		exceptGameID, exceptGameID)
	if err != nil {
		return fmt.Errorf("inspect native save-location owners: %w", err)
	}
	for _, claim := range claims {
		if pathsShareFiles(path, claim.Path) {
			return fmt.Errorf("%w: %q already owns %q (requested %q)",
				ErrGamePathOwned, claim.Name, claim.Path, path)
		}
	}
	return nil
}

// checkGameLocalRootsTx rejects a new primary/root path overlapping another
// mapped location of the SAME game. excludeRoot is the root being remapped.
func checkGameLocalRootsTx(tx *sqlx.Tx, gameID, path, excludeRoot string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	var roots []struct {
		Name string `db:"name"`
		Path string `db:"path"`
	}
	if err := tx.Select(&roots, `SELECT name, path FROM game_roots
		 WHERE game_id = ? AND name <> ? AND TRIM(path) <> ''`,
		gameID, excludeRoot); err != nil {
		return fmt.Errorf("inspect existing named roots: %w", err)
	}
	for _, root := range roots {
		if pathsShareFiles(path, root.Path) {
			return fmt.Errorf("%w: %q already maps named root %q at %q",
				ErrGamePathOwned, gameID, root.Name, root.Path)
		}
	}
	return nil
}
