package daemon

import (
	"errors"
	"testing"

	"github.com/opensave/opensave/internal/store"
)

// TrackGame checks that an id is free and then inserts; a peer's auto-track
// can land between the two. The insert error is how that race is recognised,
// so the match is pinned against what the driver really says — a wording
// change would silently turn a clear "another device synced it here" back
// into "UNIQUE constraint failed: games.id (1555)".
func TestIsDuplicateGameIDMatchesTheRealDriverError(t *testing.T) {
	d := newTestDaemon(t)
	g := store.Game{ID: "game-one", Name: "Game One", SavePath: t.TempDir(), ActiveBranch: "main"}
	if err := d.Store.CreateGame(g); err != nil {
		t.Fatal(err)
	}
	dup := d.Store.CreateGame(g)
	if dup == nil {
		t.Fatal("creating the same game twice succeeded; the id is not unique")
	}
	if !isDuplicateGameID(dup) {
		t.Errorf("the real duplicate-id error is not recognised: %v", dup)
	}
	for _, other := range []error{
		errors.New("database is closed"),
		errors.New("UNIQUE constraint failed: peers.id"),
		nil,
	} {
		if isDuplicateGameID(other) {
			t.Errorf("%v is treated as a duplicate game id", other)
		}
	}
}
