package e2e

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/opensave/opensave/internal/p2p/syncengine"
	"github.com/opensave/opensave/testutil"
)

// An isolated transport interposer for fault injection after a conflict is set.
type faultOnAdoptionTransport struct {
	syncengine.Transport
	cancel                     context.CancelFunc
	fetchBlockCalls            int
	manifestCalls              int
	simulateManifestChange     bool
	removeGameAtSecondManifest func() error
	completeEvents             int
}

func (f *faultOnAdoptionTransport) FetchBlocks(ctx context.Context, peer syncengine.Peer, ref syncengine.FileRef, indices []int, blockSize int) ([]syncengine.BlockData, error) {
	if f.cancel != nil {
		f.fetchBlockCalls++
		f.cancel()
		return nil, context.Canceled
	}
	return f.Transport.FetchBlocks(ctx, peer, ref, indices, blockSize)
}

func (f *faultOnAdoptionTransport) FetchManifest(ctx context.Context, peer syncengine.Peer, gameID string, q syncengine.ManifestQuery) (syncengine.ManifestResponse, error) {
	resp, err := f.Transport.FetchManifest(ctx, peer, gameID, q)
	if err != nil {
		return resp, err
	}
	f.manifestCalls++
	if f.manifestCalls == 2 && f.removeGameAtSecondManifest != nil {
		if err := f.removeGameAtSecondManifest(); err != nil {
			return resp, err
		}
	}
	if f.simulateManifestChange && f.manifestCalls == 2 {
		root, ok := resp.Manifest.Extra["config"]
		if !ok {
			return resp, errors.New("missing config")
		}
		entry := root.Files["settings.ini"]
		entry.Hash = "synthetic-new-peer-commit"
		root.Files["settings.ini"] = entry
		resp.Manifest.Extra["config"] = root
	}
	return resp, nil
}

func (f *faultOnAdoptionTransport) ReportSyncEvent(peer syncengine.Peer, gameID, event string, data map[string]any) {
	if event == "sync-complete" {
		f.completeEvents++
	}
	f.Transport.ReportSyncEvent(peer, gameID, event, data)
}

func TestKeepRemoteInterruptedBlockFetchKeepsConflictAndLineage(t *testing.T) {
	a, b, gameID, aConfig, bConfig := twoLocationPair(t, "KeepRemoteInterruptPull", map[string]string{"settings.ini": "BASE"})
	pauseAutoSync(t, a, b, gameID)
	writeIn(t, aConfig, "settings.ini", "LOCAL")
	writeIn(t, bConfig, "settings.ini", "REMOTE")
	time.Sleep(syncSettleWindow)
	syncTo(a, gameID, b.NodeID())
	if !testutil.WaitFor(30*time.Second, func() bool { return len(a.Daemon.P2P.Sync.ActiveRootConflicts()) > 0 }) {
		t.Fatal("expected conflict")
	}
	base := a.Daemon.Store.GetAgreedHashForRoot(gameID, b.NodeID(), "config")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	original := a.Daemon.P2P.Sync.Transport
	fault := &faultOnAdoptionTransport{Transport: original, cancel: cancel}
	a.Daemon.P2P.Sync.Transport = fault
	defer func() { a.Daemon.P2P.Sync.Transport = original }()
	err := a.Daemon.P2P.Sync.ResolveRootConflict(ctx, gameID, b.NodeID(), "config", "keep-remote")
	if err == nil {
		t.Fatal("cancelled fetch must fail")
	}
	if fault.fetchBlockCalls == 0 {
		t.Fatal("fault was never injected")
	}
	if len(a.Daemon.P2P.Sync.ActiveRootConflicts()) == 0 {
		t.Fatal("conflict cleared on failure")
	}
	if got := a.Daemon.Store.GetAgreedHashForRoot(gameID, b.NodeID(), "config"); got != base {
		t.Fatalf("lineage agreement changed: %s != %s", got, base)
	}
	if fault.completeEvents > 0 {
		t.Fatal("sync-complete before verified resolution")
	}
	if got := readIn(bConfig, "settings.ini"); got != "REMOTE" {
		t.Fatalf("remote overwritten: %q", got)
	}
}

func TestKeepRemotePeerChangedAtFinalManifestKeepsConflict(t *testing.T) {
	a, b, gameID, aConfig, bConfig := twoLocationPair(t, "KeepRemotePeerChange", map[string]string{"settings.ini": "BASE"})
	pauseAutoSync(t, a, b, gameID)
	writeIn(t, aConfig, "settings.ini", "LOCAL")
	writeIn(t, bConfig, "settings.ini", "REMOTE")
	time.Sleep(syncSettleWindow)
	syncTo(a, gameID, b.NodeID())
	if !testutil.WaitFor(30*time.Second, func() bool { return len(a.Daemon.P2P.Sync.ActiveRootConflicts()) > 0 }) {
		t.Fatal("expected conflict")
	}
	base := a.Daemon.Store.GetAgreedHashForRoot(gameID, b.NodeID(), "config")
	original := a.Daemon.P2P.Sync.Transport
	fault := &faultOnAdoptionTransport{Transport: original, simulateManifestChange: true}
	a.Daemon.P2P.Sync.Transport = fault
	defer func() { a.Daemon.P2P.Sync.Transport = original }()
	err := a.Daemon.P2P.Sync.ResolveRootConflict(context.Background(), gameID, b.NodeID(), "config", "keep-remote")
	if err == nil {
		t.Fatal("changed peer manifest should fail")
	}
	if fault.manifestCalls != 2 {
		t.Fatalf("expected two manifest reads, got %d", fault.manifestCalls)
	}
	if len(a.Daemon.P2P.Sync.ActiveRootConflicts()) == 0 {
		t.Fatal("conflict cleared despite peer state change")
	}
	if got := a.Daemon.Store.GetAgreedHashForRoot(gameID, b.NodeID(), "config"); got != base {
		t.Fatalf("agreement was persisted on error: %s != %s", got, base)
	}
	if fault.completeEvents > 0 {
		t.Fatal("sync-complete emitted on failure")
	}
	if got := readIn(bConfig, "settings.ini"); got != "REMOTE" {
		t.Fatalf("selected peer unexpectedly overwritten: %q", got)
	}
}

// Remove a disposable game after both manifests have been fetched, forcing
// the atomic resolution metadata write to refuse the now-untracked game.
func TestKeepRemoteMetadataPersistenceFailureKeepsConflict(t *testing.T) {
	a, b, gameID, aConfig, bConfig := twoLocationPair(t, "KeepRemoteMetadataFail", map[string]string{"settings.ini": "BASE"})
	pauseAutoSync(t, a, b, gameID)
	writeIn(t, aConfig, "settings.ini", "LOCAL")
	writeIn(t, bConfig, "settings.ini", "REMOTE")
	time.Sleep(syncSettleWindow)
	syncTo(a, gameID, b.NodeID())
	if !testutil.WaitFor(30*time.Second, func() bool { return len(a.Daemon.P2P.Sync.ActiveRootConflicts()) > 0 }) {
		t.Fatal("expected conflict")
	}
	original := a.Daemon.P2P.Sync.Transport
	fault := &faultOnAdoptionTransport{Transport: original, removeGameAtSecondManifest: func() error { return a.Daemon.Store.DeleteGame(gameID) }}
	a.Daemon.P2P.Sync.Transport = fault
	defer func() { a.Daemon.P2P.Sync.Transport = original }()
	err := a.Daemon.P2P.Sync.ResolveRootConflict(context.Background(), gameID, b.NodeID(), "config", "keep-remote")
	if err == nil {
		t.Fatal("metadata persistence after game removal must fail")
	}
	if fault.manifestCalls != 2 {
		t.Fatalf("expected 2 manifest reads, got %d", fault.manifestCalls)
	}
	if fault.completeEvents != 0 {
		t.Fatal("sync-complete emitted despite failure")
	}
	if len(a.Daemon.P2P.Sync.ActiveRootConflicts()) == 0 {
		t.Fatal("conflict cleared on metadata failure")
	}
	if got := readIn(bConfig, "settings.ini"); got != "REMOTE" {
		t.Fatalf("peer modified: %q", got)
	}
}
