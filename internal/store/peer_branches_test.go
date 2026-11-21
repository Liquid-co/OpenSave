package store

import "testing"

func TestPeerBranches_KeepsTheLatestAndGoesWithThePeer(t *testing.T) {
	s := openTestStore(t)
	if err := s.UpsertPeer(Peer{ID: "deck", Name: "Steam Deck", Address: "127.0.0.1", Port: 8393}); err != nil {
		t.Fatal(err)
	}
	if err := s.NotePeerBranch("elden", "deck", "main", 1000); err != nil {
		t.Fatal(err)
	}
	if err := s.NotePeerBranch("elden", "deck", "ng-plus", 2000); err != nil {
		t.Fatal(err)
	}
	if err := s.NotePeerBranch("elden", "deck", "", 3000); err != nil {
		t.Fatal(err)
	}
	got, err := s.PeerBranches("elden")
	if err != nil || got["deck"].Branch != "ng-plus" || got["deck"].SeenMs != 2000 {
		t.Fatalf("got %+v (err %v), want ng-plus at 2000: an empty answer changes nothing", got, err)
	}
	if err := s.UnpairPeer("deck"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.PeerBranches("elden"); len(got) != 0 {
		t.Fatalf("after unpairing: %+v, want nothing left", got)
	}
}
