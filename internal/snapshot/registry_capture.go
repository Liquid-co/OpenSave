package snapshot

import (
	"fmt"
	"path/filepath"

	"github.com/opensave/opensave/internal/store"
	"github.com/opensave/opensave/internal/winreg"
)

// RegistryDirName is the folder under the backups directory holding one
// materialised registry capture per game.
//
// Dot-prefixed so it can never be mistaken for a game id: the backups
// directory is <backups>/<gameId>/<branch>, and a game whose id happened to be
// "registry" would otherwise collide with it.
const RegistryDirName = ".registry"

// RegistryDir is where a game's materialised registry capture lives.
func RegistryDir(backupsDir, gameID string) string {
	return filepath.Join(backupsDir, RegistryDirName, gameID)
}

// refreshRegistryCapture rewrites a game's registry capture immediately before
// it is archived.
//
// The capture is a file inside one of the game's save locations, so the zip
// takes it along with everything else and no snapshot code needs to know about
// the registry. What it cannot do on its own is be current: the file holds
// whatever was captured last time, and the registry has moved since the game
// was played.
//
// Everything here is best-effort and logged. A registry key that cannot be
// read costs that key; failing the snapshot would cost the whole save, which
// is the trade ZipRoots already makes for an unreadable file.
func (m *Manager) refreshRegistryCapture(game store.Game, settings store.Settings, roots map[string]string) {
	keys, err := m.Store.GameRegistryKeys(game.ID)
	if err != nil || len(keys) == 0 {
		return
	}
	// The location has to be one the game actually has, or the capture would
	// be written somewhere the archive never looks.
	dir, mapped := roots[winreg.LocationName]
	if !mapped || dir == "" {
		dir = RegistryDir(settings.BackupsDir, game.ID)
	}
	warnings, capErr := winreg.CaptureToDir(keys, dir)
	if capErr != nil && m.Log != nil {
		m.Log("warn", fmt.Sprintf(
			"could not capture the registry saves for %q: %v — this snapshot holds its files only",
			game.Name, capErr))
	}
	for _, w := range warnings {
		if m.Log != nil {
			m.Log("warn", fmt.Sprintf("%s: %s", game.Name, w))
		}
	}
}

// RestoreRegistryCapture writes a game's captured registry values back, after
// its files have been restored.
//
// Returns the warnings a caller should show. A snapshot taken before registry
// support, or of a game with no keys, has nothing to put back and returns
// none.
func RestoreRegistryCapture(gameName, dir string) []string {
	warnings, err := winreg.RestoreFromDir(dir)
	if err != nil {
		return []string{fmt.Sprintf(
			"the registry saves for %q could not be restored: %v — its files were restored", gameName, err)}
	}
	return warnings
}
