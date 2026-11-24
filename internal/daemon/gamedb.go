package daemon

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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

// A game tracked without an App ID — a folder tracked by hand, or found by
// the scan under a name of its own making ("Pal (Epic/Unreal Save)") — has no
// cover art, launching, session detection or device settings. When its folder
// plainly is one game's in the game database, that is suggested on the game's
// page, and the App ID is set only when the user confirms it: through the
// App ID the database's settings files are left out of syncing, and a wrong
// guess would leave out files of the save without anyone knowing.

func gameDBMark(gameID string) string { return "gamedb:" + gameID }

func gameDBSuggestionMark(gameID string) string { return "gamedb-suggestion:" + gameID }

// GameDBSuggestion is the game in the game database a game tracked without an
// App ID looks like, waiting for the user's answer.
type GameDBSuggestion struct {
	AppID string `json:"appId"`
	Name  string `json:"name"`
}

// suggestGameDatabaseLink asks the game database about a game tracked without
// an App ID, strictly (IdentifyFolderStrictly), and records what it says as a
// suggestion. Once per game and folder (a mark): an answer is not asked for
// again, nor an App ID someone took away suggested again.
func (d *Daemon) suggestGameDatabaseLink(game store.Game) {
	if game.AppID != "" || game.SavePath == "" {
		return
	}
	if d.Store.Mark(gameDBMark(game.ID)) == game.SavePath {
		return
	}
	_ = d.Store.SetMark(gameDBMark(game.ID), game.SavePath)
	m, ok := d.gameDB().IdentifyFolderStrictly(game.SavePath)
	if !ok || m.AppID == "" {
		return
	}
	if d.Store.SetMark(gameDBSuggestionMark(game.ID), m.AppID+"\n"+m.Name) != nil {
		return
	}
	d.Log.Log("info", fmt.Sprintf("%q looks like %s in the game database — link it from the game's configuration tab for its cover art, settings and seeing it played", game.Name, m.Name))
	if d.OnGameChanged != nil {
		d.OnGameChanged(game.ID)
	}
}

// SuggestedGameDBLink is the suggestion waiting for a game, if any.
func (d *Daemon) SuggestedGameDBLink(game store.Game) *GameDBSuggestion {
	if game.AppID != "" {
		return nil
	}
	appID, name, ok := strings.Cut(d.Store.Mark(gameDBSuggestionMark(game.ID)), "\n")
	if !ok || appID == "" {
		return nil
	}
	return &GameDBSuggestion{AppID: appID, Name: name}
}

// ErrNoGameDBSuggestion is an answer for a game with no suggestion waiting.
var ErrNoGameDBSuggestion = errors.New("no game database link is suggested for this game")

// AnswerGameDBLink records the user's answer to a game's suggestion: accept
// gives the game the App ID, and cover art if it had none — its name stays
// the user's — and its device settings follow; otherwise the suggestion goes,
// and is not made again for this folder.
func (d *Daemon) AnswerGameDBLink(game store.Game, accept bool) error {
	s := d.SuggestedGameDBLink(game)
	if s == nil {
		return ErrNoGameDBSuggestion
	}
	_ = d.Store.SetMark(gameDBSuggestionMark(game.ID), "")
	if !accept {
		return nil
	}
	game.AppID = s.AppID
	if game.CoverURL == "" {
		game.CoverURL = SteamCoverURL(s.AppID)
	}
	if err := d.Store.UpdateGame(game); err != nil {
		return err
	}
	d.Log.Log("info", fmt.Sprintf("%q is linked to %s in the game database: cover art, its settings and seeing it played come from there", game.Name, s.Name))
	d.exclusionsChanged(game)
	return nil
}

// RunningGame is a game the game database knows with a program of it running
// now: what "Track game" offers first, since the game being played is the one
// most often tracked.
type RunningGame struct {
	presets.GameMatch
	// Exe is the program seen running, tracked as the game's program.
	Exe string `json:"exe"`
}

// RunningGames lists the games the game database knows that have a program
// running now, by where the program is installed (GameDB.IdentifyProgram);
// once per game, by name. Programs of the system itself are not looked at.
func (d *Daemon) RunningGames() []RunningGame {
	if d.sessions.list == nil {
		return nil
	}
	procs, err := d.sessions.list()
	if err != nil {
		return nil
	}
	db := d.gameDB()
	winDir := strings.ToLower(os.Getenv("WINDIR"))
	seen := map[string]bool{}
	out := []RunningGame{}
	for _, p := range procs {
		if p.Exe == "" || neverAGame(p.Exe) {
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
		out = append(out, RunningGame{GameMatch: m, Exe: p.Exe})
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

// nonGamePrograms are programs that are never the game being played, however
// their folders are named: web browsers, open on most desktops, and one of
// them ("Google\Chrome") shares its folder name with a game.
var nonGamePrograms = map[string]bool{
	"chrome": true, "google chrome": true, "google-chrome": true, "chromium": true, "msedge": true,
	"firefox": true, "brave": true, "opera": true, "opera_gx": true, "vivaldi": true, "iexplore": true,
	"arc": true, "safari": true,
}

// neverAGame reports whether a running program is one the running games list
// leaves out by name: a browser, or OpenSave itself.
func neverAGame(exe string) bool {
	base := filepath.Base(strings.ReplaceAll(exe, `\`, "/"))
	name := strings.ToLower(strings.TrimSuffix(base, filepath.Ext(base)))
	return nonGamePrograms[name] || strings.HasPrefix(name, "opensave")
}
