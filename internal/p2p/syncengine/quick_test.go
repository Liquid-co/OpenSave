package syncengine

import (
	"context"
	"testing"

	"github.com/opensave/opensave/internal/delta"
	"github.com/opensave/opensave/internal/snapshot"
)

// ifHashTransport answers IfHash the way a current peer does, and counts
// how many full manifests it had to send.
type ifHashTransport struct {
	*fakeTransport
	full, unchanged int
}

func (t *ifHashTransport) FetchManifest(ctx context.Context, peer Peer, gameID string, q ManifestQuery) (ManifestResponse, error) {
	resp, err := t.fakeTransport.FetchManifest(ctx, peer, gameID, q)
	if err != nil {
		return resp, err
	}
	if q.IfHash != "" && resp.Manifest.ManifestHash() == q.IfHash {
		t.unchanged++
		return ManifestResponse{ActiveBranch: resp.ActiveBranch, LatestSnapshot: resp.LatestSnapshot,
			Unchanged: true, ManifestHash: q.IfHash, Manifest: delta.Manifest{}}, nil
	}
	t.full++
	return resp, nil
}

func setupIfHashEngine(t *testing.T) (*engineEnv, *ifHashTransport) {
	t.Helper()
	env := setupEngine(t)
	tr := &ifHashTransport{fakeTransport: env.transport}
	env.engine = New(env.store, snapshot.New(env.store), tr)
	return env, tr
}

func TestQuickInSync_SkipsTheManifestWhenNothingChanged(t *testing.T) {
	env, tr := setupIfHashEngine(t)
	write(t, env.localDir, "save.dat", "same")
	write(t, env.remoteDir, "save.dat", "same")

	// First sync converges and records the agreed base.
	if res, err := env.engine.SyncWithPeer(context.Background(), "game1", env.peer); err != nil || res.Status != "in_sync" {
		t.Fatalf("first sync = %+v, %v", res, err)
	}
	if tr.full != 1 {
		t.Fatalf("setup: full manifests = %d, want 1", tr.full)
	}

	// Nothing changed anywhere: settled without a full manifest.
	res, err := env.engine.SyncWithPeer(context.Background(), "game1", env.peer)
	if err != nil || res.Status != "in_sync" {
		t.Fatalf("second sync = %+v, %v", res, err)
	}
	if tr.full != 1 || tr.unchanged != 1 {
		t.Errorf("second sync fetched %d full manifests and %d unchanged answers; want 0 more full, 1 unchanged", tr.full-1, tr.unchanged)
	}
}

// A change on either side must go the full way — the shortcut may only ever
// say "in sync" when both sides provably hold the agreed state.
func TestQuickInSync_ChangesStillSync(t *testing.T) {
	env, tr := setupIfHashEngine(t)
	write(t, env.localDir, "save.dat", "same")
	write(t, env.remoteDir, "save.dat", "same")
	if _, err := env.engine.SyncWithPeer(context.Background(), "game1", env.peer); err != nil {
		t.Fatal(err)
	}

	// The peer changes.
	write(t, env.remoteDir, "new.dat", "made on the other device")
	res, err := env.engine.SyncWithPeer(context.Background(), "game1", env.peer)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "updated" || res.Direction != "pull" {
		t.Fatalf("peer change: result = %+v, want updated/pull", res)
	}

	// This side changes.
	write(t, env.localDir, "mine.dat", "made here")
	before := env.transport.pullTriggers
	res, err = env.engine.SyncWithPeer(context.Background(), "game1", env.peer)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status == "in_sync" {
		t.Fatalf("local change was reported in sync: %+v", res)
	}
	if env.transport.pullTriggers == before {
		t.Error("the peer was not told to pull this side's change")
	}
	if tr.unchanged != 0 {
		t.Errorf("unchanged answers = %d with a change on one side, want 0", tr.unchanged)
	}
}

// A peer that predates the question sends the full manifest; it is used,
// not fetched a second time.
func TestQuickInSync_OldPeerManifestIsNotFetchedTwice(t *testing.T) {
	env := setupEngine(t) // plain fake: ignores IfHash
	write(t, env.localDir, "save.dat", "same")
	write(t, env.remoteDir, "save.dat", "same")
	if _, err := env.engine.SyncWithPeer(context.Background(), "game1", env.peer); err != nil {
		t.Fatal(err)
	}
	before := env.transport.manifestCalls
	if _, err := env.engine.SyncWithPeer(context.Background(), "game1", env.peer); err != nil {
		t.Fatal(err)
	}
	if got := env.transport.manifestCalls - before; got != 1 {
		t.Errorf("manifest requests for one sync = %d, want 1", got)
	}
}
