package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/opensave/opensave/internal/delta"
	"github.com/opensave/opensave/internal/presets"
	"github.com/opensave/opensave/internal/sessions"
	"github.com/opensave/opensave/internal/store"
)

// Tracking a game by hand, with what the game database knows about it: a
// game picked from those running, found by name, or recognised from a
// program or folder chosen. See presets/gamedb.go.

func (d *Daemon) gameDB() *presets.DeviceSettings {
	if d.Scanner == nil {
		return nil
	}
	d.devMu.Lock()
	defer d.devMu.Unlock()
	return d.devDBLocked()
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

// RunningGame is a game found running on this device.
type RunningGame struct {
	presets.GameMatch
	Exe     string `json:"exe"`
	Tracked bool   `json:"tracked"`
}

// runningMinMemory is what a program must hold to be offered as a game being
// played: a game holds hundreds of megabytes, and what is small is a tool, a
// launcher, or something a game left behind.
const runningMinMemory = 200 << 20

// RunningGames lists the programs running now that the game database knows
// as games, largest first, once per game.
func (d *Daemon) RunningGames() []RunningGame {
	if d.sessions.list == nil {
		return nil
	}
	procs, err := d.sessions.list()
	if err != nil {
		return nil
	}
	sort.Slice(procs, func(i, j int) bool { return procs[i].Memory > procs[j].Memory })
	db := d.gameDB()
	winDir := strings.ToLower(os.Getenv("WINDIR"))
	tracked := d.trackedGameKeys()
	seen := map[string]bool{}
	var out []RunningGame
	for _, p := range procs {
		if !p.Measured || p.Memory < runningMinMemory || p.Exe == "" {
			continue
		}
		if winDir != "" && strings.HasPrefix(strings.ToLower(p.Exe), winDir+string(filepath.Separator)) {
			continue
		}
		m, ok := db.IdentifyProgram(p.Exe)
		if !ok || seen[m.Name] {
			continue
		}
		seen[m.Name] = true
		d.markTracked(&m)
		out = append(out, RunningGame{GameMatch: m, Exe: p.Exe, Tracked: m.TrackedID != "" || tracked[m.AppID] || tracked[sessions.FolderName(m.Name)]})
	}
	return out
}

// trackedGameKeys holds every tracked game's App ID and name, to say which
// running games are tracked already.
func (d *Daemon) trackedGameKeys() map[string]bool {
	keys := map[string]bool{}
	games, err := d.Store.ListGames()
	if err != nil {
		return keys
	}
	for _, g := range games {
		if g.AppID != "" {
			keys[g.AppID] = true
		}
		keys[sessions.FolderName(g.Name)] = true
	}
	return keys
}

// holdsNoSave reports a folder with no file any save is made of: empty, or
// only what a launcher keeps there (delta.NeverSynced).
func holdsNoSave(path string) bool {
	m, err := delta.BuildManifest(path)
	if err != nil {
		return false // cannot tell: not "nothing"
	}
	for p := range m.Files {
		if !delta.NeverSynced(p) {
			return false
		}
	}
	return true
}

// overlappingGame is the tracked game whose folder holds path, or is inside
// it, if there is one.
func (d *Daemon) overlappingGame(path string) (store.Game, bool) {
	games, err := d.Store.ListGames()
	if err != nil {
		return store.Game{}, false
	}
	for _, g := range games {
		if g.SavePath != "" && store.PathsOverlap(path, g.SavePath) {
			return g, true
		}
	}
	return store.Game{}, false
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

func gameDBMark(gameID string) string { return "gamedb:" + gameID }

// linkToGameDatabase gives a game tracked without an App ID the one the game
// database knows its folder by, strictly (IdentifyFolderStrictly): a folder
// tracked by hand, or found under a name of the scan's own making ("Pal
// (Epic/Unreal Save)"), gets cover art, launching, session detection and its
// device settings. The name is the user's and is left as it is. Once per
// game and folder.
func (d *Daemon) linkToGameDatabase(game store.Game) store.Game {
	if game.AppID != "" || game.SavePath == "" {
		return game
	}
	if d.Store.Mark(gameDBMark(game.ID)) == game.SavePath {
		return game
	}
	_ = d.Store.SetMark(gameDBMark(game.ID), game.SavePath)
	m, ok := d.gameDB().IdentifyFolderStrictly(game.SavePath)
	if !ok || m.AppID == "" {
		return game
	}
	game.AppID = m.AppID
	if game.CoverURL == "" {
		game.CoverURL = SteamCoverURL(m.AppID)
	}
	if err := d.Store.UpdateGame(game); err != nil {
		return game
	}
	d.Log.Log("info", fmt.Sprintf("%q is %s in the game database: cover art, its settings and seeing it played come from there", game.Name, m.Name))
	d.exclusionsChanged(game)
	return game
}
