package daemon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/opensave/opensave/internal/ignore"
	"github.com/opensave/opensave/internal/store"
)

// A file found by what it holds is only suggested: it keeps syncing until
// someone confirms it in the game's configuration tab, and a file someone
// chose to keep syncing is not suggested again (asked in review on #118).
func TestSuggestedSettings_SyncUntilConfirmed(t *testing.T) {
	d := newTestDaemon(t)
	dir := t.TempDir()
	settings := "setting.fullscreen 1\nsetting.mat_vsync 0\nsetting.gamma 1\nsetting.defaultres 1920\nsetting.ssao 1\n"
	for name, content := range map[string]string{
		"slot1.sav":       "progress",
		"videoconfig.txt": settings,
		"display.cfg":     settings,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o666); err != nil {
			t.Fatal(err)
		}
	}
	game := store.Game{ID: "g", Name: "No Such Game In The Database", SavePath: dir, ActiveBranch: "main", MaxSnapshots: 5}
	if err := d.Store.CreateGame(game); err != nil {
		t.Fatal(err)
	}
	d.devMu.Lock()
	d.dev = devSettingsCache{loaded: true, patterns: map[string][]string{}, verdicts: map[string][]store.SettingsFile{}}
	d.devMu.Unlock()
	excluded := func(p string) bool { return ignore.Parse(d.IgnoreText(game)).Match(p) }

	d.DetectSettings(game)
	suggested := map[string]bool{}
	for _, s := range d.SuggestedSettings(game) {
		suggested[s.Pattern] = true
	}
	if !suggested["/videoconfig.txt"] || !suggested["/display.cfg"] {
		t.Fatalf("setup: the settings files were not suggested: %v", suggested)
	}
	if suggested["/slot1.sav"] {
		t.Error("the save was suggested as settings")
	}
	if excluded("videoconfig.txt") || excluded("display.cfg") {
		t.Fatal("a suggested file was left out of syncing before anyone confirmed it")
	}

	if err := d.DecideSuggestedSettings(game, "videoconfig.txt", true); err != nil {
		t.Fatal(err)
	}
	if err := d.DecideSuggestedSettings(game, "display.cfg", false); err != nil {
		t.Fatal(err)
	}
	if !excluded("videoconfig.txt") {
		t.Error("a confirmed settings file still syncs")
	}
	if excluded("display.cfg") {
		t.Error("a file chosen to keep syncing was left out")
	}
	if len(d.SuggestedSettings(game)) != 0 {
		t.Errorf("answered files are still suggested: %v", d.SuggestedSettings(game))
	}

	// Detection runs again: neither answer is undone, nothing suggested anew.
	d.DetectSettings(game)
	if len(d.SuggestedSettings(game)) != 0 {
		t.Errorf("a file already answered was suggested again: %v", d.SuggestedSettings(game))
	}
	if !excluded("videoconfig.txt") || excluded("display.cfg") {
		t.Error("running detection again changed what was answered")
	}
	if err := d.DecideSuggestedSettings(game, "display.cfg", true); err != ErrNotSuggested {
		t.Errorf("answering a file that is not suggested: %v, want ErrNotSuggested", err)
	}
}
