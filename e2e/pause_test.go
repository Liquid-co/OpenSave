package e2e

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/opensave/opensave/testutil"
)

// Pausing syncing, between two real daemons.
//
// A paused device must neither fetch a save nor let one be fetched from it,
// the device asking must treat that as "later" rather than as a failed sync,
// and resuming must bring both sides level without anyone asking.

// pairedTracking pairs two LAN daemons and has both track one game, synced.
func pairedTracking(t *testing.T, name string) (a, b *testutil.TestDaemon, gameID string) {
	t.Helper()
	a = testutil.NewTestDaemon(t, name+"-A")
	b = testutil.NewTestDaemon(t, name+"-B")
	a.PairWith(b)
	a.WriteSave("slot1.sav", "v1")
	gameID = a.TrackGame("Pause Game")
	b.API(http.MethodPost, "/api/games", map[string]string{"name": "Pause Game", "savePath": b.SaveDir}, nil)
	syncTo(a, gameID, b.NodeID())
	if !testutil.WaitFor(60*time.Second, func() bool { return b.ReadSave("slot1.sav") == "v1" }) {
		t.Fatal("setup: the first sync never landed")
	}
	testutil.SettleSync(t, gameID, a, b)
	return a, b, gameID
}

// A save changed on A does not reach a paused B — A's sync is turned away,
// and says why rather than failing — and arrives once B resumes.
func TestPause_APausedDeviceTakesNothingUntilItResumes(t *testing.T) {
	a, b, gameID := pairedTracking(t, "PauseIn")

	var st struct {
		Paused           bool  `json:"paused"`
		RemainingSeconds int64 `json:"remainingSeconds"`
	}
	b.API(http.MethodPost, "/api/sync/pause", map[string]any{"minutes": 60}, &st)
	if !st.Paused || st.RemainingSeconds < 3590 {
		t.Fatalf("pause answered %+v", st)
	}

	a.WriteSave("slot1.sav", "v2 from A")
	status, _ := syncTo(a, gameID, b.NodeID())
	if status != "peer_paused" {
		t.Errorf("A's sync with the paused B = %q, want peer_paused", status)
	}
	// Given every chance to arrive: it must not.
	time.Sleep(5 * time.Second)
	if got := b.ReadSave("slot1.sav"); got != "v1" {
		t.Fatalf("a paused device took a save: slot1.sav = %q", got)
	}
	// And A did not report it as a failure: a paused device is "later", not
	// broken, and an error in the activity feed would say otherwise.
	for _, e := range a.Daemon.Log.History() {
		if e.Level == "error" && strings.Contains(e.Message, "failed") {
			t.Errorf("A logged the pause as a failure: %s", e.Message)
		}
	}

	b.API(http.MethodPost, "/api/sync/resume", map[string]any{}, nil)
	if !testutil.WaitFor(60*time.Second, func() bool { return b.ReadSave("slot1.sav") == "v2 from A" }) {
		t.Fatalf("B resumed but never caught up: slot1.sav = %q", b.ReadSave("slot1.sav"))
	}
}

// A paused device keeps taking snapshots of its own changes but sends none
// of them, and sends them all once it resumes.
func TestPause_APausedDeviceSendsNothingButKeepsSnapshotting(t *testing.T) {
	a, b, gameID := pairedTracking(t, "PauseOut")
	snapshots := func() int {
		snaps, _ := a.Daemon.Store.ListSnapshots(gameID, "main")
		return len(snaps)
	}
	before := snapshots()

	a.API(http.MethodPost, "/api/sync/pause", map[string]any{"untilRestart": true}, nil)
	a.WriteSave("slot1.sav", "v2 while paused")

	// The watcher still snapshots the change...
	if !testutil.WaitFor(30*time.Second, func() bool { return snapshots() > before }) {
		t.Fatal("no snapshot of a change made while paused")
	}
	// ...and a sync asked for by hand says it is paused rather than trying.
	code := a.APIStatus(http.MethodPost, "/api/games/"+gameID+"/sync", nil, nil)
	if code == http.StatusOK {
		t.Errorf("a manual sync while paused answered 200")
	}
	if !strings.Contains(strings.ToLower(a.LastError()), "paused") {
		t.Errorf("the manual sync's refusal does not say it is paused: %q", a.LastError())
	}
	// B's own sync is turned away by A, so nothing is pulled either.
	status, _ := syncTo(b, gameID, a.NodeID())
	if status != "peer_paused" {
		t.Errorf("B's sync with the paused A = %q, want peer_paused", status)
	}
	time.Sleep(3 * time.Second)
	if got := b.ReadSave("slot1.sav"); got != "v1" {
		t.Fatalf("a save left a paused device: B has %q", got)
	}

	a.API(http.MethodPost, "/api/sync/resume", map[string]any{}, nil)
	if !testutil.WaitFor(60*time.Second, func() bool { return b.ReadSave("slot1.sav") == "v2 while paused" }) {
		t.Fatalf("A resumed but its change never reached B: %q", b.ReadSave("slot1.sav"))
	}
}

// The same over the internet relay, which has its own route table and its
// own way of turning an answer into an error.
func TestPause_OverTheRelay(t *testing.T) {
	relayURL := startRelay(t)
	a, b, gameID := pairedAndSynced(t, relayURL, "pause-relay")

	b.API(http.MethodPost, "/api/sync/pause", map[string]any{"minutes": 30}, nil)
	a.WriteSave("slot1.sav", "over the relay")
	status, _ := syncTo(a, gameID, b.NodeID())
	if status != "peer_paused" {
		t.Errorf("A's relay sync with the paused B = %q, want peer_paused", status)
	}
	time.Sleep(5 * time.Second)
	if got := b.ReadSave("slot1.sav"); got != "data" {
		t.Fatalf("a paused device took a save over the relay: %q", got)
	}

	b.API(http.MethodPost, "/api/sync/resume", map[string]any{}, nil)
	if !testutil.WaitFor(90*time.Second, func() bool { return b.ReadSave("slot1.sav") == "over the relay" }) {
		t.Fatalf("B resumed but never caught up over the relay: %q", b.ReadSave("slot1.sav"))
	}
}

// A snapshot taken while paused is not copied to the cloud, and is copied
// when syncing resumes — nothing goes out during the pause, and nothing is
// missing from the cloud after it.
func TestPause_HoldsCloudCopiesUntilResumed(t *testing.T) {
	td := testutil.NewTestDaemon(t, "PauseCloud")
	dir := useLocalCloud(t, td)
	td.WriteSave("slot1.sav", "v1")
	gameID := td.TrackGame("Cloud Pause Game")
	inCloud := func(snapID string) bool {
		for _, name := range cloudFiles(t, dir) {
			if strings.Contains(name, snapID) {
				return true
			}
		}
		return false
	}
	if !testutil.WaitFor(30*time.Second, func() bool { return len(cloudFiles(t, dir)) > 0 }) {
		t.Fatal("setup: nothing ever reached the cloud folder")
	}

	td.API(http.MethodPost, "/api/sync/pause", map[string]any{"untilRestart": true}, nil)
	td.WriteSave("slot1.sav", "v2")
	var snap struct {
		ID string `json:"id"`
	}
	td.API(http.MethodPost, "/api/games/"+gameID+"/snapshot", map[string]string{"comment": "while paused"}, &snap)
	if snap.ID == "" {
		t.Fatal("setup: no snapshot id")
	}
	time.Sleep(3 * time.Second)
	if inCloud(snap.ID) {
		t.Fatal("a snapshot taken while paused was copied to the cloud")
	}

	td.API(http.MethodPost, "/api/sync/resume", map[string]any{}, nil)
	if !testutil.WaitFor(30*time.Second, func() bool { return inCloud(snap.ID) }) {
		t.Fatalf("the snapshot held back while paused never reached the cloud after resuming: %v", cloudFiles(t, dir))
	}
}

func TestPause_RefusesNonsense(t *testing.T) {
	a := testutil.NewTestDaemon(t, "PauseBad")
	for _, body := range []map[string]any{
		{},
		{"minutes": 0},
		{"minutes": -5},
		{"minutes": 24*60 + 1},
		{"minutes": 30, "untilRestart": true},
	} {
		if code := a.APIStatus(http.MethodPost, "/api/sync/pause", body, nil); code != http.StatusBadRequest {
			t.Errorf("pause %v answered %d, want 400", body, code)
		}
	}
	var st struct {
		Paused bool `json:"paused"`
	}
	a.API(http.MethodGet, "/api/sync/pause", nil, &st)
	if st.Paused {
		t.Error("a refused pause paused anyway")
	}
	var res struct {
		Resumed bool `json:"resumed"`
	}
	a.API(http.MethodPost, "/api/sync/resume", map[string]any{}, &res)
	if res.Resumed {
		t.Error("resume reported ending a pause that did not exist")
	}
}
