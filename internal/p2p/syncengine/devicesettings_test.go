package syncengine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/opensave/opensave/internal/delta"
	"github.com/opensave/opensave/internal/ignore"
	"github.com/opensave/opensave/internal/store"
)

func withDeviceSettings(env *engineEnv, patterns ...string) {
	env.engine.DeviceSettingsFor = func(store.Game) []string { return patterns }
}

// Each device keeps its own graphics settings: a differing settings file is
// not a difference, in either direction — until the game is set to sync them.
func TestDeviceSettings_KeptPerDevice(t *testing.T) {
	env := setupEngine(t)
	withDeviceSettings(env, "/graphics.xml")
	write(t, env.localDir, "slot1.sav", "progress")
	write(t, env.localDir, "graphics.xml", "4k ultra")
	write(t, env.remoteDir, "slot1.sav", "progress")
	write(t, env.remoteDir, "graphics.xml", "720p low")

	res, err := env.engine.SyncWithPeer(context.Background(), "game1", env.peer)
	if err != nil || res.Status != "in_sync" {
		t.Fatalf("status = %q, %v; want in_sync", res.Status, err)
	}
	if got, _ := os.ReadFile(filepath.Join(env.localDir, "graphics.xml")); string(got) != "4k ultra" {
		t.Errorf("this device's settings became %q", got)
	}
	if got, _ := os.ReadFile(filepath.Join(env.remoteDir, "graphics.xml")); string(got) != "720p low" {
		t.Errorf("the peer's settings became %q", got)
	}

	// A save change still travels; the settings beside it still do not.
	write(t, env.remoteDir, "slot1.sav", "more progress")
	res, err = env.engine.SyncWithPeer(context.Background(), "game1", env.peer)
	if err != nil || res.Status != "updated" {
		t.Fatalf("status = %q, %v; want updated", res.Status, err)
	}
	if got, _ := os.ReadFile(filepath.Join(env.localDir, "slot1.sav")); string(got) != "more progress" {
		t.Errorf("the save did not arrive: %q", got)
	}
	if got, _ := os.ReadFile(filepath.Join(env.localDir, "graphics.xml")); string(got) != "4k ultra" {
		t.Errorf("this device's settings became %q", got)
	}

	game, _ := env.store.GetGame("game1")
	game.SyncDeviceSettings = true
	if env.engine.IgnoreText(game) != game.SyncIgnore {
		t.Error("set to sync its settings, the game still leaves them out")
	}
}

// The game's own rules come after the settings, so "!" brings one back.
func TestDeviceSettings_OwnRuleCanBringOneBack(t *testing.T) {
	env := setupEngine(t)
	withDeviceSettings(env, "/graphics.xml", "/input.cfg")
	game, _ := env.store.GetGame("game1")
	game.SyncIgnore = "!input.cfg"
	rules := env.engine.IgnoreText(game)
	if r := ignore.Parse(rules); !r.Match("graphics.xml") || r.Match("input.cfg") {
		t.Errorf("rules %q: graphics.xml excluded %v, input.cfg excluded %v", rules, r.Match("graphics.xml"), r.Match("input.cfg"))
	}
}

// Leaving the settings out where they were synced before is not a change of
// the save: the recorded hash is re-taken, no version is named.
func TestDeviceSettings_StartingToLeaveThemOutIsNotAVersion(t *testing.T) {
	env := setupEngine(t)
	game, _ := env.store.GetGame("game1")
	game.AutoSync = true
	_ = env.store.UpdateGame(game)
	write(t, env.localDir, "slot1.sav", "progress")
	write(t, env.localDir, "graphics.xml", "4k ultra")
	env.engine.NoteLocalChange("game1")
	before, _ := env.store.GetGameVersion("game1")

	withDeviceSettings(env, "/graphics.xml")
	m, _ := delta.BuildManifest(env.localDir)
	env.engine.AdoptExclusionView("game1", m, "", false)
	env.engine.NoteLocalChange("game1")
	after, _ := env.store.GetGameVersion("game1")
	if after.Vector != before.Vector {
		t.Fatalf("version moved from %s to %s", before.Vector, after.Vector)
	}
	if after.Hash != env.engine.versionHashOf("game1", m) {
		t.Error("the recorded hash was not re-taken")
	}
}
