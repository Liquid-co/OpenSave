package daemon

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/opensave/opensave/internal/delta"
	"github.com/opensave/opensave/internal/logging"
)

// Save folders that are not there.
//
// A tracked game's save folder can go: deleted, moved by a reinstall, on a
// drive or an SD card that is not plugged in. OpenSave does not create it
// again (see watcher.WatchWithLocations for what that used to cost); it says
// so, watches nothing there, and picks the folder up again within a minute
// of it coming back (ResyncWatchers).

// SaveFolderMissing reports whether a save path's folder is not there. For a
// save that is a single file, the file itself may not exist yet — a game
// writes it on first save — so it is the folder it would go in that counts.
func SaveFolderMissing(savePath string) bool {
	if savePath == "" {
		return false
	}
	folder := savePath
	if isFile, err := delta.ResolveLocalSaveFilePath(savePath); err == nil && isFile {
		folder = filepath.Dir(savePath)
	}
	_, err := os.Stat(folder)
	return errors.Is(err, fs.ErrNotExist)
}

// SaveDriveMissing reports whether a save path lives on a drive this device
// does not have at all — a D:\ or E:\ Steam library that exists on another
// PC. That is where a game auto-tracked from a peer lands when the peer's
// library is on a drive letter this machine lacks, and it is not something
// to warn about: nothing was lost here, the game just isn't on this device.
// Only a volume that is plainly absent counts; an unreachable network share
// reports some other error and stays a warning.
func SaveDriveMissing(savePath string) bool {
	vol := filepath.VolumeName(savePath)
	if vol == "" {
		return false
	}
	_, err := os.Stat(vol + string(filepath.Separator))
	return errors.Is(err, fs.ErrNotExist)
}

// The same save folder under another drive letter.
//
// A save inside a game's install folder — a Steam library, most often — sits
// on whichever drive that library is on, and that differs between devices: D:
// on one, E: on another. A game tracked on one device and auto-tracked on
// another arrives with the first device's drive letter, and on the second the
// folder is simply "missing" though it is right there on another drive.
//
// So a missing folder is looked for at the identical path on every other drive
// letter. Exactly one match is offered, not adopted: the same path on another
// drive can also be an old backup copy, and syncing from that would hand stale
// saves to every device. The game shows the folder found and asks; the log says
// so once, with the command that answers it (Daemon.AcceptRelocation,
// DismissRelocation). None, or more than one (which would be a guess), is said
// once too, and the search is not repeated more often than relocateRecheck: it
// touches every drive, and the minute-long watch reconcile asks about missing
// folders constantly.
const relocateRecheck = time.Hour

// statSaveFolder is os.Stat, replaced by a test that needs a drive that is not
// ready.
var statSaveFolder = os.Stat

// saveFolderThere reports whether a save path's folder can be seen to exist.
// It is not the opposite of SaveFolderMissing, which says "missing" only of a
// folder plainly not there. A drive with no disk in it, an empty card reader
// or an unreachable share answers with another error: not missing, and not
// there either. Taken as there, an empty card reader made a second match for
// every folder found on another drive, and nothing was ever offered.
func saveFolderThere(savePath string) bool {
	folder := savePath
	if isFile, err := delta.ResolveLocalSaveFilePath(savePath); err == nil && isFile {
		folder = filepath.Dir(savePath)
	}
	_, err := statSaveFolder(folder)
	return err == nil
}

// relocateCandidates lists the folders at savePath's exact path under the other
// drive letters that exist here. Windows drive letters only.
func relocateCandidates(savePath string) []string {
	vol := filepath.VolumeName(savePath)
	if len(vol) != 2 || vol[1] != ':' {
		return nil
	}
	rest := savePath[2:]
	own := strings.ToUpper(vol[:1])
	var found []string
	for c := 'C'; c <= 'Z'; c++ {
		letter := string(c)
		if letter == own {
			continue
		}
		candidate := letter + ":" + rest
		if saveFolderThere(candidate) {
			found = append(found, candidate)
		}
	}
	return found
}

// suggestOtherDrive looks for a missing save folder at the same path on the
// other drive letters, and offers exactly one match (RelocateSuggestion) rather
// than switching to it. Reports whether there is a suggestion now.
func (d *Daemon) suggestOtherDrive(gameID, name, savePath string) bool {
	d.missingMu.Lock()
	last, checked := d.relocateChecked[gameID]
	if checked && time.Since(last) < relocateRecheck {
		_, suggested := d.relocateSuggested[gameID]
		d.missingMu.Unlock()
		return suggested
	}
	if d.relocateChecked == nil {
		d.relocateChecked = map[string]time.Time{}
	}
	d.relocateChecked[gameID] = time.Now()
	d.missingMu.Unlock()

	if name == "" {
		name = gameID
	}
	found := relocateCandidates(savePath)
	switch {
	case len(found) == 0:
		if !checked {
			d.Log.Log("info", fmt.Sprintf("the save folder of %q is not on another drive here either (looked for %s on every drive letter)",
				name, logging.Quote(savePath[min(2, len(savePath)):])))
		}
		return false
	case len(found) > 1:
		if !checked {
			d.Log.Log("warn", fmt.Sprintf("the save folder of %q is missing at %s, and the same path exists on %d other drives (%s) — not guessing which; set it with Edit",
				name, logging.Quote(savePath), len(found), strings.Join(found, ", ")))
		}
		return false
	}

	d.missingMu.Lock()
	defer d.missingMu.Unlock()
	if strings.EqualFold(d.relocateDismissed[gameID], found[0]) {
		return false // the user said this one is not it
	}
	if d.relocateFor == nil {
		d.relocateFor = map[string]string{}
	}
	d.relocateFor[gameID] = savePath
	if strings.EqualFold(d.relocateSuggested[gameID], found[0]) {
		return true // offered already
	}
	if d.relocateSuggested == nil {
		d.relocateSuggested = map[string]string{}
	}
	d.relocateSuggested[gameID] = found[0]
	d.Log.Log("warn", fmt.Sprintf("the save folder of %q is not at %s, but the same folder exists at %s on this device. "+
		"It is not used until you confirm it is the right one (it could be an old copy): open the game in OpenSave, "+
		"or run `opensave relocate %s use` (or `dismiss`)",
		name, logging.Quote(savePath), logging.Quote(found[0]), gameID))
	return true
}

// RelocateSuggestion is the folder offered for a game whose save folder is
// missing — the same path on another drive — or "" when there is none.
func (d *Daemon) RelocateSuggestion(gameID string) string {
	d.missingMu.Lock()
	defer d.missingMu.Unlock()
	return d.relocateSuggested[gameID]
}

// AcceptRelocation switches a game to the folder offered for it. path must be
// the one offered, so an answer to an older question does not switch to a
// folder nobody was shown. And the game must still be at the missing path the
// offer was made for: a folder set by hand while the offer was open is the
// answer that counts, and accepting the old offer would have overwritten it.
func (d *Daemon) AcceptRelocation(gameID, path string) error {
	d.missingMu.Lock()
	offered, offeredFor := d.relocateSuggested[gameID], d.relocateFor[gameID]
	d.missingMu.Unlock()
	if offered == "" || !strings.EqualFold(offered, path) {
		return fmt.Errorf("no folder is offered for this game at %s", path)
	}
	game, err := d.Store.GetGame(gameID)
	if err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Clean(game.SavePath), filepath.Clean(offeredFor)) {
		d.missingMu.Lock()
		delete(d.relocateSuggested, gameID)
		delete(d.relocateFor, gameID)
		d.missingMu.Unlock()
		return fmt.Errorf("the save folder of %q was changed to %s after %s was offered; the offer no longer applies",
			game.Name, game.SavePath, offered)
	}
	newPath, err := d.ValidateSavePath(offered)
	if err != nil {
		return fmt.Errorf("the folder at %s cannot be used: %w", offered, err)
	}
	old := game.SavePath
	game.SavePath = newPath
	if err := d.Store.UpdateGame(game); err != nil {
		return fmt.Errorf("could not switch %q to %s: %w", game.Name, newPath, err)
	}
	d.missingMu.Lock()
	delete(d.relocateSuggested, gameID)
	delete(d.relocateFor, gameID)
	delete(d.missing, gameID)
	d.missingMu.Unlock()
	d.Log.Log("success", fmt.Sprintf("the save folder of %q is now %s on this device, as confirmed (it was %s)",
		game.Name, logging.Quote(newPath), logging.Quote(old)))
	if d.OnGameChanged != nil {
		d.OnGameChanged(gameID)
	}
	return nil
}

// DismissRelocation answers the offer for a game with "not this one": that
// folder is not offered again for it while OpenSave runs.
func (d *Daemon) DismissRelocation(gameID string) error {
	d.missingMu.Lock()
	offered := d.relocateSuggested[gameID]
	if offered == "" {
		d.missingMu.Unlock()
		return fmt.Errorf("no folder is offered for this game")
	}
	delete(d.relocateSuggested, gameID)
	delete(d.relocateFor, gameID)
	if d.relocateDismissed == nil {
		d.relocateDismissed = map[string]string{}
	}
	d.relocateDismissed[gameID] = offered
	d.missingMu.Unlock()
	d.Log.Log("info", fmt.Sprintf("not using %s for %s, as answered; set the save folder with Edit if it is elsewhere",
		logging.Quote(offered), gameID))
	return nil
}

// noteMissing records whether a game's save folder is missing, and says so
// once when that changes — not on every minute's check — so the log and the
// screen follow the folder going and coming back.
func (d *Daemon) noteMissing(gameID, name, savePath string, missing bool) {
	if missing {
		// Offered, not switched to: the game stays missing until someone says
		// the folder found is the right one (suggestOtherDrive).
		d.suggestOtherDrive(gameID, name, savePath)
	}
	d.missingMu.Lock()
	was := d.missing[gameID]
	if missing {
		if d.missing == nil {
			d.missing = map[string]bool{}
		}
		d.missing[gameID] = true
	} else {
		delete(d.missing, gameID)
		delete(d.relocateSuggested, gameID) // back where it was: nothing to offer
		delete(d.relocateFor, gameID)
	}
	d.missingMu.Unlock()
	if was == missing {
		return
	}
	if missing && d.P2P != nil && d.P2P.Sync != nil {
		// "Missing" wins over "emptied": a question asked as the folder's
		// files went first is dropped (syncengine.SaveFolderGone).
		d.P2P.Sync.SaveFolderGone(gameID)
	}
	if name == "" {
		name = gameID
	}
	if missing && SaveDriveMissing(savePath) {
		d.Log.Log("info", fmt.Sprintf("the save folder of %q is on %s, which this device doesn't have (%s) — the game is skipped here "+
			"and keeps syncing between the devices that have it", name, filepath.VolumeName(savePath), logging.Quote(savePath)))
	} else if missing {
		d.Log.Log("warn", fmt.Sprintf("the save folder of %q is not there (%s) — it is not watched or synced until it is back; "+
			"OpenSave will not create it, since an empty folder in its place would read as every file deleted", name, logging.Quote(savePath)))
	} else {
		d.Log.Log("success", fmt.Sprintf("the save folder of %q is back (%s); watching it again", name, logging.Quote(savePath)))
	}
	if d.OnGameChanged != nil {
		d.OnGameChanged(gameID)
	}
}
