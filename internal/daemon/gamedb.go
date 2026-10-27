package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/opensave/opensave/internal/presets"
	"github.com/opensave/opensave/internal/store"
)

// Tracking a game by hand, with what the game database knows about it: a
// game found by name, or recognised from a program or folder chosen. See
// presets/gamedb.go.

func (d *Daemon) gameDB() *presets.GameDB {
	if d.Scanner == nil {
		return nil
	}
	return d.Scanner.GameDB()
}

// SearchGames finds games in the game database by name.
func (d *Daemon) SearchGames(query string) []presets.GameMatch {
	out := d.gameDB().Search(query, 20)
	for i := range out {
		d.markTracked(&out[i])
	}
	return out
}

// IdentifyGame says which game a program or folder belongs to: a program by
// where it is installed, a folder by the game whose save location it is.
func (d *Daemon) IdentifyGame(path string) (presets.GameMatch, bool) {
	db := d.gameDB()
	info, err := os.Stat(path)
	if err != nil {
		return presets.GameMatch{}, false
	}
	var m presets.GameMatch
	var ok bool
	switch {
	case !info.IsDir() && strings.EqualFold(filepath.Ext(path), ".exe"):
		m, ok = db.IdentifyProgram(path)
	case !info.IsDir():
		m, ok = db.IdentifyFolder(filepath.Dir(path)) // a save file: its folder
	default:
		m, ok = db.IdentifyFolder(path)
	}
	if ok {
		d.markTracked(&m)
	}
	return m, ok
}

// overlappingGame is the tracked game whose folder holds path, or is inside
// it, if there is one. The very same folder is not counted: TrackGame says
// so in its own words.
func (d *Daemon) overlappingGame(path string) (store.Game, bool, error) {
	games, err := d.Store.ListGames()
	if err != nil {
		return store.Game{}, false, fmt.Errorf("inspect native tracked paths: %w", err)
	}
	for _, g := range games {
		if g.SavePath != "" && store.PathsNest(path, g.SavePath) {
			return g, true, nil
		}
	}
	return store.Game{}, false, nil
}

// markTracked fills in which tracked game, if any, a match already is.
func (d *Daemon) markTracked(m *presets.GameMatch) {
	games, err := d.Store.ListGames()
	if err != nil {
		return
	}
	for _, g := range games {
		same := m.AppID != "" && g.AppID == m.AppID
		for _, p := range m.SavePaths {
			if g.SavePath != "" && store.PathsOverlap(p, g.SavePath) {
				same = true
			}
		}
		if same {
			m.TrackedID, m.TrackedName = g.ID, g.Name
			return
		}
	}
}
