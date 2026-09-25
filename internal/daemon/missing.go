package daemon

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

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

// noteMissing records whether a game's save folder is missing, and says so
// once when that changes — not on every minute's check — so the log and the
// screen follow the folder going and coming back.
func (d *Daemon) noteMissing(gameID, name, savePath string, missing bool) {
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
	if missing {
		d.Log.Log("warn", fmt.Sprintf("the save folder of %q is not there (%s) — it is not watched or synced until it is back; "+
			"OpenSave will not create it, since an empty folder in its place would read as every file deleted", name, logging.Quote(savePath)))
	} else {
		d.Log.Log("success", fmt.Sprintf("the save folder of %q is back (%s); watching it again", name, logging.Quote(savePath)))
	}
	if d.OnGameChanged != nil {
		d.OnGameChanged(gameID)
	}
}
