package daemon

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/opensave/opensave/internal/switchtitle"
)

// Switch games tracked before OpenSave could name them are shown as the name
// the scan made up — "Citron Switch Emulator - Title ID: 0100F2C0115B6000".
// Each is given the name its emulator knows it by, once one does
// (presets/switchtitles.go). Only a made-up name is replaced: one the person
// typed is theirs, and the id — what the game is matched on across devices —
// does not change either way.

// madeUpSwitchName is the name the scan gave a Switch save it could not name.
var madeUpSwitchName = regexp.MustCompile(`^.+ - Title ID: ([0-9A-Fa-f]{16})$`)

// nameSwitchGames renames every tracked Switch game still under a made-up
// name that an emulator here can now name. Returns how many were renamed.
func (d *Daemon) nameSwitchGames() (renamed int) {
	if d.Scanner == nil {
		return 0
	}
	games, err := d.Store.ListGames()
	if err != nil {
		return 0
	}
	for _, g := range games {
		m := madeUpSwitchName.FindStringSubmatch(g.Name)
		if m == nil || switchtitle.Of(g.ID, g.SavePath) != strings.ToUpper(m[1]) {
			continue
		}
		name := d.Scanner.SwitchTitleName(strings.ToUpper(m[1]), g.SavePath)
		if name == "" || name == g.Name {
			continue
		}
		if ok, err := d.Store.RenameGameIf(g.ID, g.Name, name); err == nil && ok {
			renamed++
			d.Log.Log("info", fmt.Sprintf("named %q %q, as its emulator knows it", g.Name, name))
		}
	}
	if renamed > 0 && d.OnGameChanged != nil {
		d.OnGameChanged("")
	}
	return renamed
}
