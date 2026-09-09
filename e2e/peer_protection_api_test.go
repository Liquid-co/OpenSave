package e2e

import (
	"net/http"
	"testing"
	"time"

	"github.com/opensave/opensave/testutil"
)

// The encryption state has to reach the interface, not just exist in Go.
//
// The badge is only worth having if it is driven by the real condition, and
// the way that goes wrong is not the logic — it is the plumbing. A field that
// is computed correctly and never serialised leaves the app showing nothing at
// all, which reads as "no problem" on the screen where a problem would matter
// most. So this asks the same endpoint the app asks.
func TestPeerProtectionReachesTheApi(t *testing.T) {
	relayURL := startRelay(t)
	a := testutil.NewTestDaemon(t, "Prot-A")
	b := testutil.NewTestDaemon(t, "Prot-B")
	pairOverRelay(t, a, b, relayURL, "protection-room")

	// Sealing only reports on once the peer has proved it holds the key, and
	// that happens on the first authenticated request.
	a.WriteSave("slot1.sav", "data")
	gameID := a.TrackGame("Prot Game")
	b.API(http.MethodPost, "/api/games",
		map[string]string{"name": "Prot Game", "savePath": b.SaveDir}, nil)
	syncTo(a, gameID, b.NodeID())
	if !testutil.WaitFor(60*time.Second, func() bool { return b.ReadSave("slot1.sav") == "data" }) {
		t.Fatal("setup: the sync never landed")
	}

	type protection struct {
		Encrypted   bool   `json:"encrypted"`
		HasKey      bool   `json:"hasKey"`
		Verified    bool   `json:"verified"`
		OverRelay   bool   `json:"overRelay"`
		Fingerprint string `json:"fingerprint"`
	}
	var payload struct {
		Peers map[string]protection `json:"peers"`
	}

	if !testutil.WaitFor(30*time.Second, func() bool {
		payload.Peers = nil
		a.API(http.MethodGet, "/api/peers", nil, &payload)
		return payload.Peers[b.NodeID()].Encrypted
	}) {
		got := payload.Peers[b.NodeID()]
		t.Fatalf("the app would show this relay pairing as unencrypted after a successful "+
			"encrypted sync: %+v", got)
	}

	got := payload.Peers[b.NodeID()]
	if !got.HasKey || !got.Verified {
		t.Errorf("encrypted is true but its two preconditions are not both reported: %+v", got)
	}
	if !got.OverRelay {
		t.Error("a peer paired through the relay is not reported as reached over one, " +
			"so the interface would describe it as a local connection")
	}
	if got.Fingerprint == "" {
		t.Error("no pairing fingerprint reached the app, so there is nothing for the two " +
			"devices to compare")
	}

	// Both devices must agree on the fingerprint, or comparing them across two
	// screens — the only thing a fingerprint is for — tells the user nothing.
	var other struct {
		Peers map[string]protection `json:"peers"`
	}
	b.API(http.MethodGet, "/api/peers", nil, &other)
	if mine, theirs := got.Fingerprint, other.Peers[a.NodeID()].Fingerprint; mine != theirs {
		t.Errorf("the two devices show different fingerprints for the same pairing (%q vs %q)",
			mine, theirs)
	}
}
