package store

import (
	"path/filepath"
	"testing"
)

// A database from the fixes.9 test build already holds peer_links, made there
// as 0038, and has never run 0039. It must still open: a plain CREATE TABLE
// stopped the daemon with "table already exists" (as #118's review found for
// its tables).
func TestPeerLinksMigrationOpensOverTheTestBuildsTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "opensave.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	// As the test build left it: the table there, 0039 not recorded.
	if _, err := s.db.Exec(`DELETE FROM schema_migrations WHERE name = '0039_peer_links.sql'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO peer_links (peer_id, bytes_per_sec) VALUES ('node_x', 1234)`); err != nil {
		t.Fatal(err)
	}
	s.Close()

	s, err = Open(path)
	if err != nil {
		t.Fatalf("a database holding the test build's peer_links did not open: %v", err)
	}
	defer s.Close()
	var bps float64
	if err := s.db.Get(&bps, `SELECT bytes_per_sec FROM peer_links WHERE peer_id = 'node_x'`); err != nil || bps != 1234 {
		t.Errorf("what the test build measured is gone: %v %v", bps, err)
	}
}
