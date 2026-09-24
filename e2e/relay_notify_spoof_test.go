package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/opensave/opensave/testutil"
)

// Nobody else in the room can unpair your devices or untrack your games.
//
// The relay passes every frame to every device in a room and checks nothing
// about "from". It also publishes each device's paired peer IDs as presence.
// So the three bare notify frames — unpair, untrack, retrack — could be sent
// by anyone holding the room code, in the name of any paired device, and the
// receiver acted on them. Not data loss, but a stranger switching your sync
// off is not something a "private" room should permit.
//
// Tested from the attacker's seat, as the sealing was: a third socket in the
// room, sending exactly the frames a current build no longer sends bare.

// sendAs writes one raw frame to the relay, wearing whatever "from" it likes.
func sendAs(t *testing.T, relayURL, room string, frame map[string]any) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, fmt.Sprintf("%s/?room=%s&device=intruder", relayURL, room), nil)
	if err != nil {
		t.Fatalf("intruder could not join the room: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	raw, _ := json.Marshal(frame)
	if err := conn.Write(ctx, websocket.MessageText, raw); err != nil {
		t.Fatalf("intruder could not send: %v", err)
	}
	// Give the relay a moment to fan it out before the socket goes.
	time.Sleep(300 * time.Millisecond)
}

func pairedAndSynced(t *testing.T, relayURL, room string) (a, b *testutil.TestDaemon, gameID string) {
	t.Helper()
	a = testutil.NewTestDaemon(t, "Spoof-A")
	b = testutil.NewTestDaemon(t, "Spoof-B")
	pairOverRelay(t, a, b, relayURL, room)

	a.WriteSave("slot1.sav", "data")
	gameID = a.TrackGame("Spoof Game")
	b.API(http.MethodPost, "/api/games",
		map[string]string{"name": "Spoof Game", "savePath": b.SaveDir}, nil)
	syncTo(a, gameID, b.NodeID())
	if !testutil.WaitFor(60*time.Second, func() bool { return b.ReadSave("slot1.sav") == "data" }) {
		t.Fatal("setup: the sync never landed")
	}
	// The refusal only starts once B has seen A authenticate, which the sync
	// guarantees. Before that point a bare frame is accepted for an older
	// build's sake, so a forgery would succeed and the test would be testing
	// the compatibility window rather than the protection.
	if !testutil.WaitFor(30*time.Second, func() bool {
		p, err := b.Daemon.Store.GetPeer(a.NodeID())
		return err == nil && p.AuthVerifiedMs > 0
	}) {
		t.Fatal("setup: B never recorded A as authenticated")
	}
	return a, b, gameID
}

func TestRelayIntruder_CannotUnpairInAnotherDevicesName(t *testing.T) {
	relayURL := startRelay(t)
	const room = "spoof-unpair"
	a, b, _ := pairedAndSynced(t, relayURL, room)

	sendAs(t, relayURL, room, map[string]any{
		"type": "unpair-notify", "to": b.NodeID(), "from": a.NodeID(),
	})

	// Long enough for a frame that was going to act to have acted.
	time.Sleep(2 * time.Second)
	if _, err := b.Daemon.Store.GetPeer(a.NodeID()); err != nil {
		t.Fatal("a third device in the room unpaired A from B by writing A's ID into an unsigned frame")
	}
}

func TestRelayIntruder_CannotUntrackInAnotherDevicesName(t *testing.T) {
	relayURL := startRelay(t)
	const room = "spoof-untrack"
	a, b, gameID := pairedAndSynced(t, relayURL, room)

	sendAs(t, relayURL, room, map[string]any{
		"type": "untrack-notify", "to": b.NodeID(), "from": a.NodeID(), "gameId": gameID,
	})

	time.Sleep(2 * time.Second)
	if _, err := b.Daemon.Store.GetGame(gameID); err != nil {
		t.Fatal("a third device in the room untracked a game on B by writing A's ID into an unsigned frame")
	}
	if b.Daemon.Store.IsUntracked(gameID) {
		t.Fatal("the forged untrack left a tombstone on B; the game would refuse to come back")
	}
}

// And the signed path must not be forgeable either: a routed /unpair request
// with a fabricated "from" and no valid proof.
func TestRelayIntruder_CannotForgeASignedUnpair(t *testing.T) {
	relayURL := startRelay(t)
	const room = "spoof-signed"
	a, b, _ := pairedAndSynced(t, relayURL, room)

	sendAs(t, relayURL, room, map[string]any{
		"type": "request", "to": b.NodeID(), "from": a.NodeID(),
		"msgId": "forged-1", "route": "/unpair", "method": "POST",
		"body": json.RawMessage(`{"peerId":"` + a.NodeID() + `"}`),
		// No nonce, no MAC: the intruder has no key to make one with.
	})

	time.Sleep(2 * time.Second)
	if _, err := b.Daemon.Store.GetPeer(a.NodeID()); err != nil {
		t.Fatal("an unsigned /unpair request in A's name was acted on by B")
	}
}

// The real thing still works: A genuinely unpairing reaches B.
func TestRelayUnpair_FromTheRealDeviceStillPropagates(t *testing.T) {
	relayURL := startRelay(t)
	a, b, _ := pairedAndSynced(t, relayURL, "real-unpair")

	a.API(http.MethodDelete, "/api/peers/"+b.NodeID(), nil, nil)

	if !testutil.WaitFor(20*time.Second, func() bool {
		_, err := b.Daemon.Store.GetPeer(a.NodeID())
		return err != nil
	}) {
		// Both sides' activity, because "did not land" has had more than one
		// cause: a goodbye lost on the way, and one that landed and was
		// undone by B recording A's presence a moment later.
		for _, e := range a.Daemon.Log.History() {
			t.Logf("A %s %s %s", e.Timestamp, e.Level, e.Message)
		}
		for _, e := range b.Daemon.Log.History() {
			t.Logf("B %s %s %s", e.Timestamp, e.Level, e.Message)
		}
		t.Fatal("A unpaired B, but B still lists A as paired — the signed unpair did not land")
	}
}

// And a real untrack reaches B — the LAN version of this was silently broken
// by the same omission the push trigger had.
func TestRelayUntrack_FromTheRealDeviceStillPropagates(t *testing.T) {
	relayURL := startRelay(t)
	a, b, gameID := pairedAndSynced(t, relayURL, "real-untrack")

	a.API(http.MethodDelete, "/api/games/"+gameID, nil, nil)

	if !testutil.WaitFor(20*time.Second, func() bool {
		_, err := b.Daemon.Store.GetGame(gameID)
		return err != nil
	}) {
		t.Fatal("A untracked the game, but B still tracks it — the signed untrack did not land")
	}
}
