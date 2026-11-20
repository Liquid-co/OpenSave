package store

import (
	"testing"
	"time"
)

func addTestPeer(t *testing.T, s *Store, id, name string) {
	t.Helper()
	if err := s.UpsertPeer(Peer{ID: id, Name: name, DeviceType: "deck", Address: "192.0.2.5", Port: 8383, Status: "online"}); err != nil {
		t.Fatalf("UpsertPeer: %v", err)
	}
}

func TestPlaySessionsEndedSince_OldestFirstFromTheMark(t *testing.T) {
	s := openTestStore(t)
	trackGames(t, s, "elden", "hades")
	for _, p := range []PlaySession{{"elden", 100, 200}, {"hades", 300, 400}, {"elden", 500, 600}} {
		if err := s.AddPlaySession(p.GameID, p.StartedMs, p.EndedMs); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.PlaySessionsEndedSince(400, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].EndedMs != 400 || got[1].EndedMs != 600 {
		t.Fatalf("since 400: %+v, want the sessions ending at 400 and 600, oldest first", got)
	}
	if got, _ := s.PlaySessionsEndedSince(0, 1); len(got) != 1 || got[0].EndedMs != 200 {
		t.Fatalf("limit 1: %+v, want only the oldest", got)
	}
}

func TestAddPeerPlaySessions_KeepsEachOnceAndDropsTheImpossible(t *testing.T) {
	s := openTestStore(t)
	addTestPeer(t, s, "deck", "Steam Deck")
	now := time.UnixMilli(10_000_000_000)
	day := int64(24 * time.Hour / time.Millisecond)
	in := []PlaySession{
		{"elden", 1000, 5000},
		{"elden", 1000, 5000},                                 // told twice
		{"hades", 9000, 8000},                                 // ends before it starts
		{"", 1000, 2000},                                      // no game
		{"celeste", 1000, 1000 + 8*day},                       // longer than a week
		{"celeste", now.UnixMilli(), now.UnixMilli() + 2*day}, // ends far in the future
		{"celeste", 20_000, 30_000},
	}
	added, err := s.AddPeerPlaySessions("deck", in, now)
	if err != nil {
		t.Fatal(err)
	}
	if added != 2 {
		t.Fatalf("added %d, want 2 (elden once, celeste once)", added)
	}
	again, _ := s.AddPeerPlaySessions("deck", in, now)
	if again != 0 {
		t.Fatalf("sending the same sessions again added %d, want 0", again)
	}
	if latest, _ := s.LatestPeerSessionEnd("deck"); latest != 30_000 {
		t.Fatalf("latest end = %d, want 30000", latest)
	}
	if latest, _ := s.LatestPeerSessionEnd("nobody"); latest != 0 {
		t.Fatalf("latest end for a device that told nothing = %d, want 0", latest)
	}
}

func TestPeerPlayStatsByGame_TotalsPerDevice(t *testing.T) {
	s := openTestStore(t)
	addTestPeer(t, s, "deck", "Steam Deck")
	addTestPeer(t, s, "laptop", "Old Laptop")
	now := time.UnixMilli(10_000_000_000)
	if _, err := s.AddPeerPlaySessions("deck", []PlaySession{{"elden", 0 + 1, 1001}, {"elden", 2000, 4000}}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddPeerPlaySessions("laptop", []PlaySession{{"elden", 5000, 5500}, {"hades", 100, 200}}, now); err != nil {
		t.Fatal(err)
	}
	stats, err := s.PeerPlayStatsByGame()
	if err != nil {
		t.Fatal(err)
	}
	deck := stats["elden"]["deck"]
	if deck.Sessions != 2 || deck.PlaytimeMs != 3000 || deck.LastPlayedMs != 4000 {
		t.Fatalf("elden on the deck: %+v, want 2 sessions, 3000 ms, last 4000", deck)
	}
	if lap := stats["elden"]["laptop"]; lap.PlaytimeMs != 500 {
		t.Fatalf("elden on the laptop: %+v, want 500 ms", lap)
	}
	if _, ok := stats["hades"]["deck"]; ok {
		t.Fatal("hades was never played on the deck, yet has totals there")
	}
	recent, _ := s.RecentPeerPlaySessions(0, 2)
	if len(recent) != 2 || recent[0].EndedMs != 5500 || recent[0].PeerID != "laptop" {
		t.Fatalf("recent: %+v, want the laptop's 5500 first", recent)
	}
}

func TestPeerPlaySessions_GoWhenTheDeviceIsUnpaired(t *testing.T) {
	s := openTestStore(t)
	addTestPeer(t, s, "deck", "Steam Deck")
	if _, err := s.AddPeerPlaySessions("deck", []PlaySession{{"elden", 1, 1000}}, time.UnixMilli(10_000_000_000)); err != nil {
		t.Fatal(err)
	}
	if err := s.UnpairPeer("deck"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.RecentPeerPlaySessions(0, 0); len(got) != 0 {
		t.Fatalf("after unpairing, %d of its sessions are still kept", len(got))
	}
}
