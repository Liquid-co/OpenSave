// Package e2e runs two full in-process daemons against each other over
// real loopback HTTP — the Go equivalent of test-comprehensive-e2e.js and
// test-lan-pairing.js.
package e2e

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/opensave/opensave/testutil"
)

func TestPairingHandshake(t *testing.T) {
	a := testutil.NewTestDaemon(t, "Device-A")
	b := testutil.NewTestDaemon(t, "Device-B")

	a.PairWith(b)

	peerOnA, err := a.Daemon.Store.GetPeer(b.NodeID())
	if err != nil {
		t.Fatal(err)
	}
	if peerOnA.Name != "Device-B" {
		t.Errorf("peer name on A = %q, want Device-B", peerOnA.Name)
	}
	peerOnB, err := b.Daemon.Store.GetPeer(a.NodeID())
	if err != nil {
		t.Fatal(err)
	}
	if peerOnB.Name != "Device-A" {
		t.Errorf("peer name on B = %q, want Device-A", peerOnB.Name)
	}
}

func TestUnsolicitedApproveConfirmRejected(t *testing.T) {
	a := testutil.NewTestDaemon(t, "Device-A")

	// A spoofer posts approve-confirm without any prior handshake.
	req, err := http.NewRequest(http.MethodPost, "http://"+a.Addr+"/api/p2p/approve-confirm",
		jsonBody(`{"peerId":"node_spoofer","deviceName":"Evil","deviceType":"desktop","port":9}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("unsolicited approve-confirm status = %d, want 400", resp.StatusCode)
	}
	if _, err := a.Daemon.Store.GetPeer("node_spoofer"); err == nil {
		t.Error("spoofed peer must not be persisted")
	}
}

func TestFullSyncFlow(t *testing.T) {
	a := testutil.NewTestDaemon(t, "Device-A")
	b := testutil.NewTestDaemon(t, "Device-B")
	a.PairWith(b)

	// A tracks a game with save data; B doesn't know it yet.
	a.WriteSave("slot1.sav", "hello from A")
	a.WriteSave("config/video.ini", "fullscreen=1")
	gameID := a.TrackGame("Shared Game")

	// A pushes: manifest request auto-tracks on B, then B is triggered to
	// pull. Point B's auto-tracked game at B's own save dir via path
	// translation being identity here — B auto-tracks at A's path, which
	// exists on this same machine. To keep the test honest we instead
	// pre-track the game on B pointing at B's save dir.
	b.API(http.MethodPost, "/api/games", map[string]string{"name": "Shared Game", "savePath": b.SaveDir}, nil)

	a.API(http.MethodPost, "/api/games/"+gameID+"/sync", nil, nil)

	if !testutil.WaitFor(30*time.Second, func() bool {
		return b.ReadSave("slot1.sav") == "hello from A" && b.ReadSave("config/video.ini") == "fullscreen=1"
	}) {
		t.Fatalf("B never received A's files; got slot1=%q", b.ReadSave("slot1.sav"))
	}

	// Now B makes newer progress and syncs: A must receive it.
	time.Sleep(1100 * time.Millisecond) // ensure a later mtime second boundary
	b.WriteSave("slot1.sav", "B made progress")
	b.API(http.MethodPost, "/api/games/"+gameID+"/sync", nil, nil)

	if !testutil.WaitFor(30*time.Second, func() bool {
		return a.ReadSave("slot1.sav") == "B made progress"
	}) {
		t.Fatalf("A never received B's update; got %q", a.ReadSave("slot1.sav"))
	}

	// Deletion propagation: A deletes the config file and syncs.
	if err := os.Remove(filepath.Join(a.SaveDir, "config", "video.ini")); err != nil {
		t.Fatal(err)
	}
	a.API(http.MethodPost, "/api/games/"+gameID+"/sync", nil, nil)

	if !testutil.WaitFor(30*time.Second, func() bool {
		return b.ReadSave("config/video.ini") == ""
	}) {
		t.Fatal("deletion never propagated to B")
	}
}

func TestConflictFlowOverHTTP(t *testing.T) {
	a := testutil.NewTestDaemon(t, "Device-A")
	b := testutil.NewTestDaemon(t, "Device-B")
	a.PairWith(b)

	// Both sides start in sync.
	a.WriteSave("slot1.sav", "shared start")
	gameID := a.TrackGame("Conflict Game")
	b.API(http.MethodPost, "/api/games", map[string]string{"name": "Conflict Game", "savePath": b.SaveDir}, nil)
	a.API(http.MethodPost, "/api/games/"+gameID+"/sync", nil, nil)
	if !testutil.WaitFor(30*time.Second, func() bool {
		return b.ReadSave("slot1.sav") == "shared start"
	}) {
		t.Fatal("initial sync failed")
	}

	// Wait out the mtime-vs-lastSynced skew window, then both sides
	// diverge.
	time.Sleep(2500 * time.Millisecond)
	a.WriteSave("slot1.sav", "A's version")
	b.WriteSave("slot1.sav", "B's version")

	var syncResp struct {
		Results map[string]struct {
			Status string `json:"status"`
		} `json:"results"`
	}
	a.API(http.MethodPost, "/api/games/"+gameID+"/sync", nil, &syncResp)
	res, ok := syncResp.Results[b.NodeID()]
	if !ok || res.Status != "conflict" {
		t.Fatalf("expected conflict result, got %+v", syncResp.Results)
	}

	// Resolve keep-remote on A: A adopts B's version.
	a.API(http.MethodPost, "/api/games/"+gameID+"/resolve-conflict", map[string]string{
		"peerId": b.NodeID(), "resolution": "keep-remote",
	}, nil)

	if got := a.ReadSave("slot1.sav"); got != "B's version" {
		t.Errorf("after keep-remote, A has %q, want B's version", got)
	}
}

func jsonBody(s string) *os.File {
	f, _ := os.CreateTemp("", "body-*.json")
	f.WriteString(s)
	f.Seek(0, 0)
	return f
}
