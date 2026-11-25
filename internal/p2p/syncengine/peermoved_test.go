package syncengine

import (
	"context"
	"testing"

	"github.com/opensave/opensave/internal/delta"
)

// The lineage says both devices held b.sav and the peer's manifest, fetched
// before that was recorded, does not have it: read in that order it looks
// like the peer deleted b.sav. The peer had in fact only just taken it from
// here, and holds it when asked again. Nothing is deleted on that reading;
// the sync stops and is run again.
func TestPeerMoved_AFileThePeerHoldsAgainIsNotDeletedHere(t *testing.T) {
	env := setupEngine(t)
	write(t, env.localDir, "a.sav", "alpha")
	write(t, env.localDir, "b.sav", "bravo")
	write(t, env.remoteDir, "a.sav", "alpha")
	both, _ := delta.BuildManifest(env.localDir)
	// What the peer's report recorded while this sync was running.
	if err := env.store.SetSyncState("game1", env.peer.ID, []string{"a.sav", "b.sav"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := env.store.SetAgreedHash("game1", env.peer.ID, both.ManifestHash()); err != nil {
		t.Fatal(err)
	}
	calls := 0
	env.transport.onFetchManifest = func() {
		calls++
		if calls == 2 { // asked again: by now it holds what it took from here
			write(t, env.remoteDir, "b.sav", "bravo")
		}
	}

	res, err := env.engine.SyncWithPeer(context.Background(), "game1", env.peer)
	if err != nil {
		t.Fatal(err)
	}
	if got := readSaveFile(t, env.localDir, "b.sav"); got != "bravo" {
		t.Fatalf("b.sav here became %q: a file the peer holds was deleted as one it had deleted", got)
	}
	if res.Status != "peer_busy" {
		t.Errorf("status %q, want peer_busy so the sync is run again", res.Status)
	}
}

// A file the peer really deleted is still deleted here: asked again, it
// still does not have it.
func TestPeerMoved_AFileThePeerDeletedIsStillDeletedHere(t *testing.T) {
	env := setupEngine(t)
	write(t, env.localDir, "a.sav", "alpha")
	write(t, env.localDir, "b.sav", "bravo")
	write(t, env.remoteDir, "a.sav", "alpha")
	both, _ := delta.BuildManifest(env.localDir)
	if err := env.store.SetSyncState("game1", env.peer.ID, []string{"a.sav", "b.sav"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := env.store.SetAgreedHash("game1", env.peer.ID, both.ManifestHash()); err != nil {
		t.Fatal(err)
	}

	if _, err := env.engine.SyncWithPeer(context.Background(), "game1", env.peer); err != nil {
		t.Fatal(err)
	}
	if got := readSaveFile(t, env.localDir, "b.sav"); got != "" {
		t.Errorf("b.sav here is %q, want it deleted as the peer deleted it", got)
	}
}
