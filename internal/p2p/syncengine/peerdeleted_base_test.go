package syncengine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/opensave/opensave/internal/delta"
)

// The other device deleted one file and changed another, and this device has
// made the deletion it asked for. Nothing else changed here, so the other
// device's edit is taken — even when this device's untouched copy of the
// changed file carries the later date, as a restored or touched file does, or
// a clock that runs ahead.
//
// The conflict check already knew this side had not moved (peerdeleted.go);
// the direction did not, because the save here no longer hashes to the agreed
// base and neither does the other device's. It fell back to dates and sent
// the untouched copy over the edit.
func TestPeerDeletion_AgreedBaseStillDecidesDirection(t *testing.T) {
	env := setupEngine(t)
	write(t, env.localDir, "gone.sav", "deleted over there")
	write(t, env.localDir, "slot1.sav", "start")
	write(t, env.remoteDir, "gone.sav", "deleted over there")
	write(t, env.remoteDir, "slot1.sav", "start")
	if res, err := env.engine.SyncWithPeer(context.Background(), "game1", env.peer); err != nil || res.Status != "in_sync" {
		t.Fatalf("setup: %+v %v", res, err)
	}
	base, err := delta.BuildManifest(env.localDir)
	if err != nil {
		t.Fatal(err)
	}
	if got := env.store.GetAgreedHash("game1", env.peer.ID); got != base.ManifestHash() {
		t.Fatalf("setup: agreed %q, want %q", got, base.ManifestHash())
	}

	// The other device: gone.sav deleted, slot1.sav played on, an hour ago.
	_ = os.Remove(filepath.Join(env.remoteDir, "gone.sav"))
	write(t, env.remoteDir, "slot1.sav", "progress")
	hourAgo := time.Now().Add(-time.Hour)
	if err := os.Chtimes(filepath.Join(env.remoteDir, "slot1.sav"), hourAgo, hourAgo); err != nil {
		t.Fatal(err)
	}
	// This device: the deletion it was asked to make, and slot1.sav as it
	// was, dated now.
	env.engine.NotePeerDeletion("game1", "", "gone.sav", base.Files["gone.sav"], false)
	if err := os.Remove(filepath.Join(env.localDir, "gone.sav")); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := os.Chtimes(filepath.Join(env.localDir, "slot1.sav"), now, now); err != nil {
		t.Fatal(err)
	}

	res, err := env.engine.SyncWithPeer(context.Background(), "game1", env.peer)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status == "conflict" {
		t.Fatalf("only the other device changed: %+v", res)
	}
	if got, _ := os.ReadFile(filepath.Join(env.localDir, "slot1.sav")); string(got) != "progress" {
		t.Errorf("local slot1.sav = %q, want the other device's edit", got)
	}
	if got, _ := os.ReadFile(filepath.Join(env.remoteDir, "slot1.sav")); string(got) != "progress" {
		t.Errorf("the other device's edit was overwritten with %q", got)
	}
	if env.transport.pullTriggers != 0 {
		t.Errorf("this device asked the other to take its unchanged copy (%d triggers)", env.transport.pullTriggers)
	}
}
