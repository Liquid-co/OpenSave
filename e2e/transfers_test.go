package e2e

import (
	"net/http"
	"testing"
	"time"

	"github.com/opensave/opensave/testutil"
)

type transferRow struct {
	GameID    string `json:"gameId"`
	Peer      string `json:"peer"`
	Direction string `json:"direction"`
	State     string `json:"state"`
}

type transferList struct {
	Active []transferRow `json:"active"`
	Recent []transferRow `json:"recent"`
}

func hasTransfer(list transferList, gameID, peer, direction, state string) bool {
	for _, t := range append(list.Active, list.Recent...) {
		if t.GameID == gameID && t.Peer == peer && t.Direction == direction && t.State == state {
			return true
		}
	}
	return false
}

// A sync leaves a record on both devices: the one that pulled has a finished
// download from the other, and the one pulled from has the upload its peer
// told it about.
func TestTransfers_BothDevicesRecordASync(t *testing.T) {
	a, b, gameID := pairedTracking(t, "Moves")

	a.WriteSave("slot1.sav", "moved along")
	syncTo(a, gameID, b.NodeID())
	if !testutil.WaitFor(60*time.Second, func() bool { return b.ReadSave("slot1.sav") == "moved along" }) {
		t.Fatal("setup: the change never reached B")
	}

	var atB transferList
	if !testutil.WaitFor(20*time.Second, func() bool {
		b.API(http.MethodGet, "/api/transfers", nil, &atB)
		return hasTransfer(atB, gameID, "Moves-A", "download", "done")
	}) {
		t.Errorf("B has no finished download from A: %+v", atB)
	}
	var atA transferList
	if !testutil.WaitFor(20*time.Second, func() bool {
		a.API(http.MethodGet, "/api/transfers", nil, &atA)
		return hasTransfer(atA, gameID, "Moves-B", "upload", "done")
	}) {
		t.Errorf("A has no finished upload to B: %+v", atA)
	}
	// Nothing left looking as if it were still running.
	for _, list := range []transferList{atA, atB} {
		if len(list.Active) != 0 {
			t.Errorf("a finished sync left transfers running: %+v", list.Active)
		}
	}
}
