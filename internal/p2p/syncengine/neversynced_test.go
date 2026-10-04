package syncengine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/opensave/opensave/internal/delta"
)

// Steam's remotecache.vdf is each device's own bookkeeping: never taken from
// a peer, never pushed, never deleted because the peer lacks it.
func TestNeverSynced_LeftAloneOnBothSides(t *testing.T) {
	env := setupEngine(t)
	write(t, env.localDir, "save.dat", "progress")
	write(t, env.localDir, "remotecache.vdf", "this device's steam")
	write(t, env.remoteDir, "save.dat", "progress")
	write(t, env.remoteDir, "remotecache.vdf", "the other device's steam")

	res, err := env.engine.SyncWithPeer(context.Background(), "game1", env.peer)
	if err != nil {
		t.Fatalf("SyncWithPeer error = %v", err)
	}
	if res.Status != "in_sync" {
		t.Fatalf("status = %q, want in_sync: the saves are the same", res.Status)
	}
	got, _ := os.ReadFile(filepath.Join(env.localDir, "remotecache.vdf"))
	if string(got) != "this device's steam" {
		t.Errorf("this device's remotecache.vdf became %q", got)
	}
	if got, _ := os.ReadFile(filepath.Join(env.remoteDir, "remotecache.vdf")); string(got) != "the other device's steam" {
		t.Errorf("the peer's remotecache.vdf became %q", got)
	}
}

// The served manifest keeps listing it: a device on an older build would read
// the gap as the file deleted, and delete its own.
func TestNeverSynced_StillServed(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "remotecache.vdf", "steam")
	m, err := delta.BuildManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m.Files["remotecache.vdf"]; !ok {
		t.Fatal("the manifest a device serves left remotecache.vdf out")
	}
}

// A build with a new list of such files re-takes the recorded hash quietly
// when the save is the one it was recorded for, and the next change seen is
// not called a new version over it. A save that did change is left to count.
func TestNeverSynced_AdoptingTheViewIsNotAVersion(t *testing.T) {
	env := setupEngine(t)
	game, err := env.store.GetGame("game1")
	if err != nil {
		t.Fatal(err)
	}
	game.AutoSync = true
	if err := env.store.UpdateGame(game); err != nil {
		t.Fatal(err)
	}
	write(t, env.localDir, "save.dat", "progress")
	write(t, env.localDir, "remotecache.vdf", "steam")
	env.engine.NoteLocalChange("game1") // the first version: named by content

	m, err := delta.BuildManifest(env.localDir)
	if err != nil {
		t.Fatal(err)
	}
	// As a build before the list recorded it.
	rec, err := env.store.GetGameVersion("game1")
	if err != nil || rec.Vector == "" {
		t.Fatalf("no version recorded: %v", err)
	}
	vector := rec.Vector
	rec.Hash = versionHashBeforeNeverSynced("", m)
	if err := env.store.SaveGameVersion(rec); err != nil {
		t.Fatal(err)
	}

	env.engine.AdoptNeverSyncedView("game1", m)
	env.engine.NoteLocalChange("game1")
	rec, _ = env.store.GetGameVersion("game1")
	if rec.Vector != vector {
		t.Fatalf("version moved from %s to %s over a save nobody changed", vector, rec.Vector)
	}
	if rec.Hash != env.engine.versionHashOf("game1", m) {
		t.Error("the recorded hash was not re-taken")
	}

	// Recorded for a save that has since changed: not adopted, so the change
	// is a version.
	write(t, env.localDir, "save.dat", "more progress")
	delta.InvalidateRoot(env.localDir)
	changed, _ := delta.BuildManifest(env.localDir)
	rec.Hash = versionHashBeforeNeverSynced("", m)
	_ = env.store.SaveGameVersion(rec)
	env.engine.AdoptNeverSyncedView("game1", changed)
	env.engine.NoteLocalChange("game1")
	rec, _ = env.store.GetGameVersion("game1")
	if rec.Vector == vector {
		t.Error("a real change after the upgrade did not become a version")
	}
}

// Held only by the peer: not taken. Held only here: not pushed, and not
// deleted because the peer lacks it.
func TestNeverSynced_OneSidedIsNotADifference(t *testing.T) {
	env := setupEngine(t)
	write(t, env.localDir, "save.dat", "progress")
	write(t, env.remoteDir, "save.dat", "progress")
	write(t, env.remoteDir, "RemoteCache.VDF", "only there")

	res, err := env.engine.SyncWithPeer(context.Background(), "game1", env.peer)
	if err != nil || res.Status != "in_sync" {
		t.Fatalf("status = %q, %v; want in_sync", res.Status, err)
	}
	if _, err := os.Stat(filepath.Join(env.localDir, "RemoteCache.VDF")); err == nil {
		t.Error("the peer's remotecache.vdf was pulled")
	}

	os.Remove(filepath.Join(env.remoteDir, "RemoteCache.VDF"))
	write(t, env.localDir, "remotecache.vdf", "only here")
	delta.InvalidateRoot(env.localDir)
	res, err = env.engine.SyncWithPeer(context.Background(), "game1", env.peer)
	if err != nil || res.Status != "in_sync" {
		t.Fatalf("status = %q, %v; want in_sync", res.Status, err)
	}
	if _, err := os.Stat(filepath.Join(env.remoteDir, "remotecache.vdf")); err == nil {
		t.Error("this device's remotecache.vdf was pushed")
	}
	if _, err := os.Stat(filepath.Join(env.localDir, "remotecache.vdf")); err != nil {
		t.Error("this device's remotecache.vdf was deleted")
	}
}
