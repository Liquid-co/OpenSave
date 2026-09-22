package e2e

import (
	"net/http"
	"testing"
	"time"

	"github.com/opensave/opensave/testutil"
)

// Untracking clears everything a game agreed with its peers — and then a
// sync that was already in flight writes some of it back.
//
// UntrackGame deletes the game and calls ForgetGameSyncState, which is
// correct and thorough. What neither it nor the writers know is that the
// lineage row can be recreated a moment later: SetSyncState is an upsert,
// game_peer_sync_state has no foreign key to games, and both writers — a
// sync finishing its own work, and a peer's "I pulled these files" report
// arriving over the network — write without asking whether the game is
// still tracked.
//
// Re-tracking then produces the same slug, so the game comes back holding
// a record of files it agreed with a peer in a previous life. Anything
// removed from the folder while it was untracked reads as a deletion to
// propagate, and the peer — which did nothing at all — loses the file.
//
// That is the failure CI reported on Windows under the race detector:
// "a game tracked afresh inherited its old lineage and DELETED the peer's
// b.sav". It is a race there; here the late write is made deliberately, so
// the mechanism is tested rather than the timing.
func TestUntrack_LateLineageWriteMustNotSurviveIntoARetrack(t *testing.T) {
	a := testutil.NewTestDaemon(t, "Revival-A")
	b := testutil.NewTestDaemon(t, "Revival-B")
	a.PairWith(b)

	a.WriteSave("a.sav", "alpha")
	a.WriteSave("b.sav", "bravo")
	gameID := a.TrackGame("Revival Game")
	b.API(http.MethodPost, "/api/games",
		map[string]string{"name": "Revival Game", "savePath": b.SaveDir}, nil)
	syncTo(a, gameID, b.NodeID())
	if !testutil.WaitFor(60*time.Second, func() bool {
		return b.ReadSave("a.sav") == "alpha" && b.ReadSave("b.sav") == "bravo"
	}) {
		t.Fatal("setup: the initial sync never landed")
	}
	if !testutil.WaitFor(30*time.Second, func() bool {
		files, _, err := a.Daemon.Store.GetSyncState(gameID, b.NodeID())
		return err == nil && len(files) == 2
	}) {
		t.Fatal("setup: no lineage was recorded after the sync")
	}

	// Untrack. Everything this game agreed with B is supposed to go with it.
	a.API(http.MethodDelete, "/api/games/"+gameID, nil, nil)
	if files, _, _ := a.Daemon.Store.GetSyncState(gameID, b.NodeID()); len(files) != 0 {
		t.Fatalf("untracking left lineage behind: %v", files)
	}

	// Now the write that was already on its way. This is what a peer's
	// sync-event does when it reports what it just pulled — it arrives over
	// the network and cannot be ordered against a local untrack.
	a.Daemon.P2P.Sync.AddConfirmedLineage(gameID, b.NodeID(), []string{"a.sav", "b.sav"})

	if files, _, _ := a.Daemon.Store.GetSyncState(gameID, b.NodeID()); len(files) != 0 {
		t.Errorf("a lineage row was created for a game that is not tracked: %v — "+
			"re-tracking this id inherits it", files)
	}

	// While untracked, the folder loses a file. Then the game is tracked
	// again: same name, same folder, same slug.
	a.RemoveSave("b.sav")
	var again struct {
		ID string `json:"id"`
	}
	a.API(http.MethodPost, "/api/games",
		map[string]string{"name": "Revival Game", "savePath": a.SaveDir}, &again)
	if again.ID != gameID {
		t.Fatalf("retracking produced id %q, want %q", again.ID, gameID)
	}

	syncTo(a, gameID, b.NodeID())

	// A fresh track has no history, so b.sav is a file the peer has and this
	// side does not: pulled back, never deleted. The peer touched nothing and
	// must not lose it.
	if !testutil.WaitFor(60*time.Second, func() bool { return a.ReadSave("b.sav") == "bravo" }) {
		if b.ReadSave("b.sav") == "" {
			t.Fatal("the peer's b.sav was DELETED — a late lineage write outlived the " +
				"untrack, and the re-tracked game read a file missing here as a deletion to propagate")
		}
		t.Errorf("b.sav was not pulled back to the retracked side (A=%q, B=%q)",
			a.ReadSave("b.sav"), b.ReadSave("b.sav"))
	}
}
