package transfers

import (
	"fmt"
	"testing"
	"time"
)

func withClock() (*Log, *time.Time) {
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	l := New()
	l.now = func() time.Time { return now }
	return l, &now
}

func TestATransferRunsThenIsRemembered(t *testing.T) {
	l, _ := withClock()
	l.Started("hades", Event{Peer: "Deck", Direction: "download", TotalBytes: 1000})
	l.Progressed("hades", Event{Peer: "Deck", Direction: "download", BytesTransferred: 400, Percentage: 40, SpeedBytesPerSec: 200})

	s := l.Now()
	if len(s.Active) != 1 || len(s.Recent) != 0 {
		t.Fatalf("while running: %+v", s)
	}
	a := s.Active[0]
	if a.GameID != "hades" || a.Peer != "Deck" || a.State != "running" || a.BytesTransferred != 400 || a.TotalBytes != 1000 || a.SpeedBytesPerSec != 200 {
		t.Errorf("running transfer = %+v", a)
	}

	l.Finished("hades", Event{Peer: "Deck", Direction: "download"})
	s = l.Now()
	if len(s.Active) != 0 || len(s.Recent) != 1 {
		t.Fatalf("after finishing: %+v", s)
	}
	r := s.Recent[0]
	if r.State != "done" || r.Percentage != 100 || r.BytesTransferred != 1000 || r.EndedAt == "" || r.SpeedBytesPerSec != 0 {
		t.Errorf("finished transfer = %+v", r)
	}
}

func TestAFailureIsRememberedWithItsReason(t *testing.T) {
	l, _ := withClock()
	l.Started("hades", Event{Peer: "Deck"})
	l.Finished("hades", Event{Peer: "Deck", Error: "connection reset"})
	r := l.Now().Recent[0]
	if r.State != "error" || r.Error != "connection reset" {
		t.Errorf("failed transfer = %+v", r)
	}
}

// Each device and direction is its own transfer: a game syncing with two
// devices at once is two rows, and an upload is not a download.
func TestTransfersAreKeptApartByDeviceAndDirection(t *testing.T) {
	l, _ := withClock()
	l.Started("hades", Event{Peer: "Deck", Direction: "download"})
	l.Started("hades", Event{Peer: "Laptop", Direction: "download"})
	l.Started("hades", Event{Peer: "Deck", Direction: "upload"})
	if got := len(l.Now().Active); got != 3 {
		t.Fatalf("active = %d, want 3", got)
	}
	l.Finished("hades", Event{Peer: "Deck", Direction: "upload"})
	s := l.Now()
	if len(s.Active) != 2 || s.Recent[0].Direction != "upload" {
		t.Errorf("finishing the upload ended the wrong transfer: %+v", s)
	}
}

// A device from before reports carried a direction sends progress without
// one; it joins the upload already running instead of opening a second row.
func TestProgressWithoutADirectionJoinsTheRunningTransfer(t *testing.T) {
	l, _ := withClock()
	l.Started("hades", Event{Peer: "Deck", Direction: "upload"})
	l.Progressed("hades", Event{Peer: "Deck", Percentage: 50})
	s := l.Now()
	if len(s.Active) != 1 || s.Active[0].Direction != "upload" || s.Active[0].Percentage != 50 {
		t.Errorf("active = %+v, want the one upload at 50%%", s.Active)
	}
}

// Reports are fire-and-forget. Progress with no start seen still shows, a
// finish with no start is still remembered, and one that stops reporting
// is not left running forever.
func TestLostReportsDoNotLeaveGhosts(t *testing.T) {
	l, now := withClock()
	l.Progressed("celeste", Event{Peer: "Deck", Percentage: 10})
	if len(l.Now().Active) != 1 {
		t.Fatal("progress without a start was dropped")
	}
	l.Finished("balatro", Event{Peer: "Deck"})
	if r := l.Now().Recent; len(r) != 1 || r[0].GameID != "balatro" {
		t.Fatalf("a finish without a start was dropped: %+v", r)
	}

	*now = now.Add(staleAfter + time.Second)
	s := l.Now()
	if len(s.Active) != 0 {
		t.Fatalf("a transfer that stopped reporting is still running: %+v", s.Active)
	}
	if s.Recent[0].GameID != "celeste" || s.Recent[0].State != "error" {
		t.Errorf("the silent transfer should be remembered as ended unannounced: %+v", s.Recent[0])
	}
}

func TestOnlyTheRecentOnesAreKept(t *testing.T) {
	l, _ := withClock()
	for i := 0; i < KeepRecent+10; i++ {
		l.Finished(fmt.Sprintf("game-%d", i), Event{Peer: "Deck"})
	}
	s := l.Now()
	if len(s.Recent) != KeepRecent {
		t.Fatalf("remembered %d, want %d", len(s.Recent), KeepRecent)
	}
	if s.Recent[0].GameID != fmt.Sprintf("game-%d", KeepRecent+9) {
		t.Errorf("newest first: got %s first", s.Recent[0].GameID)
	}
}

// A copy goes out, so a caller holding it cannot change what is recorded.
func TestTheSnapshotIsACopy(t *testing.T) {
	l, _ := withClock()
	l.Finished("hades", Event{Peer: "Deck"})
	s := l.Now()
	s.Recent[0].GameID = "tampered"
	if l.Now().Recent[0].GameID != "hades" {
		t.Error("changing the returned snapshot changed the log")
	}
}
