package store

import (
	"path/filepath"
	"testing"
)

// A database from a build that made the folder-trust table as migration 0038
// opens, keeping what it holds, now that the table is migration 0044.
func TestFolderTrustTableFromAnEarlierBuildOpens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "opensave.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO peer_folder_trust (peer_id, since_ms) VALUES ('peer-1', 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE schema_migrations SET name = '0038_peer_folder_trust.sql' WHERE name = '0044_peer_folder_trust.sql'`); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	s, err = Open(path)
	if err != nil {
		t.Fatalf("a database from such a build no longer opens: %v", err)
	}
	defer s.Close()
	if !s.PeerChoosesFolders("peer-1") {
		t.Error("the trust it held was lost")
	}
}
