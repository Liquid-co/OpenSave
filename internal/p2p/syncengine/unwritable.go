package syncengine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"
)

// A save folder this device may not write to — one under Program Files, for a
// program not run as administrator — fails every pull into it with "access
// denied". That is not something the next pass fixes, and every pass from
// every peer tried again: the same raw error in the activity feed every few
// seconds, saying nothing about what to do.
//
// So it is said once, plainly, with what to do, and pulls into that save wait
// unwritableRetry before trying again. In memory: a restart, which may come
// with new rights, tries straight away.

// unwritableRetry is how long pulls into a save folder that could not be
// written wait before trying it again.
const unwritableRetry = 30 * time.Minute

// errSaveUnwritable is what a pull into such a folder returns, said already.
var errSaveUnwritable = errors.New("the save folder cannot be written")

// unwritableUntil reports whether pulls into gameID's save are waiting, and
// until when.
func (e *Engine) unwritableUntil(gameID string) (time.Time, bool) {
	e.unwritableMu.Lock()
	defer e.unwritableMu.Unlock()
	until, ok := e.unwritable[gameID]
	if !ok || time.Now().After(until) {
		delete(e.unwritable, gameID)
		return time.Time{}, false
	}
	return until, true
}

// noteUnwritable turns a pull's error into errSaveUnwritable when it is the
// folder refusing to be written, saying so once; any other error is returned
// as it is. dir is the folder the file was being written in, saveFolder the
// game's save folder, as said to the user.
//
// "Access denied" alone does not say which: on Windows, replacing a file a
// running game holds open fails the same way. So the folder is asked
// directly, with a small file made and removed in it. If that works, the
// folder takes writes and it was a file in use: the ordinary retry applies.
func (e *Engine) noteUnwritable(gameID, gameName, saveFolder, dir string, err error) error {
	if !errors.Is(err, fs.ErrPermission) || folderTakesWrites(dir) {
		return err
	}
	e.unwritableMu.Lock()
	if e.unwritable == nil {
		e.unwritable = map[string]time.Time{}
	}
	e.unwritable[gameID] = time.Now().Add(unwritableRetry)
	e.unwritableMu.Unlock()
	e.Log("error", fmt.Sprintf("%q: OpenSave may not write to its save folder %s (%v). Give OpenSave write access to the folder, or track the game at another one. Trying again in %d minutes.",
		gameName, saveFolder, err, int(unwritableRetry.Minutes())))
	return fmt.Errorf("%w: %v", errSaveUnwritable, err)
}

// folderTakesWrites reports whether a file can be made, and removed again, in
// dir.
func folderTakesWrites(dir string) bool {
	f, err := os.CreateTemp(dir, ".opensave-probe-*")
	if err != nil {
		return false
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	return true
}
