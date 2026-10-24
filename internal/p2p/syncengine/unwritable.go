package syncengine

import (
	"errors"
	"fmt"
	"io/fs"
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
// as it is.
func (e *Engine) noteUnwritable(gameID, gameName, path string, err error) error {
	if !errors.Is(err, fs.ErrPermission) {
		return err
	}
	e.unwritableMu.Lock()
	if e.unwritable == nil {
		e.unwritable = map[string]time.Time{}
	}
	e.unwritable[gameID] = time.Now().Add(unwritableRetry)
	e.unwritableMu.Unlock()
	e.Log("error", fmt.Sprintf("%q: OpenSave may not write to its save folder %s (%v). Give OpenSave write access to the folder, or track the game at another one. Trying again in %d minutes.",
		gameName, path, err, int(unwritableRetry.Minutes())))
	return fmt.Errorf("%w: %v", errSaveUnwritable, err)
}
