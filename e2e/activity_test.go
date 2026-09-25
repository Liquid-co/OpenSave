package e2e

import (
	"net/http"
	"testing"
	"time"

	"github.com/opensave/opensave/testutil"
)

type activityReport struct {
	Items []struct {
		Kind   string `json:"kind"`
		GameID string `json:"gameId"`
		Device string `json:"device"`
		Files  int    `json:"files"`
	} `json:"items"`
	Games []struct {
		GameID       string `json:"gameId"`
		LastPlayedAt int64  `json:"lastPlayedAt"`
		LastPlayedOn string `json:"lastPlayedOn"`
		LastSyncedAt int64  `json:"lastSyncedAt"`
	} `json:"games"`
}

func (r activityReport) has(kind, device string) bool {
	for _, it := range r.Items {
		if it.Kind == kind && it.Device == device && it.Files > 0 {
			return true
		}
	}
	return false
}

// The activity page's history: a save made on one device shows on the other
// as received from it — and that device as where the game was last played —
// and on the first as taken by the other.
func TestActivity_ASyncIsRecordedOnBothSides(t *testing.T) {
	a, b, gameID := pairAndTrack(t, "Activity Game", map[string]string{"slot1.sav": "start"})
	testutil.SettleSync(t, gameID, a, b)

	a.WriteSave("slot1.sav", "played on A")
	if !testutil.WaitFor(30*time.Second, func() bool { return b.ReadSave("slot1.sav") == "played on A" }) {
		t.Fatal("the save never reached B")
	}
	testutil.SettleSync(t, gameID, a, b)

	var onB, onA activityReport
	if !testutil.WaitFor(10*time.Second, func() bool {
		b.API(http.MethodGet, "/api/activity", nil, &onB)
		a.API(http.MethodGet, "/api/activity", nil, &onA)
		return onB.has("received", a.Name()) && onA.has("sent", b.Name())
	}) {
		t.Fatalf("history: B %+v; A %+v — want B to have received from %q and A to have sent to %q", onB.Items, onA.Items, a.Name(), b.Name())
	}
	for _, g := range onB.Games {
		if g.GameID == gameID && (g.LastPlayedOn != a.Name() || g.LastPlayedAt == 0 || g.LastSyncedAt == 0) {
			t.Errorf("B says the game was last played on %q at %d, synced at %d; want on %q", g.LastPlayedOn, g.LastPlayedAt, g.LastSyncedAt, a.Name())
		}
	}
	snapshots := 0
	for _, it := range onA.Items {
		if it.Kind == "snapshot" && it.GameID == gameID {
			snapshots++
		}
	}
	if snapshots == 0 {
		t.Error("A's timeline has none of its snapshots")
	}
}
