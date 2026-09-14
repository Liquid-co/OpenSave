// Package config resolves OpenSave's on-disk locations and performs the
// one-time ".savesync" -> ".opensave" home directory migration that the
// original JS app performed on every startup.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

const (
	oldHomeDirName = ".savesync"
	homeDirName    = ".opensave"

	// LegacyDBFileName is the JSON database file written by the original
	// Node.js daemon (src/daemon/db.js).
	LegacyDBFileName = "opensave-db.json"

	// SQLiteFileName is the new embedded database file.
	SQLiteFileName = "opensave.db"

	backupsDirName = "backups"
)

// Paths holds every filesystem location OpenSave needs, resolved once at
// startup relative to the user's home directory.
type Paths struct {
	HomeDir      string
	LegacyDB     string
	SQLiteDB     string
	BackupsDir   string
	MigrationLog string
	AppCacheFile string // Steam AppID->name cache
}

// Resolve migrates the legacy ~/.savesync directory to ~/.opensave if needed
// (mirrors db.js's startup check: rename if old exists and new does not),
// ensures the home directory exists, and returns the resolved Paths.
func Resolve() (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, fmt.Errorf("resolve home dir: %w", err)
	}
	return resolveWithin(home)
}

// ResolveAt uses dataDir directly as the OpenSave home directory (tests
// and portable installs) — no legacy .savesync migration is attempted.
func ResolveAt(dataDir string) (Paths, error) {
	return buildPaths(dataDir)
}

func resolveWithin(home string) (Paths, error) {
	oldHome := filepath.Join(home, oldHomeDirName)
	newHome := filepath.Join(home, homeDirName)

	if dirExists(oldHome) && !dirExists(newHome) {
		if err := os.Rename(oldHome, newHome); err != nil {
			return Paths{}, fmt.Errorf("migrate %s to %s: %w", oldHome, newHome, err)
		}
		// The old JS app also renamed a stray savesync-db.json inside the
		// folder if the outer rename above hadn't already moved it.
		oldDBFile := filepath.Join(newHome, "savesync-db.json")
		newDBFile := filepath.Join(newHome, LegacyDBFileName)
		if fileExists(oldDBFile) && !fileExists(newDBFile) {
			_ = os.Rename(oldDBFile, newDBFile)
		}
	}

	return buildPaths(newHome)
}

func buildPaths(homeDir string) (Paths, error) {
	// Private to this user. The database in here holds the device's private
	// key — the one that decrypts every save sent to it and proves who it is
	// to every peer — plus the Google tokens and the vault keys. With the
	// usual umask, 0o777 became a world-readable directory and SQLite's
	// default made a world-readable file inside it, so on a shared Linux or
	// macOS machine any other account could read all of that. Windows was
	// never exposed: the profile folder carries its own per-user ACL.
	//
	// 0o700 is what ssh and gpg use for the same reason, and an existing
	// directory is tightened too, because the ones already out there are
	// the ones that matter.
	if err := os.MkdirAll(homeDir, 0o700); err != nil {
		return Paths{}, fmt.Errorf("create home dir: %w", err)
	}
	tightenPermissions(homeDir)
	backupsDir := filepath.Join(homeDir, backupsDirName)
	if err := os.MkdirAll(backupsDir, 0o700); err != nil {
		return Paths{}, fmt.Errorf("create backups dir: %w", err)
	}

	return Paths{
		HomeDir:      homeDir,
		LegacyDB:     filepath.Join(homeDir, LegacyDBFileName),
		SQLiteDB:     filepath.Join(homeDir, SQLiteFileName),
		BackupsDir:   backupsDir,
		MigrationLog: filepath.Join(homeDir, "migration.log"),
		AppCacheFile: filepath.Join(homeDir, "steam-app-cache.json"),
	}, nil
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// tightenPermissions makes a directory private to its owner if it is not
// already, on the platforms where mode bits are the access control.
//
// Best-effort: a failure here is not worth refusing to start over, and on
// Windows the call is a no-op in all but name — the profile folder's ACL is
// what protects the data there.
func tightenPermissions(dir string) {
	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Stat(dir)
	if err != nil {
		return
	}
	if info.Mode().Perm()&0o077 != 0 {
		_ = os.Chmod(dir, 0o700)
	}
}
