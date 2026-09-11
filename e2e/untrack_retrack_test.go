package e2e

import (
	"net/http"
	"testing"
	"time"

	"github.com/opensave/opensave/testutil"
)

// Untracking a game must not leave its sync history behind to act on later.
//
// Game IDs are slugs of the name, so untrack "Elden Ring" and track it again
// and it is the same ID — and the lineage, merge base and push record for
// that ID with every peer were never removed. They come back live. The
// lineage says "both sides held b.sav"; if the folder lost b.sav while it was
// untracked, the first sync reads that as "this side deleted it" and removes
// the peer's copy. The peer did nothing, and lost a file to a game that was
// tracked afresh a moment ago.
//
// The deletion records were already cleared on untrack, with exactly this
// reasoning written next to the call. The lineage is the same hazard.
func TestUntrackThenRetrack_DoesNotInheritStaleLineage(t *testing.T) {
	a := testutil.NewTestDaemon(t, "Retrack-A")
	b := testutil.NewTestDaemon(t, "Retrack-B")
	a.PairWith(b)

	a.WriteSave("a.sav", "alpha")
	a.WriteSave("b.sav", "bravo")
	gameID := a.TrackGame("Retrack Game")
	b.API(http.MethodPost, "/api/games",
		map[string]string{"name": "Retrack Game", "savePath": b.SaveDir}, nil)
	syncTo(a, gameID, b.NodeID())
	if !testutil.WaitFor(60*time.Second, func() bool {
		return b.ReadSave("a.sav") == "alpha" && b.ReadSave("b.sav") == "bravo"
	}) {
		t.Fatal("setup: the initial sync never landed")
	}
	// The lineage on A now records both files as shared. Confirm, so the
	// test is about that record and not about whether one was written.
	if !testutil.WaitFor(30*time.Second, func() bool {
		files, _, err := a.Daemon.Store.GetSyncState(gameID, b.NodeID())
		return err == nil && len(files) == 2
	}) {
		t.Fatal("setup: no lineage was recorded after the sync")
	}

	// Untrack on A. While untracked, the folder loses a file — a manual
	// clean-up, a game reinstall, anything. B is not touched.
	a.API(http.MethodDelete, "/api/games/"+gameID, nil, nil)
	a.RemoveSave("b.sav")

	// Track it again: same name, same folder, same slug — same ID.
	var again struct {
		ID string `json:"id"`
	}
	a.API(http.MethodPost, "/api/games",
		map[string]string{"name": "Retrack Game", "savePath": a.SaveDir}, &again)
	if again.ID != gameID {
		t.Fatalf("retracking produced id %q, want the same slug %q; the hazard under test "+
			"needs the ID to repeat", again.ID, gameID)
	}

	syncTo(a, gameID, b.NodeID())

	// A fresh track has no history, so b.sav is simply a file the peer has
	// and this side does not: pulled, never deleted. The peer must keep it.
	if !testutil.WaitFor(60*time.Second, func() bool { return a.ReadSave("b.sav") == "bravo" }) {
		if b.ReadSave("b.sav") == "" {
			t.Fatal("a game tracked afresh inherited its old lineage and DELETED the peer's b.sav — " +
				"the peer did nothing and lost a file")
		}
		t.Fatalf("b.sav was not pulled back to the retracked side (A=%q, B=%q)",
			a.ReadSave("b.sav"), b.ReadSave("b.sav"))
	}
	if b.ReadSave("b.sav") != "bravo" {
		t.Fatalf("the peer's b.sav was disturbed: %q", b.ReadSave("b.sav"))
	}

	// And the peer must still be tracking ITS OWN folder. The untrack removed
	// its game record, path included, and the re-track used to bring it back
	// by auto-tracking at a path guessed from A's — leaving the peer's real
	// saves in a folder nothing watched any more.
	restored, err := b.Daemon.Store.GetGame(gameID)
	if err != nil {
		t.Fatalf("the peer does not track the game after the re-track: %v", err)
	}
	if restored.SavePath != b.SaveDir {
		t.Errorf("after the round trip the peer tracks %s, not its own folder %s — its real "+
			"saves are orphaned and will never sync again", restored.SavePath, b.SaveDir)
	}
}
