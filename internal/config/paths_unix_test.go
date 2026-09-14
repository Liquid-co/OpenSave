//go:build !windows

package config

import (
	"os"
	"path/filepath"
	"testing"
)

// The home directory holds the device's private key, the Google tokens and
// the vault keys, and used to be created world-readable. On a shared machine
// that gave every other account the key that decrypts every save sent here.
func TestHomeDirIsPrivateToTheOwner(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "opensave-home")

	if _, err := buildPaths(home); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{home, filepath.Join(home, backupsDirName)} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm&0o077 != 0 {
			t.Errorf("%s is %o; any other user on this machine can read the private key in it", dir, perm)
		}
	}
}

// And a directory created by an earlier version is tightened on the next
// start — those are the installs that are actually exposed.
func TestExistingWorldReadableHomeIsTightened(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "old-home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(home, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := buildPaths(home); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(home)
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		t.Errorf("an existing %o home directory was left at %o", 0o755, perm)
	}
}
