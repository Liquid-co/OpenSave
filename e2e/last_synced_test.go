package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/opensave/opensave/internal/store"
	"github.com/opensave/opensave/testutil"
)

// "Is my Deck up to date with this save?" — answered per game, per device,
// from the payload the app and the CLI both read.
//
// The device-level stamp cannot answer it: one device syncing three games
// writes one peers.lastSynced, and the game that was skipped reads as fresh
// as the two that moved. So the test tracks two games and syncs one, and
// insists the other stays "never" while the device says "just now".
func TestLastSynced_PerGamePerPeer(t *testing.T) {
	a := testutil.NewTestDaemon(t, "Stamp-A")
	b := testutil.NewTestDaemon(t, "Stamp-B")
	logOnFailure(t, a, b)
	a.PairWith(b)
	// B does not adopt games it has not been told where to keep. That makes
	// a game tracked only on A one that A "syncs" with B and nothing moves —
	// the case the per-game stamp has to stay silent on.
	setUnknownGamePolicy(t, b, store.UnknownGameAsk)

	// Game one, tracked on both. B first, so A's tracking finds it there.
	b.API(http.MethodPost, "/api/games", map[string]string{"name": "Stamp One", "savePath": b.SaveDir}, nil)
	a.WriteSave("one.sav", "one")
	one := a.TrackGame("Stamp One")

	// Game two, tracked on A alone, in a sibling folder since one folder is
	// one game.
	twoDir := filepath.Join(filepath.Dir(a.SaveDir), "save-two")
	if err := os.MkdirAll(twoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(twoDir, "two.sav"), []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	var twoResp struct {
		ID string `json:"id"`
	}
	a.API(http.MethodPost, "/api/games", map[string]string{"name": "Stamp Two", "savePath": twoDir}, &twoResp)
	two := twoResp.ID

	start := time.Now().Add(-5 * time.Second) // slack for clock granularity
	syncTo(a, one, b.NodeID())
	if !testutil.WaitFor(45*time.Second, func() bool { return b.ReadSave("one.sav") == "one" }) {
		t.Fatal("setup: game one never reached B")
	}

	// A, which ran the sync, stamps B for game one.
	if !testutil.WaitFor(10*time.Second, func() bool { return lastSyncedWith(a, one)[b.NodeID()] != "" }) {
		t.Fatalf("A has no last-synced time for game one with B: %v", lastSyncedWith(a, one))
	}
	assertRecent(t, "A's stamp for game one with B", lastSyncedWith(a, one)[b.NodeID()], start)

	// B, which pulled, stamps A for game one on its own side: its user asks
	// the same question from the other chair.
	if !testutil.WaitFor(10*time.Second, func() bool { return lastSyncedWith(b, one)[a.NodeID()] != "" }) {
		t.Fatalf("B has no last-synced time for game one with A: %v", lastSyncedWith(b, one))
	}
	assertRecent(t, "B's stamp for game one with A", lastSyncedWith(b, one)[a.NodeID()], start)

	// Game two: A asks, B has nowhere to put it, nothing moves. The
	// device-level stamp advances — the two devices did talk and finish,
	// which is all it has ever meant — and that is exactly why it cannot
	// answer for one game. The per-game stamp must not.
	deviceBefore := peerLastSynced(a, b.NodeID())
	time.Sleep(1100 * time.Millisecond) // stamps have millisecond resolution; make a move visible
	if status, _ := syncTo(a, two, b.NodeID()); status != "peer_awaiting_folder" {
		t.Fatalf("setup: syncing the unplaced game reported %q, want peer_awaiting_folder", status)
	}
	if got := lastSyncedWith(a, two); len(got) != 0 {
		t.Errorf("nothing of game two moved, yet A reports it synced: %v", got)
	}
	if after := peerLastSynced(a, b.NodeID()); after <= deviceBefore {
		t.Errorf("the device-level stamp did not advance on the empty sync (%q -> %q); "+
			"then it could have told the games apart, and the point of the per-game stamp is that it cannot", deviceBefore, after)
	}

	// A second sync of game one with nothing to move is still a sync: A finds
	// the two identical, tells B, and B re-checks and moves its own stamp.
	// This path has no sync of its own on B to announce it, so the payload is
	// pushed to B's dashboard by the confirmation instead.
	//
	// B must be the one NOT syncing here, or the assertion proves nothing:
	// the pull wrote one.sav into B's folder, B's watcher saw that, and the
	// sync it starts a couple of seconds later would move the stamp by
	// itself. So B stops watching the game, and the stamp is left to settle
	// before it is read — a control with the confirmation stamp removed
	// passed this assertion until it was written this way.
	b.API(http.MethodPatch, "/api/games/"+one, map[string]any{"autoSync": false}, nil)
	// And B's dashboard is listening, since the whole point of the
	// confirmation path is that the app finds out without polling.
	dash := listenToDashboard(t, b)
	before := lastSyncedWith(b, one)[a.NodeID()]
	for settled := false; !settled; {
		time.Sleep(4 * time.Second)
		if now := lastSyncedWith(b, one)[a.NodeID()]; now == before {
			settled = true
		} else {
			before = now
		}
	}
	if status, _ := syncTo(a, one, b.NodeID()); status != "in_sync" {
		t.Fatalf("setup: the second sync of game one reported %q, want in_sync", status)
	}
	if !testutil.WaitFor(15*time.Second, func() bool { return lastSyncedWith(b, one)[a.NodeID()] > before }) {
		t.Errorf("B's stamp for A did not move on an in-sync confirmation: still %q", before)
	}
	if !testutil.WaitFor(10*time.Second, func() bool { return dash.gameStamp(one, a.NodeID()) > before }) {
		t.Errorf("B's dashboard was never sent the moved stamp (last games-update says %q); "+
			"the app would keep showing the old time until something else happened", dash.gameStamp(one, a.NodeID()))
	}

	// Unpairing takes the stamps with it; the list is the paired devices.
	a.API(http.MethodDelete, "/api/peers/"+b.NodeID(), nil, nil)
	if !testutil.WaitFor(10*time.Second, func() bool { return len(lastSyncedWith(a, one)) == 0 }) {
		t.Errorf("after unpairing B, A still reports %v for game one", lastSyncedWith(a, one))
	}
}

// dashboard collects the games-update frames a daemon pushes to its own
// app, the way the app receives them.
type dashboard struct {
	mu    sync.Mutex
	games map[string]struct {
		LastSyncedWith map[string]string `json:"lastSyncedWith"`
	}
}

func (d *dashboard) gameStamp(gameID, peerID string) string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.games[gameID].LastSyncedWith[peerID]
}

func listenToDashboard(t *testing.T, td *testutil.TestDaemon) *dashboard {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	conn, _, err := websocket.Dial(ctx, "ws://"+td.Addr+"/ws", nil)
	if err != nil {
		cancel()
		t.Fatalf("could not open the dashboard socket: %v", err)
	}
	conn.SetReadLimit(16 << 20)
	t.Cleanup(func() {
		cancel()
		conn.Close(websocket.StatusNormalClosure, "")
	})
	d := &dashboard{}
	go func() {
		for {
			_, raw, err := conn.Read(ctx)
			if err != nil {
				return
			}
			var msg struct {
				Type string          `json:"type"`
				Data json.RawMessage `json:"data"`
			}
			if json.Unmarshal(raw, &msg) != nil || msg.Type != "games-update" {
				continue
			}
			var games map[string]struct {
				LastSyncedWith map[string]string `json:"lastSyncedWith"`
			}
			if json.Unmarshal(msg.Data, &games) != nil {
				continue
			}
			d.mu.Lock()
			d.games = games
			d.mu.Unlock()
		}
	}()
	return d
}

func peerLastSynced(td *testutil.TestDaemon, peerID string) string {
	td.T.Helper()
	p, err := td.Daemon.Store.GetPeer(peerID)
	if err != nil {
		td.T.Fatal(err)
	}
	return p.LastSynced.String
}

// The terminal says it too, per game and per device, through the real
// binary — a Deck in Game Mode may have no other way to ask.
func TestCLIStatus_SaysWhenEachDeviceLastSyncedEachGame(t *testing.T) {
	a := testutil.NewTestDaemon(t, "StampCLI-A")
	b := testutil.NewTestDaemon(t, "StampCLI-B")
	a.PairWith(b)
	setUnknownGamePolicy(t, b, store.UnknownGameAsk)

	b.API(http.MethodPost, "/api/games", map[string]string{"name": "Synced Game", "savePath": b.SaveDir}, nil)
	a.WriteSave("s.sav", "x")
	synced := a.TrackGame("Synced Game")

	lonely := filepath.Join(filepath.Dir(a.SaveDir), "save-lonely")
	if err := os.MkdirAll(lonely, 0o755); err != nil {
		t.Fatal(err)
	}
	a.API(http.MethodPost, "/api/games", map[string]string{"name": "Lonely Game", "savePath": lonely}, nil)

	syncTo(a, synced, b.NodeID())
	if !testutil.WaitFor(45*time.Second, func() bool { return lastSyncedWith(a, synced)[b.NodeID()] != "" }) {
		t.Fatal("setup: the sync never stamped")
	}

	out := runCLIAgainst(t, a, "status")
	t.Logf("what a person sees:\n%s", out)

	// Each game names the device and says when — or that it never has.
	syncedAt := strings.Index(out, "Synced Game")
	lonelyAt := strings.Index(out, "Lonely Game")
	if syncedAt < 0 || lonelyAt < 0 {
		t.Fatalf("status lists neither game:\n%s", out)
	}
	// Games print in list order; take each game's block as the text up to
	// the next game (or the devices section).
	block := func(from int) string {
		rest := out[from:]
		end := len(rest)
		for _, marker := range []string{"Synced Game", "Lonely Game", "Paired devices"} {
			if i := strings.Index(rest[1:], marker); i >= 0 && i+1 < end {
				end = i + 1
			}
		}
		return rest[:end]
	}
	if got := block(syncedAt); !strings.Contains(got, "StampCLI-B") || !strings.Contains(got, "synced just now") {
		t.Errorf("the synced game does not say the device synced just now:\n%s", got)
	}
	if got := block(lonelyAt); !strings.Contains(got, "StampCLI-B") || !strings.Contains(got, "never synced") {
		t.Errorf("the game B never took does not say so:\n%s", got)
	}

	// And the JSON form carries the raw stamps, parsed not string-matched.
	var js struct {
		Games []struct {
			ID             string            `json:"id"`
			LastSyncedWith map[string]string `json:"lastSyncedWith"`
		} `json:"games"`
	}
	if err := json.Unmarshal([]byte(runCLIAgainst(t, a, "status", "--json")), &js); err != nil {
		t.Fatalf("`opensave status --json` is not JSON: %v", err)
	}
	byID := map[string]map[string]string{}
	for _, g := range js.Games {
		byID[g.ID] = g.LastSyncedWith
	}
	if byID[synced][b.NodeID()] == "" {
		t.Errorf("--json: the synced game has no stamp for B: %v", byID[synced])
	}
	if lw, ok := byID["lonely-game"]; !ok || len(lw) != 0 {
		t.Errorf("--json: the lonely game should carry an empty lastSyncedWith, got %v (present=%v)", lw, ok)
	}
}

// lastSyncedWith reads one game's per-device stamps through the API the
// app uses, so what is asserted is what a person is shown.
func lastSyncedWith(td *testutil.TestDaemon, gameID string) map[string]string {
	td.T.Helper()
	var games map[string]struct {
		LastSyncedWith map[string]string `json:"lastSyncedWith"`
	}
	td.API(http.MethodGet, "/api/games", nil, &games)
	g, ok := games[gameID]
	if !ok {
		td.T.Fatalf("game %s is not in /api/games", gameID)
	}
	if g.LastSyncedWith == nil {
		td.T.Errorf("game %s has no lastSyncedWith field at all; the app would show nothing rather than \"never\"", gameID)
		return map[string]string{}
	}
	return g.LastSyncedWith
}

func assertRecent(t *testing.T, what, iso string, notBefore time.Time) {
	t.Helper()
	ts, err := time.Parse("2006-01-02T15:04:05.000Z", iso)
	if err != nil {
		t.Errorf("%s is %q, not the timestamp form the daemon writes: %v", what, iso, err)
		return
	}
	if ts.Before(notBefore) || ts.After(time.Now().Add(5*time.Second)) {
		t.Errorf("%s is %s, outside the window of this test (%s .. now)", what, ts.Format(time.RFC3339), notBefore.Format(time.RFC3339))
	}
}
