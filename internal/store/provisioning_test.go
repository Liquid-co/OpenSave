package store

import (
	"fmt"
	"testing"
)

func TestCreateHeldGameIsAtomicAndDefaultsExistingGamesClear(t *testing.T) {
	s := openTestStore(t)

	plain := Game{ID: "already-live", Name: "Already Live", SavePath: "/tmp/live", ActiveBranch: "main", AutoSync: true, MaxSnapshots: 20}
	if err := s.CreateGame(plain); err != nil {
		t.Fatal(err)
	}
	held, err := s.ProvisioningHeld("already-live")
	if err != nil {
		t.Fatal(err)
	}
	if held {
		t.Fatal("a game created without the hold was held; existing libraries must not change")
	}

	if err := s.CreateHeldGame(Game{
		ID: "configuring", Name: "Configuring", SavePath: "/tmp/cfg", MaxSnapshots: 20,
		AutoSync: true, // caller cannot override the forced off
	}); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetGame("configuring")
	if err != nil {
		t.Fatal(err)
	}
	if got.AutoSync {
		t.Fatal("held game was stored with AutoSync on")
	}
	held, err = s.ProvisioningHeld("configuring")
	if err != nil || !held {
		t.Fatalf("hold missing after commit: held=%v err=%v", held, err)
	}

	set, err := s.ProvisioningHeldSet()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := set["configuring"]; !ok {
		t.Fatal("held set omitted the new game")
	}
	if _, ok := set["already-live"]; ok {
		t.Fatal("held set included a game that was not held")
	}

	released, err := s.ReleaseProvisioning("already-live")
	if err != nil {
		t.Fatal(err)
	}
	if released {
		t.Fatal("releasing a game that was not held reported a release")
	}
	plain, err = s.GetGame("already-live")
	if err != nil {
		t.Fatal(err)
	}
	if !plain.AutoSync {
		t.Fatal("a no-op release changed AutoSync on a game that was not held")
	}

	if err := s.CreateHeldGame(Game{ID: "staged", Name: "Staged", SavePath: "/tmp/staged", MaxSnapshots: 20}); err != nil {
		t.Fatal(err)
	}
	released, err = s.ReleaseProvisioningMode("staged", false)
	if err != nil || !released {
		t.Fatalf("staged release = %v, %v", released, err)
	}
	staged, err := s.GetGame("staged")
	if err != nil {
		t.Fatal(err)
	}
	if staged.AutoSync {
		t.Fatal("staged release turned AutoSync on")
	}

	released, err = s.ReleaseProvisioning("configuring")
	if err != nil || !released {
		t.Fatalf("release = %v, %v", released, err)
	}
	got, err = s.GetGame("configuring")
	if err != nil {
		t.Fatal(err)
	}
	if !got.AutoSync {
		t.Fatal("release left AutoSync off")
	}
	held, err = s.ProvisioningHeld("configuring")
	if err != nil || held {
		t.Fatalf("hold survived release: held=%v err=%v", held, err)
	}

	released, err = s.ReleaseProvisioning("configuring")
	if err != nil || released {
		t.Fatalf("second release = %v, %v; want a no-op", released, err)
	}
}

func TestProvisioningReadFaultIsNotNotHeld(t *testing.T) {
	s := openTestStore(t)
	if err := s.CreateHeldGame(Game{ID: "held-game", Name: "Held", SavePath: "/tmp/held", MaxSnapshots: 20}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddGameAlias("alias-of-held", "held-game"); err != nil {
		t.Fatal(err)
	}
	blocks, err := s.ProvisioningBlocks("alias-of-held")
	if err != nil || !blocks {
		t.Fatalf("alias of a held game blocks=%v err=%v", blocks, err)
	}
	blocks, err = s.ProvisioningBlocks("no-such-game")
	if err != nil || blocks {
		t.Fatalf("unknown id blocks=%v err=%v; want not held", blocks, err)
	}

	s.SetProvisioningReadFault(fmt.Errorf("injected hold read failure"))
	blocks, err = s.ProvisioningBlocks("held-game")
	if err == nil || blocks {
		t.Fatalf("faulty read blocks=%v err=%v; a failed read must be an error, not held=false", blocks, err)
	}
	if _, err := s.ProvisioningHeldSet(); err == nil {
		t.Fatal("held set succeeded while reads were faulted")
	}
	// The game row itself is still readable. That is the fail-open shape:
	// the hold query fails and a caller that ignores the error would sync.
	if _, err := s.GetGame("held-game"); err != nil {
		t.Fatalf("fault leaked into game reads: %v", err)
	}
}

func TestValidExplicitGameID(t *testing.T) {
	ok := []string{"hades", "crash-bandicoot-4", "a1"}
	bad := []string{"", "Hades", "../x", "a/b", "a b", "a--b", "-a", "a-"}
	for _, id := range ok {
		if !ValidExplicitGameID(id) {
			t.Errorf("rejected %q", id)
		}
	}
	for _, id := range bad {
		if ValidExplicitGameID(id) {
			t.Errorf("accepted %q", id)
		}
	}
}
