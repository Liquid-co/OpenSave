package store

import (
	"testing"

	"github.com/opensave/opensave/internal/e2ee"
)

func testPublicKey(t *testing.T) string {
	t.Helper()
	id, err := e2ee.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	return e2ee.EncodeKey(id.Public)
}

func TestUnpairedPeersAreKeptUntilTold(t *testing.T) {
	s := openTestStore(t)
	key := testPublicKey(t)

	if err := s.RememberUnpaired(UnpairedPeer{ID: "deck", Name: "Deck", Address: "relay", PublicKey: key, UnpairedMs: 100}); err != nil {
		t.Fatal(err)
	}
	// Remembering again replaces rather than duplicates.
	if err := s.RememberUnpaired(UnpairedPeer{ID: "deck", Name: "Deck", Address: "10.0.0.5", Port: 8383, PublicKey: key, UnpairedMs: 200}); err != nil {
		t.Fatal(err)
	}
	got, err := s.UnpairedPeers()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Address != "10.0.0.5" || got[0].Port != 8383 || got[0].PublicKey != key || got[0].UnpairedMs != 200 {
		t.Fatalf("UnpairedPeers = %+v", got)
	}

	if err := s.ForgetUnpaired("deck"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.UnpairedPeers(); len(got) != 0 {
		t.Fatalf("still kept after being forgotten: %+v", got)
	}
}

// Nothing can be signed without a key, and a device paired before keys
// existed accepts an unsigned goodbye anyway.
func TestUnpairedPeerWithoutAKeyIsNotKept(t *testing.T) {
	s := openTestStore(t)
	if err := s.RememberUnpaired(UnpairedPeer{ID: "old", PublicKey: "  ", UnpairedMs: 1}); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.UnpairedPeers(); len(got) != 0 {
		t.Fatalf("kept a device with no key: %+v", got)
	}
}

func TestUnpairedPeersExpire(t *testing.T) {
	s := openTestStore(t)
	key := testPublicKey(t)
	for _, p := range []UnpairedPeer{{ID: "old", PublicKey: key, UnpairedMs: 100}, {ID: "new", PublicKey: key, UnpairedMs: 900}} {
		if err := s.RememberUnpaired(p); err != nil {
			t.Fatal(err)
		}
	}
	n, err := s.ForgetUnpairedBefore(500)
	if err != nil || n != 1 {
		t.Fatalf("ForgetUnpairedBefore = %d, %v; want 1", n, err)
	}
	if got, _ := s.UnpairedPeers(); len(got) != 1 || got[0].ID != "new" {
		t.Fatalf("left = %+v", got)
	}
}

// Pairing again settles it: a goodbye sent after that would undo the new
// pairing.
func TestPairingAgainForgetsTheUnpairedRecord(t *testing.T) {
	s := openTestStore(t)
	key := testPublicKey(t)
	if err := s.RememberUnpaired(UnpairedPeer{ID: "deck", PublicKey: key, UnpairedMs: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertPeer(Peer{ID: "deck", Name: "Deck", Address: "relay", Status: "online"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPeerPublicKey("deck", key); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.UnpairedPeers(); len(got) != 0 {
		t.Fatalf("still owed a goodbye after pairing again: %+v", got)
	}
}

// Presence code reads a peer, marks it online and writes it back. If the
// peer is unpaired in between — its goodbye handled on another goroutine
// while its heartbeat is handled on this one — writing it back must not pair
// it again. It did: the unpair landed and was undone a moment later.
func TestAPresenceUpdateDoesNotPairAgainAPeerUnpairedMeanwhile(t *testing.T) {
	s := openTestStore(t)
	if err := s.UpsertPeer(Peer{ID: "a", Name: "A", Address: "relay", Status: "offline"}); err != nil {
		t.Fatal(err)
	}

	seen, err := s.GetPeer("a") // the heartbeat handler reads the peer…
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UnpairPeer("a"); err != nil { // …the goodbye is handled…
		t.Fatal(err)
	}
	seen.Status = "online"
	if err := s.UpdatePeer(seen); err != nil { // …and the handler writes back.
		t.Fatal(err)
	}
	if _, err := s.GetPeer("a"); err == nil {
		t.Fatal("a peer unpaired while its presence was being recorded is paired again")
	}

	// The same write through UpsertPeer is what did it, which is why presence
	// must not use it.
	if err := s.UpsertPeer(seen); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetPeer("a"); err != nil {
		t.Fatal("UpsertPeer no longer inserts, so this test no longer shows why UpdatePeer exists")
	}
}
