package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/opensave/opensave/internal/delta"
	"github.com/opensave/opensave/internal/ignore"
	"github.com/opensave/opensave/internal/store"
	"github.com/opensave/opensave/internal/watcher"
)

// When the game database starts naming a game's settings, the hash its last
// snapshot recorded is carried over — only where the save is the one it was
// recorded for — so the watcher does not take the new view for a change and
// snapshot a game nobody played.
func TestAdoptExclusionView_CarriesTheLastSnapshotHashOver(t *testing.T) {
	d := newTestDaemon(t)
	mk := func(id string, files map[string]string) store.Game {
		dir := filepath.Join(t.TempDir(), id)
		for name, data := range files {
			if err := os.MkdirAll(dir, 0o777); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o666); err != nil {
				t.Fatal(err)
			}
		}
		g := store.Game{ID: id, Name: id, SavePath: dir, ActiveBranch: "main", MaxSnapshots: 5}
		if err := d.Store.CreateGame(g); err != nil {
			t.Fatal(err)
		}
		m, err := delta.BuildManifest(dir)
		if err != nil {
			t.Fatal(err)
		}
		if err := d.Store.SetLastManifestHash(id, watcher.ContentHash(m, "")); err != nil {
			t.Fatal(err)
		}
		g, _ = d.Store.GetGame(id)
		return g
	}
	same := mk("same", map[string]string{"slot1.sav": "a", "graphics.xml": "4k"})
	changed := mk("changed", map[string]string{"slot1.sav": "a", "graphics.xml": "4k"})
	// A real change since the last snapshot, not yet seen by the watcher.
	if err := os.WriteFile(filepath.Join(changed.SavePath, "slot1.sav"), []byte("b"), 0o666); err != nil {
		t.Fatal(err)
	}
	delta.InvalidateRoot(changed.SavePath)
	_ = d.Store.SetMark("never_synced", delta.NeverSyncedList)

	d.devMu.Lock()
	d.dev = devSettingsCache{loaded: true, patterns: map[string][]string{}, verdicts: map[string][]store.SettingsFile{}}
	for _, g := range []store.Game{same, changed} {
		d.dev.patterns[g.Name+"\x00"+g.AppID+"\x00"+g.SavePath] = []string{"/graphics.xml"}
	}
	d.devMu.Unlock()

	d.adoptExclusionView([]store.Game{same, changed})

	hashNow := func(g store.Game) string {
		m, _ := delta.BuildManifest(g.SavePath)
		return watcher.ContentHash(m, d.IgnoreText(g))
	}
	if g, _ := d.Store.GetGame("same"); g.LastManifestHash != hashNow(g) {
		t.Error("an unchanged save's last-snapshot hash was not carried over: the watcher will snapshot it")
	}
	if g, _ := d.Store.GetGame("changed"); g.LastManifestHash == hashNow(g) {
		t.Error("a changed save's hash was carried over: its change would never be snapshotted")
	}
	if d.Store.Mark(exclusionsMark("same")) == "" {
		t.Error("nothing recorded: this would run again at every start")
	}
}

// A file taken for settings that another device's copy shows changing with
// every change of the save is given back to syncing; one that stays put while
// the save moves stays left out.
func TestObserveSettings_GivesBackWhatChangesLikeASave(t *testing.T) {
	d := newTestDaemon(t)
	dir := t.TempDir()
	game := store.Game{ID: "g", Name: "g", SavePath: dir, ActiveBranch: "main", MaxSnapshots: 5}
	if err := d.Store.CreateGame(game); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"progress.cfg", "video.cfg"} {
		if err := d.Store.SetSettingsFile(store.SettingsFile{GameID: "g", Path: p, Verdict: store.VerdictSettings, Reason: "test"}); err != nil {
			t.Fatal(err)
		}
	}
	d.devMu.Lock()
	d.dev = devSettingsCache{loaded: true, patterns: map[string][]string{}, verdicts: map[string][]store.SettingsFile{}}
	d.devMu.Unlock()
	excluded := func(p string) bool { return ignore.Parse(d.IgnoreText(game)).Match(p) }
	if !excluded("progress.cfg") || !excluded("video.cfg") {
		t.Fatal("setup: the detected files are not left out")
	}

	manifest := func(i int, video string) delta.Manifest {
		return delta.Manifest{Files: map[string]delta.FileEntry{
			"slot1.sav":    {Hash: fmt.Sprint("save", i)},
			"progress.cfg": {Hash: fmt.Sprint("progress", i)},
			"video.cfg":    {Hash: video},
		}}
	}
	d.observeSettings(game, "peerA", manifest(0, "1080p"))
	for i := 1; i <= 2; i++ {
		d.observeSettings(game, "peerA", manifest(i, "1080p"))
	}
	if !excluded("progress.cfg") {
		t.Fatal("given back after two changes: too eager")
	}
	d.observeSettings(game, "peerA", manifest(3, "1080p"))
	if excluded("progress.cfg") {
		t.Error("a file that changed with the save three times out of three is still left out")
	}
	if !excluded("video.cfg") {
		t.Error("a settings file that stayed put was given back")
	}
	v, _ := d.Store.SettingsFiles("g")
	for _, f := range v {
		if f.Path == "progress.cfg" && f.Verdict != store.VerdictSave {
			t.Errorf("verdict %q, want save", f.Verdict)
		}
	}
}
