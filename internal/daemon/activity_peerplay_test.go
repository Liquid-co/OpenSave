package daemon

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/opensave/opensave/internal/store"
)

// What a paired device told this one of its play sessions shows on the
// Activity page as time played there, and, when it ended later than anything
// here, as where the game was last played.
func TestActivity_TimePlayedOnEachDevice(t *testing.T) {
	d := newTestDaemon(t)
	s := d.Store
	if err := s.CreateGame(store.Game{ID: "elden", Name: "Elden Ring", SavePath: filepath.Join(t.TempDir(), "elden"), ActiveBranch: "main"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertPeer(store.Peer{ID: "deck", Name: "Steam Deck", DeviceType: "deck", Address: "192.0.2.5", Port: 8383, Status: "online"}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	hour := time.Hour.Milliseconds()
	here := now.Add(-48 * time.Hour).UnixMilli()
	if err := s.AddPlaySession("elden", here, here+2*hour); err != nil {
		t.Fatal(err)
	}
	deckEnd := now.Add(-time.Hour).UnixMilli()
	if _, err := s.AddPeerPlaySessions("deck", []store.PlaySession{
		{GameID: "elden", StartedMs: deckEnd - 3*hour, EndedMs: deckEnd},
		{GameID: "elden", StartedMs: here - 10*hour, EndedMs: here - 9*hour},
	}, now); err != nil {
		t.Fatal(err)
	}

	report, err := d.Activity(0, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	var ga *GameActivity
	for i := range report.Games {
		if report.Games[i].GameID == "elden" {
			ga = &report.Games[i]
		}
	}
	if ga == nil {
		t.Fatal("Elden Ring is not in the report")
	}
	if ga.LastPlayedOn != "Steam Deck" || ga.LastPlayedAt != deckEnd {
		t.Fatalf("last played %q at %d, want the Steam Deck at %d", ga.LastPlayedOn, ga.LastPlayedAt, deckEnd)
	}
	if ga.PlaytimeMs != 2*hour || ga.Sessions != 1 {
		t.Fatalf("played here %d ms in %d sessions, want 2 h in 1: this device's own totals are unchanged", ga.PlaytimeMs, ga.Sessions)
	}
	if len(ga.Devices) != 2 || !ga.Devices[0].Here || ga.Devices[0].PlaytimeMs != 2*hour {
		t.Fatalf("devices = %+v, want this one first with 2 h", ga.Devices)
	}
	if deck := ga.Devices[1]; deck.Device != "Steam Deck" || deck.PlaytimeMs != 4*hour || deck.Sessions != 2 {
		t.Fatalf("the deck = %+v, want 4 h in 2 sessions", deck)
	}

	played := 0
	for _, it := range report.Items {
		if it.Kind == "played" && it.Device == "Steam Deck" {
			played++
		}
	}
	if played != 2 {
		t.Fatalf("%d sessions on the deck in the timeline, want 2", played)
	}
}
