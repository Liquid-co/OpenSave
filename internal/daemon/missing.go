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
// letter. Exactly one match is adopted for this device, and said once. None, or
// more than one (which would be a guess), is said once too, and the search is
// not repeated more often than relocateRecheck: it touches every drive, and the
// minute-long watch reconcile asks about missing folders constantly.
const relocateRecheck = time.Hour

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
		if !SaveFolderMissing(candidate) {
			found = append(found, candidate)
		}
	}
	return found
}

// relocateToOtherDrive switches a game whose save folder is missing to the same
// path on another drive, when there is exactly one. Reports whether it did.
func (d *Daemon) relocateToOtherDrive(gameID, name, savePath string) bool {
	d.missingMu.Lock()
	last, checked := d.relocateChecked[gameID]
	if checked && time.Since(last) < relocateRecheck {
		d.missingMu.Unlock()
		return false
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

	newPath, err := d.ValidateSavePath(found[0])
	if err != nil {
		d.Log.Log("warn", fmt.Sprintf("the save folder of %q exists at %s, but it cannot be used: %v", name, logging.Quote(found[0]), err))
		return false
	}
	game, err := d.Store.GetGame(gameID)
	if err != nil || game.SavePath != savePath {
		return false // changed meanwhile; the next reconcile sees the new path
	}
	game.SavePath = newPath
	if err := d.Store.UpdateGame(game); err != nil {
		d.Log.Log("warn", fmt.Sprintf("could not switch %q to %s: %v", name, logging.Quote(newPath), err))
		return false
	}
	d.Log.Log("success", fmt.Sprintf("the save folder of %q was not at %s but is at %s on this device — using that",
		name, logging.Quote(savePath), logging.Quote(newPath)))
	d.missingMu.Lock()
	delete(d.missing, gameID)
	d.missingMu.Unlock()
	if d.OnGameChanged != nil {
		d.OnGameChanged(gameID)
	}
	return true
}

// noteMissing records whether a game's save folder is missing, and says so
// once when that changes — not on every minute's check — so the log and the
// screen follow the folder going and coming back.
func (d *Daemon) noteMissing(gameID, name, savePath string, missing bool) {
	if missing && d.relocateToOtherDrive(gameID, name, savePath) {
		return // found under another drive letter and switched to it
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
	}
	d.missingMu.Unlock()
	if was == missing {
		return
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
