package e2e

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/opensave/opensave/testutil"
)

// Deleting a save the moment it has finished reaching the other device.
//
// This used to lose the deletion. A file only enters the shared lineage once
// the pushing device has SEEN it in the peer's manifest, and until then a local
// deletion of it is read as "new on the peer" — so the next sync pulls the file
// back instead of propagating the delete. The peer having the file on disk is
// not the same moment as the pusher having written that down, and a user
// deleting a save right after it syncs lands squarely in the gap.
//
// Measured, under concurrent load, deleting immediately on arrival:
//
//	lineage recorded only by manifest refresh   : 4 lost out of 20
//	lineage recorded from the peer's own report : 6 lost out of 135
//
// The second figure was first recorded as "0 out of 20", which was true and
// misleading: at the rate later measured, twenty rounds show zero failures
// about a third of the time. It was re-measured across 135 rounds and three
// build configurations, and the residual rate is real — around 4% per round,
// unchanged by the request-authentication and hash-cache work that was
// suspected of causing it. Treat this test as reducing the window, not closing
// it, and do not read a single green run as proof of a fix.
//
// The test deliberately does NOT settle before deleting. Settling would make it
// pass whatever the engine does, and the window is the entire subject.
func TestDeletionImmediatelyAfterArrivalStillPropagates(t *testing.T) {
	// Several rounds: the window is a race, and one attempt could miss it.
	for round := 1; round <= 5; round++ {
		func() {
			a := testutil.NewTestDaemon(t, "DelWin-A")
			b := testutil.NewTestDaemon(t, "DelWin-B")
			a.PairWith(b)

			a.WriteSave("keep.sav", "keep me")
			a.WriteSave("drop.sav", "drop me")
			gameID := a.TrackGame("DeleteWindowGame")

			var gameB struct {
				ID string `json:"id"`
			}
			b.API(http.MethodPost, "/api/games",
				map[string]string{"name": "DeleteWindowGame", "savePath": b.SaveDir}, &gameB)
			if gameB.ID == "" {
				t.Fatalf("round %d: tracking on the peer failed (%s)", round, b.LastError())
			}

			syncTo(a, gameID, b.NodeID())
			if !testutil.WaitFor(60*time.Second, func() bool { return b.ReadSave("drop.sav") == "drop me" }) {
				t.Fatalf("round %d setup: the file never reached the peer", round)
			}

			// Delete NOW — no settle. This is the window.
			if err := os.Remove(filepath.Join(a.SaveDir, "drop.sav")); err != nil {
				t.Fatal(err)
			}
			a.API(http.MethodPost, "/api/games/"+gameID+"/snapshot",
				map[string]string{"comment": "deleted a save"}, nil)
			syncTo(a, gameID, b.NodeID())

			if !testutil.WaitFor(40*time.Second, func() bool { return b.ReadSave("drop.sav") == "" }) {
				lineage, _, _ := a.Daemon.Store.GetSyncState(gameID, b.NodeID())
				t.Errorf("round %d: the deletion never reached the peer — it still holds a "+
					"file the user deleted (lineage at delete time: %v)", round, lineage)
			}

			// The deletion must not have taken the neighbouring save with it,
			// and must not have been undone by a pull-back on this side.
			if got := b.ReadSave("keep.sav"); got != "keep me" {
				t.Errorf("round %d: an unrelated save was disturbed on the peer: %q", round, got)
			}
			if got := a.ReadSave("drop.sav"); got != "" {
				t.Errorf("round %d: the deleted file came back on the device that "+
					"deleted it (%q) — it was pulled back from the peer", round, got)
			}
		}()
	}
}
