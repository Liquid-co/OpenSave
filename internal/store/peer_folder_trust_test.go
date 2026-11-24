package store

import "testing"

func TestPeerFolderTrust(t *testing.T) {
	s := openTestStore(t)
	if err := s.EnsureDefaultSettings(t.TempDir(), t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertPeer(Peer{ID: "p1", Name: "Deck"}); err != nil {
		t.Fatal(err)
	}
	if s.PeerChoosesFolders("p1") {
		t.Fatal("a device chooses folders before being let")
	}
	if err := s.SetPeerChoosesFolders("p1", true); err != nil {
		t.Fatal(err)
	}
	if !s.PeerChoosesFolders("p1") {
		t.Fatal("not recorded")
	}
	if err := s.SetPeerChoosesFolders("nobody", true); err == nil {
		t.Error("an unpaired device was let choose folders")
	}
	// Trust goes with the pairing.
	if err := s.UnpairPeer("p1"); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertPeer(Peer{ID: "p1", Name: "Deck"}); err != nil {
		t.Fatal(err)
	}
	if s.PeerChoosesFolders("p1") {
		t.Error("paired again, the device kept its old trust")
	}
}

// An offer for a game tracked since is not shown, however it came to be.
func TestOfferedGamesLeaveOutTrackedOnes(t *testing.T) {
	s := openTestStore(t)
	if err := s.EnsureDefaultSettings(t.TempDir(), t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertPeer(Peer{ID: "p1", Name: "Deck"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"tracked-game", "waiting-game"} {
		if err := s.RecordOfferedGame(OfferedGame{GameID: id, PeerID: "p1", Name: id, PeerPath: "/x"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.CreateGame(Game{ID: "tracked-game", Name: "Tracked", SavePath: t.TempDir(), ActiveBranch: "main"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.ListOfferedGames()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].GameID != "waiting-game" {
		t.Errorf("offered = %+v, want only waiting-game", got)
	}
}
