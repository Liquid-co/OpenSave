package daemon

import "github.com/opensave/opensave/internal/presets"

// refreshDeviceSettings reads the game database again when a newer index of it
// has been put in place since it was last read, and carries every game's
// last-snapshot hash over to what the game is excluded by now, as at start.
//
// The first read comes while the daemon starts, and does not wait for an index
// due a rebuild. On a fresh install that is the database built into the app,
// which names no settings at all: every settings file synced to the other
// devices until OpenSave was restarted. The weekly catalogue update, likewise,
// took effect only at the next start. Called from the minute ticker.
func (d *Daemon) refreshDeviceSettings() {
	if d.Scanner == nil {
		return
	}
	gen := presets.CatalogueGeneration()
	d.devMu.Lock()
	seen := d.devGen
	d.devMu.Unlock()
	if gen == seen {
		return
	}
	// Read before it is swapped in, so the settings are never empty in between.
	db := d.Scanner.DeviceSettingsNoWait()
	d.devMu.Lock()
	d.devGen = gen
	d.dev = devSettingsCache{loaded: true, db: db, patterns: map[string][]string{}}
	d.devMu.Unlock()

	games, err := d.Store.ListGames()
	if err != nil {
		return
	}
	d.adoptExclusionView(games)
}
