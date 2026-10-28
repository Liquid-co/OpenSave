package daemon

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/opensave/opensave/internal/presets"
	"github.com/opensave/opensave/internal/store"
)

// A game tracked without an App ID is not given the game database's on a
// guess: the game its folder plainly is gets suggested, and the App ID is set
// only when the user confirms it. A folder holding many games' saves is not
// suggested as one; an answer either way, and an App ID taken away, are not
// asked about again.
func TestGameDBLink_SuggestedThenConfirmed(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the game database below uses Windows placeholders")
	}
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	low := filepath.Join(home, "AppData", "LocalLow")
	duck := filepath.Join(low, "Soda", "Duck")
	if err := os.MkdirAll(filepath.Join(duck, "Saves"), 0o777); err != nil {
		t.Fatal(err)
	}
	d := newTestDaemon(t)
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

	game := store.Game{ID: "duckov", Name: "Duckov", SavePath: duck, ActiveBranch: "main", MaxSnapshots: 5}
	declined := store.Game{ID: "duckov2", Name: "Duckov copy", SavePath: duck, ActiveBranch: "main", MaxSnapshots: 5}
	wide := store.Game{ID: "lowlow", Name: "Everything in LocalLow", SavePath: low, ActiveBranch: "main", MaxSnapshots: 5}
	for _, g := range []store.Game{game, declined, wide} {
		if err := d.Store.CreateGame(g); err != nil {
			t.Fatal(err)
		}
		d.suggestGameDatabaseLink(g)
	}
	reload := func(id string) store.Game {
		g, _ := d.Store.GetGame(id)
		return g
	}

	// Suggested, not set.
	s := d.SuggestedGameDBLink(reload(game.ID))
	if s == nil || s.AppID != "3167020" || s.Name != "Duck Game" {
		t.Fatalf("suggestion = %+v, want Duck Game (3167020)", s)
	}
	if reload(game.ID).AppID != "" {
		t.Fatal("the App ID was set before anyone confirmed it")
	}
	if d.SuggestedGameDBLink(reload(wide.ID)) != nil {
		t.Error("a folder holding many games' saves was suggested as one")
	}

	// Confirmed: linked, the name left as it is.
	if err := d.AnswerGameDBLink(reload(game.ID), true); err != nil {
		t.Fatal(err)
	}
	if g := reload(game.ID); g.AppID != "3167020" || g.Name != "Duckov" || g.CoverURL == "" {
		t.Errorf("after confirming: %+v", g)
	}

	// Declined: nothing set, and not asked again.
	if err := d.AnswerGameDBLink(reload(declined.ID), false); err != nil {
		t.Fatal(err)
	}
	d.suggestGameDatabaseLink(reload(declined.ID))
	if g := reload(declined.ID); g.AppID != "" || d.SuggestedGameDBLink(g) != nil {
		t.Errorf("a declined link came back: %+v", g)
	}
	if err := d.AnswerGameDBLink(reload(declined.ID), true); err != ErrNoGameDBSuggestion {
		t.Errorf("answering with nothing suggested: %v, want ErrNoGameDBSuggestion", err)
	}

	// The App ID taken away again: not suggested again.
	g := reload(game.ID)
	g.AppID, g.CoverURL = "", ""
	if err := d.Store.UpdateGame(g); err != nil {
		t.Fatal(err)
	}
	d.suggestGameDatabaseLink(reload(game.ID))
	if d.SuggestedGameDBLink(reload(game.ID)) != nil {
		t.Error("an App ID taken away was suggested again")
	}
}
