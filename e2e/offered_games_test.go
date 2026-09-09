package e2e

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/opensave/opensave/internal/store"
	"github.com/opensave/opensave/testutil"
)

// "Ask me where to put it" — the alternative to guessing a folder from the
// peer's save path.
//
// Auto-tracking stays the default and is what makes OpenSave zero-config. This
// is for the setups where the guess is least reliable: several drives, folders
// moved with a junction, saves kept in a per-account directory whose name is
// different on every machine.

// setUnknownGamePolicy switches a device between guessing and asking.
func setUnknownGamePolicy(t *testing.T, d *testutil.TestDaemon, policy string) {
	t.Helper()
	d.API(http.MethodPost, "/api/settings", map[string]any{"unknownGameFromPeer": policy}, nil)
	settings, err := d.Daemon.Store.GetSettings()
	if err != nil {
		t.Fatalf("reading settings back: %v", err)
	}
	if got := settings.UnknownGameFromPeer; got != policy {
		t.Fatalf("policy did not stick: stored %q, wanted %q", got, policy)
	}
}

// The default has to be exactly what it has always been. A user who never
// opened settings must not find that their games stopped appearing.
//
// Asserted as "a game was created, and no offer was recorded" rather than "the
// files arrived". Both daemons run on one machine here, so an auto-tracked
// game on B resolves to A's own folder — the two are then genuinely in sync and
// nothing is copied into B's separate save dir. TestFullSyncFlow documents the
// same thing and works around it by pre-tracking on B. What matters for this
// change is which of the two paths ran, and that is exactly what is checked.
func TestOfferedGames_DefaultStillAutoTracks(t *testing.T) {
	a := testutil.NewTestDaemon(t, "Offer-Default-A")
	b := testutil.NewTestDaemon(t, "Offer-Default-B")
	a.PairWith(b)

	a.WriteSave("slot1.sav", "from A")
	gameID := a.TrackGame("AutoTracked Game")
	syncTo(a, gameID, b.NodeID())

	var games []store.Game
	if !testutil.WaitFor(30*time.Second, func() bool {
		games, _ = b.Daemon.Store.ListGames()
		return len(games) > 0
	}) {
		t.Fatal("the default no longer auto-tracks — the peer's game was never " +
			"created, so a user who never opened settings would silently lose sync")
	}
	if games[0].ID != gameID {
		t.Errorf("auto-tracked under %q, want the peer's id %q", games[0].ID, gameID)
	}
	if games[0].SavePath == "" {
		t.Error("the auto-tracked game has no save path")
	}

	offers, err := b.Daemon.Store.ListOfferedGames()
	if err != nil {
		t.Fatal(err)
	}
	if len(offers) != 0 {
		t.Errorf("auto-tracking also recorded %d offer(s); the two paths must be "+
			"exclusive or the user is asked about a game already syncing", len(offers))
	}
}

// With asking turned on, an unknown game becomes an offer and nothing is
// created or synced until a person chooses a folder.
func TestOfferedGames_AskRecordsAnOfferAndTracksNothing(t *testing.T) {
	a := testutil.NewTestDaemon(t, "Offer-Ask-A")
	b := testutil.NewTestDaemon(t, "Offer-Ask-B")
	a.PairWith(b)
	setUnknownGamePolicy(t, b, store.UnknownGameAsk)

	a.WriteSave("slot1.sav", "from A")
	gameID := a.TrackGame("Asked Game")
	syncTo(a, gameID, b.NodeID())

	var offers []store.OfferedGame
	if !testutil.WaitFor(30*time.Second, func() bool {
		offers, _ = b.Daemon.Store.ListOfferedGames()
		return len(offers) > 0
	}) {
		t.Fatal("no offer was recorded — the peer's game vanished without a trace, " +
			"which is the one outcome worse than guessing a folder")
	}

	got := offers[0]
	if got.GameID != gameID {
		t.Errorf("offer recorded under %q, want the peer's id %q — placing it must "+
			"create the game under the id both devices match on", got.GameID, gameID)
	}
	if got.Name != "Asked Game" {
		t.Errorf("offer name = %q", got.Name)
	}
	if got.PeerID != a.NodeID() {
		t.Errorf("offer peer = %q, want %q — the list has to say who is waiting",
			got.PeerID, a.NodeID())
	}
	if got.PeerPath == "" {
		t.Error("the peer's own save path was not kept; it is the hint a person " +
			"uses to pick the matching folder here")
	}

	// Nothing may have been tracked or written.
	games, err := b.Daemon.Store.ListGames()
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 0 {
		t.Errorf("asking still created %d game(s): %+v", len(games), games)
	}
	if b.ReadSave("slot1.sav") != "" {
		t.Error("a save was written despite nobody choosing a folder")
	}
}

// The requesting device must be told the truth. "Not found" is what a
// deliberately untracked game reports, and reusing it here would show the user
// nothing wrong while their save waited on a click at the other end.
func TestOfferedGames_ThePeerIsToldItIsWaiting(t *testing.T) {
	a := testutil.NewTestDaemon(t, "Offer-Status-A")
	b := testutil.NewTestDaemon(t, "Offer-Status-B")
	a.PairWith(b)
	setUnknownGamePolicy(t, b, store.UnknownGameAsk)

	a.WriteSave("slot1.sav", "from A")
	gameID := a.TrackGame("Status Game")
	status, _ := syncTo(a, gameID, b.NodeID())

	if status == "peer_missing" {
		t.Error("the sync reported peer_missing — that is what an untracked game " +
			"reports, so a game waiting for a folder is indistinguishable from one " +
			"the peer does not have")
	}
	if status != "peer_awaiting_folder" {
		t.Errorf("sync status = %q, want peer_awaiting_folder", status)
	}
}

// Asking governs only the guess. A game already tracked on both devices, or
// matched by App ID, never reaches that branch and must sync as it always has.
func TestOfferedGames_AskingDoesNotAffectAlreadyTrackedGames(t *testing.T) {
	a := testutil.NewTestDaemon(t, "Offer-Tracked-A")
	b := testutil.NewTestDaemon(t, "Offer-Tracked-B")
	a.PairWith(b)
	setUnknownGamePolicy(t, b, store.UnknownGameAsk)

	// Both devices track it deliberately, at folders that share nothing.
	rootA := filepath.Join(a.SaveDir, "here")
	rootB := filepath.Join(b.SaveDir, "somewhere", "else")
	for _, d := range []string{rootA, rootB} {
		if err := os.MkdirAll(d, 0o777); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite(t, filepath.Join(rootA, "slot1.sav"), "from A")

	gameA := trackAt(a, "Both Sides Game", rootA)
	trackAt(b, "Both Sides Game", rootB)
	syncTo(a, gameA, b.NodeID())

	if !testutil.WaitFor(45*time.Second, func() bool {
		d, err := os.ReadFile(filepath.Join(rootB, "slot1.sav"))
		return err == nil && string(d) == "from A"
	}) {
		t.Error("a game tracked on both devices did not sync while asking was on — " +
			"the setting must only govern the folder guess")
	}
	offers, _ := b.Daemon.Store.ListOfferedGames()
	if len(offers) != 0 {
		t.Errorf("an already-tracked game was recorded as an offer: %+v", offers)
	}
}

// The whole point, end to end: a game offered by a peer, placed at a folder
// this device chose, then syncing normally — with the two devices keeping
// completely different paths.
func TestOfferedGames_PlacingAnOfferSyncsAtTheChosenFolder(t *testing.T) {
	a := testutil.NewTestDaemon(t, "Offer-Place-A")
	b := testutil.NewTestDaemon(t, "Offer-Place-B")
	a.PairWith(b)
	setUnknownGamePolicy(t, b, store.UnknownGameAsk)

	// A keeps the game somewhere B has no reason to mirror.
	rootA := filepath.Join(a.SaveDir, "SaveGames", "76561198000000001")
	if err := os.MkdirAll(rootA, 0o777); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(rootA, "world.sav"), "from A")
	gameA := trackAt(a, "Placed Game", rootA)

	syncTo(a, gameA, b.NodeID())
	if !testutil.WaitFor(30*time.Second, func() bool {
		offers, _ := b.Daemon.Store.ListOfferedGames()
		return len(offers) > 0
	}) {
		t.Fatal("setup: no offer was recorded")
	}

	// B chooses its own folder — a different name at a different depth.
	rootB := filepath.Join(b.SaveDir, "my", "own", "place")
	if err := os.MkdirAll(rootB, 0o777); err != nil {
		t.Fatal(err)
	}
	var placed struct {
		ID       string `json:"id"`
		SavePath string `json:"savePath"`
	}
	b.API(http.MethodPost, "/api/offered-games/"+gameA+"/place",
		map[string]string{"path": rootB}, &placed)

	if placed.ID != gameA {
		t.Fatalf("placed under id %q, want the peer's id %q — a different id would "+
			"never sync with the device that offered it (%s)", placed.ID, gameA, b.LastError())
	}
	if placed.SavePath != rootB {
		t.Errorf("placed at %q, want the folder that was chosen (%q)", placed.SavePath, rootB)
	}

	// The offer is answered and must not linger.
	if offers, _ := b.Daemon.Store.ListOfferedGames(); len(offers) != 0 {
		t.Errorf("placing left %d offer(s) behind", len(offers))
	}

	// And now it syncs, into B's folder rather than A's.
	syncTo(a, gameA, b.NodeID())
	if !testutil.WaitFor(45*time.Second, func() bool {
		d, err := os.ReadFile(filepath.Join(rootB, "world.sav"))
		return err == nil && string(d) == "from A"
	}) {
		got, err := os.ReadFile(filepath.Join(rootB, "world.sav"))
		t.Errorf("the placed game did not sync into the chosen folder: %q (err %v)", got, err)
	}
	// A's folder layout must not have been recreated under B.
	if _, err := os.Stat(filepath.Join(b.SaveDir, "SaveGames")); err == nil {
		t.Error("A's folder layout was recreated on B — the chosen folder was not used")
	}
}

// Declining has to stick. A peer asking again every few minutes would
// otherwise put the same prompt back for ever.
func TestOfferedGames_DecliningStopsThePeerReAsking(t *testing.T) {
	a := testutil.NewTestDaemon(t, "Offer-Decline-A")
	b := testutil.NewTestDaemon(t, "Offer-Decline-B")
	a.PairWith(b)
	setUnknownGamePolicy(t, b, store.UnknownGameAsk)

	a.WriteSave("slot1.sav", "from A")
	gameID := a.TrackGame("Declined Game")
	syncTo(a, gameID, b.NodeID())
	if !testutil.WaitFor(30*time.Second, func() bool {
		offers, _ := b.Daemon.Store.ListOfferedGames()
		return len(offers) > 0
	}) {
		t.Fatal("setup: no offer was recorded")
	}

	b.API(http.MethodPost, "/api/offered-games/"+gameID+"/decline", nil, nil)
	if offers, _ := b.Daemon.Store.ListOfferedGames(); len(offers) != 0 {
		t.Fatalf("declining left %d offer(s)", len(offers))
	}

	// The peer asks again, as it will.
	syncTo(a, gameID, b.NodeID())
	if offers, _ := b.Daemon.Store.ListOfferedGames(); len(offers) != 0 {
		t.Errorf("the peer's next request put the declined game back (%d offers) — "+
			"the prompt would return for ever", len(offers))
	}
	if games, _ := b.Daemon.Store.ListGames(); len(games) != 0 {
		t.Errorf("a declined game was tracked anyway: %+v", games)
	}
}

// Tracking a declined game deliberately must undo the refusal, exactly as
// re-tracking an untracked game does. One concept, not two.
func TestOfferedGames_TrackingADeclinedGameUndoesTheRefusal(t *testing.T) {
	a := testutil.NewTestDaemon(t, "Offer-Undecline-A")
	b := testutil.NewTestDaemon(t, "Offer-Undecline-B")
	a.PairWith(b)
	setUnknownGamePolicy(t, b, store.UnknownGameAsk)

	a.WriteSave("slot1.sav", "from A")
	gameID := a.TrackGame("Rethought Game")
	syncTo(a, gameID, b.NodeID())
	testutil.WaitFor(30*time.Second, func() bool {
		offers, _ := b.Daemon.Store.ListOfferedGames()
		return len(offers) > 0
	})
	b.API(http.MethodPost, "/api/offered-games/"+gameID+"/decline", nil, nil)

	if !b.Daemon.Store.IsUntracked(gameID) {
		t.Fatal("declining did not record a refusal, so nothing stops the peer re-asking")
	}

	rootB := filepath.Join(b.SaveDir, "changed", "my", "mind")
	if err := os.MkdirAll(rootB, 0o777); err != nil {
		t.Fatal(err)
	}
	trackAt(b, "Rethought Game", rootB)

	if b.Daemon.Store.IsUntracked(gameID) {
		t.Error("tracking the game deliberately left the refusal in place — the " +
			"user's own action must override their earlier decline")
	}
}

// The CLI has to reach this too: a headless box or a relay host has no
// Settings window, and those are exactly the machines with unusual layouts.
func TestOfferedGames_TheCLICanSetThePolicy(t *testing.T) {
	c := newCLI(t)
	c.startDaemon()

	c.mustRun("config", "set", "unknown-game-from-peer", "ask")
	if out := c.mustRun("config", "list"); !strings.Contains(out, "ask") {
		t.Errorf("`config list` does not report the policy:\n%s", out)
	}

	// A typo must be refused, not read as "track". Someone typing this is
	// deliberately turning the guess off, and silently leaving it on is the
	// wrong way to be wrong.
	out := c.mustFail("config", "set", "unknown-game-from-peer", "asdf")
	if !strings.Contains(out, "track") || !strings.Contains(out, "ask") {
		t.Errorf("the refusal does not say what the valid answers are:\n%s", out)
	}
}
