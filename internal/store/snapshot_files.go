package store

import (
	"fmt"

	"github.com/jmoiron/sqlx"
)

// CapturedFile is one file inside a snapshot: which save location it belongs
// to, its path within that location, and the hash of its contents.
//
// Root is empty for the game's primary save folder, matching delta.PrimaryRoot,
// so a game with a single folder reads exactly as it would have before extra
// locations existed.
type CapturedFile struct {
	Root string `db:"root"`
	Path string `db:"path"`
	Hash string `db:"hash"`
}

// RecordSnapshotFiles writes what a snapshot captured, one row per file.
//
// Written once, when the snapshot is created, and never updated afterwards.
// That is deliberate and is the whole difference between this and the
// whole-save hash it replaces: a snapshot is a fact about a moment, so nothing
// that happens later can make this description of it wrong. A value describing
// "the current state" is wrong the instant any code path forgets to move it,
// which is how a save with one edited file came to read as entirely
// uncaptured.
//
// A snapshot with no files is not an error — an empty save folder is a real
// thing, and is already reported elsewhere.
func (s *Store) RecordSnapshotFiles(snapshotID string, files []CapturedFile) error {
	if len(files) == 0 {
		return nil
	}
	tx, err := s.db.Beginx()
	if err != nil {
		return fmt.Errorf("record snapshot files for %s: %w", snapshotID, err)
	}
	defer tx.Rollback()

	// Prepared once: a save can hold thousands of files and this runs while
	// somebody is waiting for a snapshot to finish.
	stmt, err := tx.Preparex(
		`INSERT OR REPLACE INTO snapshot_files (snapshot_id, root, path, hash) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("record snapshot files for %s: %w", snapshotID, err)
	}
	defer stmt.Close()

	for _, f := range files {
		if _, err := stmt.Exec(snapshotID, f.Root, f.Path, f.Hash); err != nil {
			return fmt.Errorf("record snapshot file %q for %s: %w", f.Path, snapshotID, err)
		}
	}
	return tx.Commit()
}

// IsContentCaptured reports whether some snapshot of this game holds exactly
// this content, for exactly this path, in exactly this location.
//
// False means "cannot be shown to be captured", NOT "is not captured".
// Snapshots taken before these records existed have no rows and answer false
// for everything they hold. Callers must therefore treat false as the cautious
// answer and protect the file anyway: doing so needlessly costs a snapshot,
// while reading false as proof of absence would skip protecting a file that
// was never safe, and that is how saves are lost.
func (s *Store) IsContentCaptured(gameID, root, path, hash string) (bool, error) {
	if gameID == "" || path == "" || hash == "" {
		return false, nil
	}
	var n int
	err := s.db.Get(&n, `
		SELECT COUNT(*) FROM (
			SELECT 1 FROM snapshot_files sf
			JOIN snapshots s ON s.id = sf.snapshot_id
			WHERE s.game_id = ? AND sf.root = ? AND sf.path = ? AND sf.hash = ?
			LIMIT 1
		)`, gameID, root, path, hash)
	if err != nil {
		return false, fmt.Errorf("look up captured content for %s/%s: %w", gameID, path, err)
	}
	return n > 0, nil
}

// SnapshotFiles returns what a single snapshot captured. Empty for a snapshot
// taken before these records existed, which is not distinguishable from — and
// deliberately treated the same as — a snapshot of nothing.
func (s *Store) SnapshotFiles(snapshotID string) ([]CapturedFile, error) {
	var rows []CapturedFile
	if err := s.db.Select(&rows,
		`SELECT root, path, hash FROM snapshot_files WHERE snapshot_id = ? ORDER BY root, path`,
		snapshotID); err != nil {
		return nil, fmt.Errorf("list snapshot files for %s: %w", snapshotID, err)
	}
	return rows, nil
}

// SnapshotFileCount is how many files a snapshot captured in one save location.
type SnapshotFileCount struct {
	SnapshotID string `db:"snapshot_id"`
	Files      int    `db:"files"`
}

// SnapshotFileCounts lists a game's snapshots that captured at least minFiles
// files in one save location, with their counts, newest first. Only counts:
// a save can hold hundreds of thousands of files, and a caller looking for one
// snapshot reads them one at a time (SnapshotRootFiles) rather than all at once.
func (s *Store) SnapshotFileCounts(gameID, root string, minFiles int) ([]SnapshotFileCount, error) {
	var rows []SnapshotFileCount
	if err := s.db.Select(&rows, `
		SELECT sf.snapshot_id, COUNT(*) AS files FROM snapshot_files sf
		JOIN snapshots s ON s.id = sf.snapshot_id
		WHERE s.game_id = ? AND sf.root = ?
		GROUP BY sf.snapshot_id
		HAVING COUNT(*) >= ?
		ORDER BY s.timestamp DESC, s.id DESC`, gameID, root, minFiles); err != nil {
		return nil, fmt.Errorf("count snapshot files of %s: %w", gameID, err)
	}
	return rows, nil
}

// SnapshotRootFiles returns what one snapshot captured in one save location:
// path to content hash.
func (s *Store) SnapshotRootFiles(snapshotID, root string) (map[string]string, error) {
	var rows []CapturedFile
	if err := s.db.Select(&rows,
		`SELECT root, path, hash FROM snapshot_files WHERE snapshot_id = ? AND root = ?`,
		snapshotID, root); err != nil {
		return nil, fmt.Errorf("list snapshot files for %s: %w", snapshotID, err)
	}
	files := make(map[string]string, len(rows))
	for _, r := range rows {
		files[r.Path] = r.Hash
	}
	return files, nil
}

// AllSnapshotsOfGame lists every snapshot of a game, on every branch, oldest
// first.
func (s *Store) AllSnapshotsOfGame(gameID string) ([]Snapshot, error) {
	var out []Snapshot
	if err := s.db.Select(&out, `SELECT * FROM snapshots WHERE game_id = ? ORDER BY timestamp`, gameID); err != nil {
		return nil, fmt.Errorf("list snapshots of %s: %w", gameID, err)
	}
	return out, nil
}

// SnapshotHasFiles reports whether a snapshot's file list is recorded:
// snapshots taken before these records existed have none.
func (s *Store) SnapshotHasFiles(snapshotID string) bool {
	var n int
	err := s.db.Get(&n, `SELECT EXISTS (SELECT 1 FROM snapshot_files WHERE snapshot_id = ?)`, snapshotID)
	return err == nil && n == 1
}

// SnapshotPathHashes returns the content hashes a snapshot recorded for the
// given paths of one save location; a path it did not hold is absent.
func (s *Store) SnapshotPathHashes(snapshotID, root string, paths []string) (map[string]string, error) {
	out := map[string]string{}
	if len(paths) == 0 {
		return out, nil
	}
	query, args, err := sqlx.In(`SELECT path, hash FROM snapshot_files WHERE snapshot_id = ? AND root = ? AND path IN (?)`,
		snapshotID, root, paths)
	if err != nil {
		return nil, err
	}
	var rows []CapturedFile
	if err := s.db.Select(&rows, s.db.Rebind(query), args...); err != nil {
		return nil, fmt.Errorf("read snapshot files of %s: %w", snapshotID, err)
	}
	for _, r := range rows {
		out[r.Path] = r.Hash
	}
	return out, nil
}

// SnapshotsDiffer reports whether two snapshots differ in one save location,
// leaving out the paths in except: a file one holds that the other does not,
// at the same content. Answered by the database, which stops at the first
// difference — a save of a quarter-million files is never read into memory
// to compare two snapshots of it.
func (s *Store) SnapshotsDiffer(a, b, root string, except []string) (bool, error) {
	except = append(append([]string{}, except...), "") // never empty for IN
	query, args, err := sqlx.In(`SELECT EXISTS (
		SELECT 1 FROM snapshot_files x WHERE x.snapshot_id = ? AND x.root = ? AND x.path NOT IN (?)
		AND NOT EXISTS (SELECT 1 FROM snapshot_files y
			WHERE y.snapshot_id = ? AND y.root = x.root AND y.path = x.path AND y.hash = x.hash)
	) OR EXISTS (
		SELECT 1 FROM snapshot_files x WHERE x.snapshot_id = ? AND x.root = ? AND x.path NOT IN (?)
		AND NOT EXISTS (SELECT 1 FROM snapshot_files y
			WHERE y.snapshot_id = ? AND y.root = x.root AND y.path = x.path AND y.hash = x.hash)
	)`, a, root, except, b, b, root, except, a)
	if err != nil {
		return false, err
	}
	var differ bool
	if err := s.db.Get(&differ, s.db.Rebind(query), args...); err != nil {
		return false, fmt.Errorf("compare snapshots %s and %s: %w", a, b, err)
	}
	return differ, nil
}
