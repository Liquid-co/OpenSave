package daemon

import (
	"errors"
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
func (d *Daemon) overlappingGame(path string) (store.Game, bool) {
	games, err := d.Store.ListGames()
	if err != nil {
		return store.Game{}, false
	}
	for _, g := range games {
		if g.SavePath != "" && store.PathsNest(path, g.SavePath) {
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
