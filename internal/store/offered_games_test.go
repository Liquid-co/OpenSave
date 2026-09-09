package store

import "testing"

func offerFixture(gameID, peerID string) OfferedGame {
	return OfferedGame{
		GameID: gameID, PeerID: peerID, Name: "Some Game",
		AppID: "1091500", CoverURL: "https://example.invalid/c.jpg",
		PeerPath: `C:\Users\them\AppData\Local\Some Game`,
	}
}

func TestOfferedGameRoundTrip(t *testing.T) {
	s := openTestStore(t)

	if err := s.RecordOfferedGame(offerFixture("somegame", "peer-1")); err != nil {
		t.Fatal(err)
	}
	all, err := s.ListOfferedGames()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("listed %d offers, want 1", len(all))
	}
	got := all[0]
	if got.GameID != "somegame" || got.PeerID != "peer-1" || got.Name != "Some Game" {
		t.Errorf("offer came back as %+v", got)
	}
	if got.AppID != "1091500" || got.CoverURL == "" {
		t.Errorf("the details a tile needs were lost: %+v", got)
	}
	if got.FirstSeen == "" {
		t.Error("first_seen was not stamped")
	}
}

// An offer must be identifiable and complete enough to act on. A row with no
// game id or no peer is neither.
func TestOfferedGameRequiresIdentity(t *testing.T) {
	s := openTestStore(t)
	for label, o := range map[string]OfferedGame{
		"no game id": {PeerID: "peer-1", Name: "X"},
		"no peer id": {GameID: "x", Name: "X"},
		"blank ids":  {GameID: "  ", PeerID: "  ", Name: "X"},
	} {
		if err := s.RecordOfferedGame(o); err == nil {
			t.Errorf("%s was accepted", label)
		}
	}
}

// A peer re-asking every few minutes must not keep rewriting first_seen, or
// the list reorders under the user while they are reading it.
func TestRepeatedOffersKeepTheirPlace(t *testing.T) {
	s := openTestStore(t)

	first := offerFixture("somegame", "peer-1")
	first.FirstSeen = "2020-01-01T00:00:00Z"
	if err := s.RecordOfferedGame(first); err != nil {
		t.Fatal(err)
	}

	again := offerFixture("somegame", "peer-1")
	again.Name = "Some Game (renamed)"
	again.FirstSeen = "2026-01-01T00:00:00Z"
	if err := s.RecordOfferedGame(again); err != nil {
		t.Fatal(err)
	}

	all, _ := s.ListOfferedGames()
	if len(all) != 1 {
		t.Fatalf("a repeated offer created %d rows, want 1", len(all))
	}
	if all[0].FirstSeen != "2020-01-01T00:00:00Z" {
		t.Errorf("first_seen moved to %q on a repeat offer", all[0].FirstSeen)
	}
	// The details themselves should refresh — a game renamed on the peer
	// should not keep showing its old name here.
	if all[0].Name != "Some Game (renamed)" {
		t.Errorf("the offer's details did not refresh: %q", all[0].Name)
	}
}

// Two devices can offer the same game. Both are recorded, because the list is
// more useful saying who is waiting.
func TestSeveralPeersCanOfferTheSameGame(t *testing.T) {
	s := openTestStore(t)
	for _, peer := range []string{"peer-1", "peer-2"} {
		if err := s.RecordOfferedGame(offerFixture("shared", peer)); err != nil {
			t.Fatal(err)
		}
	}
	all, _ := s.ListOfferedGames()
	if len(all) != 2 {
		t.Fatalf("got %d offers, want one per peer", len(all))
	}

	// Answering the question answers it for everyone: both peers were waiting
	// on the same thing, a folder on this device.
	if err := s.ClearOfferedGame("shared"); err != nil {
		t.Fatal(err)
	}
	if all, _ = s.ListOfferedGames(); len(all) != 0 {
		t.Errorf("clearing a game left %d offers behind", len(all))
	}
}

// Unpairing a device removes its offers: placing one afterwards would create a
// game with nobody to sync it to.
func TestClearingAPeerLeavesOtherPeersOffers(t *testing.T) {
	s := openTestStore(t)
	if err := s.RecordOfferedGame(offerFixture("game-a", "peer-1")); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordOfferedGame(offerFixture("game-b", "peer-2")); err != nil {
		t.Fatal(err)
	}
	if err := s.ClearOfferedGamesForPeer("peer-1"); err != nil {
		t.Fatal(err)
	}
	all, _ := s.ListOfferedGames()
	if len(all) != 1 || all[0].PeerID != "peer-2" {
		t.Errorf("after unpairing peer-1 the offers are %+v", all)
	}
}

// Looking up a game nobody has offered is an ordinary empty answer, not an
// error: the caller asks this on every manifest request.
func TestLookingUpAnUnofferedGameIsNotAnError(t *testing.T) {
	s := openTestStore(t)
	got, err := s.OfferedGame("never-heard-of-it")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d offers for an unknown game", len(got))
	}
}

// The fallback direction matters more than the parsing. An unrecognised value
// has to mean "keep syncing", because the alternative is a device that
// silently stops tracking anything new and gives no reason.
func TestUnknownSettingValuesFallBackToTracking(t *testing.T) {
	for _, value := range []string{"", "track", "TRACK", "nonsense", "asking", "0", "true"} {
		if (Settings{UnknownGameFromPeer: value}).ShouldAskBeforeTracking() {
			t.Errorf("%q was read as \"ask\"; anything unrecognised must keep the "+
				"tracking behaviour rather than silently stopping sync", value)
		}
	}
	for _, value := range []string{"ask", "ASK", " Ask "} {
		if !(Settings{UnknownGameFromPeer: value}).ShouldAskBeforeTracking() {
			t.Errorf("%q was not read as \"ask\"", value)
		}
	}
}

// A fresh install must behave exactly as it always has.
func TestNewInstallsDefaultToTracking(t *testing.T) {
	s := openTestStore(t)
	// The settings row is created on first launch, not by opening the store.
	if err := s.EnsureDefaultSettings(t.TempDir(), t.TempDir()); err != nil {
		t.Fatal(err)
	}
	settings, err := s.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	if settings.ShouldAskBeforeTracking() {
		t.Errorf("a new install defaulted to asking (value %q); auto-tracking is "+
			"what makes OpenSave zero-config and must stay the default",
			settings.UnknownGameFromPeer)
	}
}

// Unpairing must take the offers with it, wherever the unpair came from. The
// cleanup lives inside UnpairPeer rather than at its call sites so a new one
// cannot forget it.
func TestUnpairingADeviceRemovesItsOffers(t *testing.T) {
	s := openTestStore(t)
	if err := s.UpsertPeer(Peer{ID: "peer-1", Name: "Laptop", Status: "online"}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordOfferedGame(offerFixture("game-a", "peer-1")); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordOfferedGame(offerFixture("game-b", "peer-2")); err != nil {
		t.Fatal(err)
	}

	if err := s.UnpairPeer("peer-1"); err != nil {
		t.Fatal(err)
	}
	all, _ := s.ListOfferedGames()
	if len(all) != 1 || all[0].PeerID != "peer-2" {
		t.Errorf("after unpairing peer-1 the offers are %+v — an offer from an "+
			"unpaired device would invite creating a game with no peer to sync it", all)
	}
}
