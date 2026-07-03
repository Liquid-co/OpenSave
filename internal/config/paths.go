// Package config resolves OpenSave's on-disk locations and performs the
// one-time ".savesync" -> ".opensave" home directory migration that the
// original JS app performed on every startup.
package config

import (
	"fmt"
	"os"
	"path/filepath"
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
	HomeDir     string
	LegacyDB    string
	SQLiteDB    string
	BackupsDir  string
	MigrationLog string
}

// Resolve migrates the legacy ~/.savesync directory to ~/.opensave if needed
// (mirrors db.js's startup check: rename if old exists and new does not),
// ensures the home directory exists, and returns the resolved Paths.
func Resolve() (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, fmt.Errorf("resolve home dir: %w", err)
	}

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

	if err := os.MkdirAll(newHome, 0o777); err != nil {
		return Paths{}, fmt.Errorf("create home dir: %w", err)
	}
	backupsDir := filepath.Join(newHome, backupsDirName)
	if err := os.MkdirAll(backupsDir, 0o777); err != nil {
		return Paths{}, fmt.Errorf("create backups dir: %w", err)
	}

	return Paths{
		HomeDir:      newHome,
		LegacyDB:     filepath.Join(newHome, LegacyDBFileName),
		SQLiteDB:     filepath.Join(newHome, SQLiteFileName),
		BackupsDir:   backupsDir,
		MigrationLog: filepath.Join(newHome, "migration.log"),
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
