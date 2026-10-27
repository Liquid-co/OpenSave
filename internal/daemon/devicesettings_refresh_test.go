package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opensave/opensave/internal/delta"
	"github.com/opensave/opensave/internal/store"
	"github.com/opensave/opensave/internal/watcher"
)

// A fresh install reads the game database before the catalogue download is
// indexed, and the one built into the app names no settings. Once the download
// is indexed, the settings it names are left out without a restart, and the
// last snapshot's hash is carried over, so the watcher sees no change.
func TestRefreshDeviceSettings_TakesTheCatalogueOnceIndexed(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	d := newTestDaemon(t)
	d.Scanner.HomeDir = home

	dir := filepath.Join(home, "RefreshGame")
	if err := os.MkdirAll(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{"slot1.sav": "a", "settings.ini": "4k"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o666); err != nil {
			t.Fatal(err)
		}
	}
	g := store.Game{ID: "refresh", Name: "Refresh Game", SavePath: dir, ActiveBranch: "main", MaxSnapshots: 5}
	if err := d.Store.CreateGame(g); err != nil {
		t.Fatal(err)
	}
	m, err := delta.BuildManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Store.SetLastManifestHash(g.ID, watcher.ContentHash(m, "")); err != nil {
		t.Fatal(err)
	}
	g, _ = d.Store.GetGame(g.ID)
	if got := d.IgnoreText(g); got != g.SyncIgnore {
		t.Fatalf("settings left out before any catalogue: %q", got)
	}

	// The catalogue arrives and is indexed.
	cache := filepath.Dir(d.Scanner.CacheFile)
	if err := os.MkdirAll(cache, 0o777); err != nil {
		t.Fatal(err)
	}
	manifest := "Refresh Game:\n" +
		"  files:\n" +
		"    \"<home>/RefreshGame/settings.ini\":\n" +
		"      tags: [config]\n" +
		"    \"<home>/RefreshGame/*.sav\":\n" +
		"      tags: [save]\n"
	if err := os.WriteFile(filepath.Join(cache, "ludusavi-manifest.yaml"), []byte(manifest), 0o666); err != nil {
		t.Fatal(err)
	}
	if d.Scanner.DeviceSettings() == nil {
		t.Fatal("the catalogue was not indexed")
	}

	d.refreshDeviceSettings()

	if got := d.IgnoreText(g); !strings.Contains(got, "/settings.ini") {
		t.Fatalf("settings still synced after the catalogue was indexed: exclusions %q", got)
	}
	g, _ = d.Store.GetGame(g.ID)
	if g.LastManifestHash != watcher.ContentHash(m, d.IgnoreText(g)) {
		t.Error("the last snapshot's hash was not carried over: the watcher would snapshot a game nobody played")
	}
}
