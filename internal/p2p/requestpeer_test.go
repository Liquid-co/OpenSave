package p2p

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/opensave/opensave/internal/store"
)

// Two devices behind one address: the one that signed the request is the one
// it came from. By address, the Deck's "in sync" report stamped the laptop as
// synced with a game it did not hold.
func TestRequestPeer_TheSignedDeviceNotTheFirstAtItsAddress(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "opensave.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	for _, p := range []store.Peer{
		{ID: "laptop", Name: "Laptop", Address: "192.168.1.9", Port: 8403},
		{ID: "deck", Name: "Steam Deck", Address: "192.168.1.9", Port: 8393},
	} {
		if err := s.UpsertPeer(p); err != nil {
			t.Fatal(err)
		}
	}
	e := &Engine{Store: s, Log: func(string, string) {}}

	r := httptest.NewRequest("POST", "/api/p2p/sync-event/elden", nil)
	r.RemoteAddr = "192.168.1.9:50000"
	byAddress, _ := e.peerByAddress("192.168.1.9")
	signedBy := "deck"
	if byAddress.ID == "deck" {
		signedBy = "laptop" // whichever the address lookup does not pick
	}
	r = r.WithContext(context.WithValue(r.Context(), lanPeerKey{}, signedBy))
	if got, ok := e.requestPeer(r); !ok || got.ID != signedBy {
		t.Fatalf("request signed by %s attributed to %q", signedBy, got.ID)
	}

	// A build that does not sign is still known by its address.
	unsigned := httptest.NewRequest("POST", "/api/p2p/sync-event/elden", nil)
	unsigned.RemoteAddr = "192.168.1.9:50000"
	if got, ok := e.requestPeer(unsigned); !ok || got.ID != byAddress.ID {
		t.Fatalf("unsigned request attributed to %q, want %q by address", got.ID, byAddress.ID)
	}
}
