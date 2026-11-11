package store

import (
	"path/filepath"
	"testing"
)

// A database from a build of the earlier draft of this change already holds
// settings_files, settings_observations and marks, made under other migration
// numbers, and has never run 0042. It must still open: a plain CREATE TABLE
// stopped the daemon with "table already exists", as sync_device_settings
// once did (asked in review on #118).
func TestSettingsFilesMigrationOpensOverTheDraftsTables(t *testing.T) {
	path := filepath.Join(t.TempDir(), "opensave.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	// As the draft's build left it: the tables there, 0042 not recorded.
	if _, err := s.db.Exec(`DELETE FROM schema_migrations WHERE name = '0042_settings_files.sql'`); err != nil {
		t.Fatal(err)
	}
	if err := s.SetMark("kept", "from the draft's build"); err != nil {
		t.Fatal(err)
	}
	s.Close()

	s, err = Open(path)
	if err != nil {
		t.Fatalf("a database holding the draft's tables did not open: %v", err)
	}
	defer s.Close()
	if got := s.Mark("kept"); got != "from the draft's build" {
		t.Errorf("what the draft's build recorded is gone: %q", got)
	}
}
