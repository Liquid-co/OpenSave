package daemon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/opensave/opensave/internal/delta"
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

	d.devMu.Lock()
	d.dev = devSettingsCache{loaded: true, patterns: map[string][]string{}}
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
	// Again at the next start: nothing more to carry over, and nothing lost.
	d.adoptExclusionView([]store.Game{mustGame(t, d, "same")})
	if g, _ := d.Store.GetGame("same"); g.LastManifestHash != hashNow(g) {
		t.Error("a second start lost the carried-over hash")
	}
}

func mustGame(t *testing.T, d *Daemon, id string) store.Game {
	t.Helper()
	g, err := d.Store.GetGame(id)
	if err != nil {
		t.Fatal(err)
	}
	return g
}
