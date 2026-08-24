package daemon

import (
	"fmt"
	"os"

	"github.com/opensave/opensave/internal/snapshot"
	"github.com/opensave/opensave/internal/store"
	"github.com/opensave/opensave/internal/winreg"
)

// setUpRegistryCapture gives a game whose saves live in the registry somewhere
// to keep them.
//
// The keys come from the manifest. The capture itself is a file in a directory
// OpenSave owns, registered as one of the game's save locations — from there
// the ordinary machinery archives it, hashes it, and syncs it, and no snapshot
// or sync code has to know the registry exists. See internal/winreg for why a
// capture has to be a real file rather than something the archive holds alone.
//
// Everything is best-effort and logged. A game whose registry half could not be
// set up is still tracked and still backed up: its files are the larger part of
// the save, and refusing to track it at all would be a worse answer than
// tracking it and saying what is missing.
func (d *Daemon) setUpRegistryCapture(game store.Game) {
	if d.Scanner == nil {
		return
	}
	keys := d.Scanner.RegistryKeysFor(game.Name, game.AppID)
	if len(keys) == 0 {
		return
	}

	if err := d.Store.SetGameRegistryKeys(game.ID, keys); err != nil {
		d.Log.Log("warn", fmt.Sprintf(
			"could not record the registry saves for %q: %v — its files will still be backed up",
			game.Name, err))
		return
	}

	settings, err := d.Store.GetSettings()
	if err != nil {
		return
	}
	dir := snapshot.RegistryDir(settings.BackupsDir, game.ID)
	if err := os.MkdirAll(dir, 0o777); err != nil {
		d.Log.Log("warn", fmt.Sprintf(
			"could not prepare the registry location for %q: %v", game.Name, err))
		return
	}
	if err := d.Store.AddGameRoot(game.ID, winreg.LocationName, dir); err != nil {
		d.Log.Log("warn", fmt.Sprintf(
			"could not add the registry save location for %q: %v", game.Name, err))
		return
	}

	// Say so plainly on a device that cannot read a registry. A Steam Deck
	// syncing with a Windows machine still carries these captures and hands
	// them back on restore, but it contributes none of its own — and a user
	// who tracked the game there deserves to know its save is only half
	// covered from this end.
	if !winreg.Available() {
		d.Log.Log("warn", fmt.Sprintf(
			"%q keeps %d save(s) in the Windows registry, which cannot be read on this device — "+
				"its files are backed up here, and the registry half syncs from a Windows device",
			game.Name, len(keys)))
		return
	}
	d.Log.Log("info", fmt.Sprintf(
		"%q keeps %d save(s) in the registry; they are included in its snapshots", game.Name, len(keys)))
}
