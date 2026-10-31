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


// ErrOwnershipClaimsChanged means a claim appeared after path resolution.
// It is safe to retry the entire operation, but never to commit using stale
// preflight path identities.
var ErrOwnershipClaimsChanged = errors.New("save-location ownership changed during validation; retry")

// preparedNativePath contains only in-memory comparison data. Filesystem I/O
// (including ancestor symlink resolution and Stat) happens before Beginx.
type preparedNativePath struct {
	text string
	canonical string
	hasCanonical bool
	info os.FileInfo
}

type preparedNativePaths struct {
	target string
	values map[string]preparedNativePath
}

func prepareNativePath(path string) preparedNativePath {
	p := preparedNativePath{text: normalizeLocationPath(path)}
	if strings.TrimSpace(path) == "" {
		return p
	}
	if resolved, ok := canonicalExistingAncestor(path); ok {
		p.canonical = normalizeLocationPath(resolved)
		p.hasCanonical = true
	}
	if info, err := os.Stat(path); err == nil {
		p.info = info
	}
	return p
}

// prepareNativePathsFor intentionally does not hold a transaction or a
// database connection while resolving paths. The resolver argument allows a
// deterministic blocked-filesystem test without a package-global hook.
func (s *Store) prepareNativePathsFor(candidate string, resolver func(string) preparedNativePath) (*preparedNativePaths, error) {
	p := &preparedNativePaths{target: candidate, values: make(map[string]preparedNativePath)}
	if strings.TrimSpace(candidate) == "" {
		return p, nil
	}
	var paths []string
	if err := s.db.Select(&paths, `
		SELECT save_path AS path FROM games WHERE TRIM(save_path) <> ''
		UNION SELECT path FROM game_roots WHERE TRIM(path) <> ''`); err != nil {
		return nil, fmt.Errorf("prepare tracked save locations: %w", err)
	}
	paths = append(paths, candidate)
	for _, path := range paths {
		if _, present := p.values[path]; !present {
			p.values[path] = resolver(path)
		}
	}
	return p, nil
}

func (s *Store) prepareNativePaths(candidate string) (*preparedNativePaths, error) {
	return s.prepareNativePathsFor(candidate, prepareNativePath)
}

func preparedPathOverlaps(a, b string) bool {
	return a != "" && b != "" && (a == b || pathContains(a,b) || pathContains(b,a))
}

// shares compares solely precomputed data. Never call EvalSymlinks, Stat,
// os.Getwd or path resolution functions while a SQLite transaction is open.
func (p *preparedNativePaths) shares(a, b string) (bool, error) {
	if strings.TrimSpace(a) == "" || strings.TrimSpace(b) == "" {
		return false, nil
	}
	pa, okA := p.values[a]
	pb, okB := p.values[b]
	if !okA || !okB {
		return false, ErrOwnershipClaimsChanged
	}
	if preparedPathOverlaps(pa.text, pb.text) {
		return true, nil
	}
	if pa.hasCanonical && pb.hasCanonical && preparedPathOverlaps(pa.canonical, pb.canonical) {
		return true, nil
	}
	return pa.info != nil && pb.info != nil && os.SameFile(pa.info, pb.info), nil
}

// checkGamePathOwnershipTx is the final authoritative ownership check. It
// reads current claims inside the SAME SQLite transaction as the write but
// compares only paths resolved outside the transaction. A newly introduced
// raw path is not allowed to escape the check: it fails closed for a fresh
// preflight instead of being resolved while holding the database.
func checkGamePathOwnershipTx(tx *sqlx.Tx, prepared *preparedNativePaths, exceptGameID string) error {
	if prepared == nil {
		return ErrOwnershipClaimsChanged
	}
	path := prepared.target
	if strings.TrimSpace(path) == "" {
		return nil
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
		overlap, err := prepared.shares(path, claim.Path)
		if err != nil {
			return fmt.Errorf("inspect changed save-location owner: %w", err)
		}
		if overlap {
			return fmt.Errorf("%w: %q already owns %q (requested %q)",
				ErrGamePathOwned, claim.Name, claim.Path, path)
		}
	}
	return nil
}

// checkGameLocalRootsTx checks same-game roots using the same resolved
// identities. The authoritative SQL claims are still read inside the writer
// transaction; any claim not prepared before it fails closed.
func checkGameLocalRootsTx(tx *sqlx.Tx, prepared *preparedNativePaths, gameID, excludeRoot string) error {
	if prepared == nil {
		return ErrOwnershipClaimsChanged
	}
	path := prepared.target
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
		overlap, err := prepared.shares(path, root.Path)
		if err != nil {
			return fmt.Errorf("inspect changed named root: %w", err)
		}
		if overlap {
			return fmt.Errorf("%w: %q already maps named root %q at %q",
				ErrGamePathOwned, gameID, root.Name, root.Path)
		}
	}
	return nil
}
