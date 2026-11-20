package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/opensave/opensave/internal/store"
)

func getOverview(t *testing.T, ts *testServer) map[string]cloudGame {
	t.Helper()
	resp, err := http.Get(ts.base + "/api/cloud/overview")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("overview: status %d", resp.StatusCode)
	}
	var list []cloudGame
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	out := map[string]cloudGame{}
	for _, g := range list {
		out[g.GameID] = g
	}
	return out
}

// The Backups page's facts: which copies this PC knows the reason for, and
// whether the save each game is at has reached the cloud. A copy this PC does
// not hold says so rather than borrowing a kind.
func TestCloudOverview_SaysWhatIsUpAndWhatThisPCKnowsOfIt(t *testing.T) {
	ts := startTestServer(t)
	if err := os.WriteFile(filepath.Join(ts.saveDir, "slot1.sav"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if resp, body := ts.do(t, http.MethodPost, "/api/games", map[string]any{"name": "Shelf Game", "savePath": ts.saveDir}); resp.StatusCode != http.StatusOK {
		t.Fatalf("track: %d %s", resp.StatusCode, body)
	}
	const gameID = "shelf-game"
	if !waitFor(20*time.Second, func() bool {
		snaps, _ := ts.daemon.Store.ListSnapshots(gameID, "main")
		return len(snaps) == 1
	}) {
		t.Fatal("setup: the initial snapshot never landed")
	}
	initial, _ := ts.daemon.Store.ListSnapshots(gameID, "main")
	current := initial[0].ID

	// An older snapshot taken by hand, which this PC holds.
	if err := ts.daemon.Store.CreateSnapshot(store.Snapshot{
		ID: "snap_1000", GameID: gameID, BranchName: "main", Timestamp: "2026-01-01T10:00:00.000Z",
		Comment: "Before the boss", IsSystemAuto: false, ZipPath: filepath.Join(t.TempDir(), "x.zip"),
	}); err != nil {
		t.Fatal(err)
	}

	cloudDir := t.TempDir()
	put := func(name string) {
		if err := os.WriteFile(filepath.Join(cloudDir, name), []byte("zip"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	put(gameID + "__main__snap_1000.zip") // held here
	put(gameID + "__main__snap_2000.zip") // not held here
	put("only-in-cloud__main__snap_3000.zip")
	if err := ts.daemon.Store.UpdateCloudConfig(store.CloudConfig{Enabled: true, Provider: "local", URL: cloudDir, HeadersJSON: "{}"}); err != nil {
		t.Fatal(err)
	}

	games := getOverview(t, ts)
	g, ok := games[gameID]
	if !ok || !g.Tracked || len(g.Copies) != 2 {
		t.Fatalf("tracked game: %+v", g)
	}
	byID := map[string]cloudCopy{}
	for _, c := range g.Copies {
		byID[c.SnapshotID] = c
	}
	if c := byID["snap_1000"]; !c.Known || c.Comment != "Before the boss" || c.IsSystemAuto || c.TakenMs != 1000 {
		t.Errorf("the copy held here: %+v, want known, taken by hand, at 1000", c)
	}
	if c := byID["snap_2000"]; c.Known || c.Comment != "" {
		t.Errorf("the copy not held here: %+v, want it unknown, with no reason given", c)
	}
	if g.Current != current || g.CurrentUploaded {
		t.Errorf("current %q uploaded=%v, want %q not uploaded", g.Current, g.CurrentUploaded, current)
	}
	if g.LocalSnapshots != 2 {
		t.Errorf("local snapshots %d, want 2", g.LocalSnapshots)
	}
	if o, ok := games["only-in-cloud"]; !ok || o.Tracked || len(o.Copies) != 1 {
		t.Errorf("game only in the cloud: %+v", o)
	}

	put(gameID + "__main__" + current + ".zip")
	if g := getOverview(t, ts)[gameID]; !g.CurrentUploaded {
		t.Error("the current save's copy is up, but the game is not shown as backed up")
	}

	// A sync from a paired device writes its snapshot without moving the
	// cloud record. That snapshot is the save now, and it is not up.
	later := time.Now().Add(time.Minute).UTC()
	if err := ts.daemon.Store.CreateSnapshot(store.Snapshot{
		ID: "snap_mirror", GameID: gameID, BranchName: "main", Timestamp: later.Format("2006-01-02T15:04:05.000Z"),
		Comment: "Synced from peer: Deck (Auto backup)", IsSystemAuto: true, ZipPath: filepath.Join(t.TempDir(), "m.zip"),
	}); err != nil {
		t.Fatal(err)
	}
	if g := getOverview(t, ts)[gameID]; g.Current != "snap_mirror" || g.CurrentUploaded {
		t.Errorf("after a sync from a peer: current %q uploaded=%v, want snap_mirror, not up", g.Current, g.CurrentUploaded)
	}

	// A restore after that moves the record on, and the restored save is
	// the one that counts.
	head, _, _ := ts.daemon.Store.GetCloudHead(gameID)
	head.Snapshot, head.Since = "snap_1000", later.Add(time.Minute).Format("2006-01-02T15:04:05.000Z")
	if err := ts.daemon.Store.SetCloudHead(head); err != nil {
		t.Fatal(err)
	}
	if g := getOverview(t, ts)[gameID]; g.Current != "snap_1000" || !g.CurrentUploaded {
		t.Errorf("after a restore: current %q uploaded=%v, want snap_1000, up", g.Current, g.CurrentUploaded)
	}
}
