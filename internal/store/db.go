// Package store implements OpenSave's persistence layer: an embedded
// SQLite database (replacing the original single-JSON-file db.js) plus a
// one-time importer for existing users' legacy JSON data.
package store

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Store wraps the SQLite connection and exposes entity-scoped query
// methods (see settings.go, games.go, branches.go, snapshots.go, peers.go,
// cloudtokens.go).
type Store struct {
	db           *sqlx.DB
	claimWaitFor time.Duration // test-injectable admission bound; defaults to 30s
	path         string        // the database file, for EnableIncrementalVacuum's space check
}

// Open creates (if needed) and opens the SQLite database at path, applying
// any migrations that haven't run yet.
func Open(path string) (*Store, error) {
	db, err := openDB(path)
	if err != nil {
		return nil, err
	}

	s := &Store{db: db, path: path}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	// The file holds the device's private key, the Google tokens and the
	// vault keys. SQLite creates it with the process umask, which is usually
	// world-readable; the directory around it is private now, but the file
	// should be too, as an ssh key is — the directory is the first wall and
	// this is the second. Best-effort, and a no-op in practice on Windows.
	restrictToOwner(path)
	restrictToOwner(path + "-wal")
	restrictToOwner(path + "-shm")
	return s, nil
}

// openDB opens the SQLite connection Open builds a Store on.
func openDB(path string) (*sqlx.DB, error) {
	// _pragma params ensure foreign keys are enforced (SQLite defaults them
	// off per-connection) and busy_timeout avoids spurious SQLITE_BUSY
	// errors from the watcher/api/p2p goroutines all touching the DB.
	//
	// WAL: a write is appended to one log and the database file is updated
	// from it later, rather than every write creating, flushing and deleting a
	// rollback journal of its own — which made a quarter of a million small
	// writes take most of an hour. synchronous NORMAL is what WAL is meant to
	// run with: the database stays whole whatever happens; a power cut can
	// only lose the last moments' writes, never corrupt it.
	//
	// auto_vacuum INCREMENTAL comes first: it takes effect only while the
	// file is still empty, before WAL writes its header (see compact.go). On
	// an existing database it changes nothing.
	dsn := fmt.Sprintf("file:%s?_pragma=auto_vacuum(2)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)", path)
	db, err := sqlx.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	db.SetMaxOpenConns(1) // modernc.org/sqlite + a single file: avoid concurrent-writer lock contention

	// Tolerate columns the struct doesn't know about. Queries here use
	// SELECT *, so without this a database written by a NEWER build hard-fails
	// an older one ("missing destination name <col>") and the app won't start
	// at all — a downgrade, or just testing a beta and going back, bricks it.
	// Ignoring unmapped columns makes the schema forward-compatible instead.
	return db.Unsafe(), nil
}

// restrictToOwner sets a file to 0600 where mode bits are the access control.
// Missing files are fine (the WAL and SHM files exist only while in use).
func restrictToOwner(path string) {
	if runtime.GOOS == "windows" {
		return
	}
	if info, err := os.Stat(path); err == nil && info.Mode().Perm()&0o077 != 0 {
		_ = os.Chmod(path, 0o600)
	}
}

// Close releases the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// migration is one embedded schema step, applied in name order.
type migration struct {
	name string
	sql  string
}

func (s *Store) migrate() error {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("read embedded migrations: %w", err)
	}
	var migrations []migration
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		sqlBytes, err := migrationsFS.ReadFile("migrations/" + e.Name())
		if err != nil {
			return fmt.Errorf("read migration %s: %w", e.Name(), err)
		}
		migrations = append(migrations, migration{name: e.Name(), sql: string(sqlBytes)})
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].name < migrations[j].name })
	return s.applyMigrations(migrations)
}

// applyMigrations applies the migrations not yet recorded, all in one
// transaction.
//
// One, not one per migration: every commit flushes the journal and the
// database to disk, and a new database has every migration to apply. Committed
// one by one that was some 150 flushes before the daemon could start, which a
// busy Windows disk took over thirty seconds to do — past the time the CLI and
// the tests give a daemon to come up. In one transaction it is a single
// commit, and a failed step leaves the schema as it was rather than half
// upgraded.
func (s *Store) applyMigrations(migrations []migration) error {
	tx, err := s.db.Beginx()
	if err != nil {
		return fmt.Errorf("begin migrations: %w", err)
	}
	defer tx.Rollback() // a no-op once committed

	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY)`); err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}
	var names []string
	if err := tx.Select(&names, `SELECT name FROM schema_migrations`); err != nil {
		return fmt.Errorf("read applied migrations: %w", err)
	}
	applied := make(map[string]bool, len(names))
	for _, name := range names {
		applied[name] = true
	}

	for _, m := range migrations {
		if applied[m.name] {
			continue
		}
		if _, err := tx.Exec(m.sql); err != nil {
			return fmt.Errorf("apply migration %s: %w", m.name, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (name) VALUES (?)`, m.name); err != nil {
			return fmt.Errorf("record migration %s: %w", m.name, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migrations: %w", err)
	}
	return nil
}
