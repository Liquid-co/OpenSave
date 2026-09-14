//go:build !windows

package store

import (
	"os"
	"path/filepath"
	"testing"
)

// The database file itself must be private, not only the directory around
// it: it is the file that holds the private key.
func TestDatabaseFileIsPrivateToTheOwner(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "opensave.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		t.Errorf("the database is %o; other users could read the device key and tokens", perm)
	}
}
