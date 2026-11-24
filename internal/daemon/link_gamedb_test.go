package daemon

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/opensave/opensave/internal/presets"
	"github.com/opensave/opensave/internal/store"
)

// duckDaemon is a daemon whose game database knows Duck Game, saving in
// LocalLow/Soda/Duck/Saves, with that folder in place. It returns the
// daemon, the game's folder ("Duck") and LocalLow, which holds many games'.
func duckDaemon(t *testing.T) (d *Daemon, duck, low string) {
	t.Helper()
	if runtime.GOOS != "windows" {
		t.Skip("the game database below uses Windows placeholders")
	}
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	low = filepath.Join(home, "AppData", "LocalLow")
	duck = filepath.Join(low, "Soda", "Duck")
	if err := os.MkdirAll(filepath.Join(duck, "Saves"), 0o777); err != nil {
		t.Fatal(err)
	}
	d = newTestDaemon(t)
	dir := t.TempDir()
	manifest := `
Duck Game:
  files:
    "<home>/AppData/LocalLow/Soda/Duck/Saves/*.sav":
      tags: [save]
  steam:
    id: 3167020
`
	if err := os.WriteFile(filepath.Join(dir, "ludusavi-manifest.yaml"), []byte(manifest), 0o666); err != nil {
		t.Fatal(err)
	}
	d.Scanner = &presets.Scanner{CacheFile: filepath.Join(dir, "steam-app-cache.json"), GOOS: "windows"}
	return d, duck, low
}

// trackedAt creates a game at folder without an App ID and asks the game
// database about it, as detection does.
func trackedAt(t *testing.T, d *Daemon, id, folder string) store.Game {
	t.Helper()
	g := store.Game{ID: id, Name: "Duckov", SavePath: folder, ActiveBranch: "main", MaxSnapshots: 5}
	if err := d.Store.CreateGame(g); err != nil {
		t.Fatal(err)
	}
	d.suggestGameDatabaseLink(g)
	g, _ = d.Store.GetGame(id)
	return g
}

// A game tracked without an App ID is not given the game database's on a
// guess: the game its folder plainly is gets suggested, and the App ID is set
// only when the user confirms it. An App ID taken away again is not
// suggested again.
func TestGameDBLink_SuggestedThenConfirmed(t *testing.T) {
	d, duck, _ := duckDaemon(t)
	game := trackedAt(t, d, "duckov", duck)

	s := d.SuggestedGameDBLink(game)
	if s == nil || s.AppID != "3167020" || s.Name != "Duck Game" {
		t.Fatalf("suggestion = %+v, want Duck Game (3167020)", s)
	}
	if game.AppID != "" {
		t.Fatal("the App ID was set before anyone confirmed it")
	}
	if err := d.AnswerGameDBLink(game, true); err != nil {
		t.Fatal(err)
	}
	g, _ := d.Store.GetGame(game.ID)
	if g.AppID != "3167020" || g.Name != "Duckov" || g.CoverURL == "" {
		t.Errorf("after confirming: %+v", g)
	}

	g.AppID, g.CoverURL = "", ""
	if err := d.Store.UpdateGame(g); err != nil {
		t.Fatal(err)
	}
	d.suggestGameDatabaseLink(g)
	g, _ = d.Store.GetGame(game.ID)
	if d.SuggestedGameDBLink(g) != nil {
		t.Error("an App ID taken away was suggested again")
	}
}

// Declined, the suggestion goes, nothing is set, and it is not asked again.
func TestGameDBLink_DeclinedIsNotAskedAgain(t *testing.T) {
	d, duck, _ := duckDaemon(t)
	game := trackedAt(t, d, "duckov", duck)
	if err := d.AnswerGameDBLink(game, false); err != nil {
		t.Fatal(err)
	}
	d.suggestGameDatabaseLink(game)
	g, _ := d.Store.GetGame(game.ID)
	if g.AppID != "" || d.SuggestedGameDBLink(g) != nil {
		t.Errorf("a declined link came back: %+v", g)
	}
	if err := d.AnswerGameDBLink(g, true); err != ErrNoGameDBSuggestion {
		t.Errorf("answering with nothing suggested: %v, want ErrNoGameDBSuggestion", err)
	}
}

// A folder holding many games' saves is not suggested as one of them.
func TestGameDBLink_AWideFolderIsNotSuggested(t *testing.T) {
	d, _, low := duckDaemon(t)
	wide := trackedAt(t, d, "lowlow", low)
	if s := d.SuggestedGameDBLink(wide); s != nil {
		t.Errorf("a folder holding many games' saves was suggested as %+v", s)
	}
}
