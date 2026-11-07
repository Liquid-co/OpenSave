package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func TestConcurrentDisjointRegistrations(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "native.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	other, err := Open(s.path)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	stores := []*Store{s, other}
	root := t.TempDir()
	start := make(chan struct{})
	results := make(chan error, 24)
	for i := 0; i < 24; i++ {
		go func(i int) {
			<-start
			results <- stores[i%len(stores)].CreateGame(Game{ID: fmt.Sprintf("game-%d", i), Name: fmt.Sprintf("Game %d", i), SavePath: filepath.Join(root, fmt.Sprintf("save-%d", i))})
		}(i)
	}
	close(start)
	failures := 0
	for i := 0; i < 24; i++ {
		if err := <-results; err != nil {
			failures++
			t.Log(err)
		}
	}
	if failures != 0 {
		t.Fatalf("%d/24 disjoint registrations failed", failures)
	}
}

func TestRegistrationWaitsForImportClaim(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "native.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a, err := s.BeginImportPath(filepath.Join(t.TempDir(), "import"), "incoming")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Release()
	result := make(chan error, 1)
	root := filepath.Join(t.TempDir(), "unrelated")
	go func() { result <- s.CreateGame(Game{ID: "unrelated", Name: "Unrelated", SavePath: root}) }()
	select {
	case err := <-result:
		t.Fatalf("registration returned before claim release: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	a.Release()
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("registration did not resume")
	}
}

func TestRegistrationClaimTimeoutIsBusyWithoutMutation(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "native.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.claimWaitFor = 40 * time.Millisecond
	target := filepath.Join(t.TempDir(), "target")
	a, err := s.BeginImportPath(target, "incoming")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Release()
	start := time.Now()
	err = s.CreateGame(Game{ID: "waiting", Name: "Waiting", SavePath: target})
	if !errors.Is(err, ErrPathClaimBusy) || !errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrGamePathOwned) {
		t.Fatalf("wrong timeout classification: %v", err)
	}
	if time.Since(start) < s.claimWaitFor {
		t.Fatal("refused before bounded wait elapsed")
	}
	if _, err := s.GetGame("waiting"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("timed out registration mutated state: %v", err)
	}
	a.Release()
	if err := s.CreateGame(Game{ID: "waiting", Name: "Waiting", SavePath: target}); err != nil {
		t.Fatalf("claim leaked after timeout: %v", err)
	}
}

func TestClaimWaitCancellationDoesNotAcquireOrLeak(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "native.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a, err := s.BeginImportPath(filepath.Join(t.TempDir(), "target"), "incoming")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Release()
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		result <- s.CreateGameContext(ctx, Game{ID: "cancelled", Name: "Cancelled", SavePath: filepath.Join(t.TempDir(), "cancelled")})
	}()
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) || !errors.Is(err, ErrPathClaimBusy) {
			t.Fatalf("wrong cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled admission did not return")
	}
	a.Release()
	if _, err := s.GetGame("cancelled"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cancelled registration mutated state: %v", err)
	}
	release, err := s.tryClaimLock()
	if err != nil {
		t.Fatal(err)
	}
	release()
}

func TestRegistrationRechecksOwnershipAfterWaiting(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "native.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	target := filepath.Join(t.TempDir(), "target")
	a, err := s.BeginImportPath(target, "incoming")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Release()
	result := make(chan error, 1)
	go func() { result <- s.CreateGame(Game{ID: "other", Name: "Other", SavePath: target}) }()
	select {
	case err := <-result:
		t.Fatalf("returned before release: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	if err := s.CreateGameAdmitted(Game{ID: "incoming", Name: "Incoming", SavePath: target}, a); err != nil {
		t.Fatal(err)
	}
	a.Release()
	select {
	case err := <-result:
		if !errors.Is(err, ErrGamePathOwned) || errors.Is(err, ErrPathClaimBusy) {
			t.Fatalf("ownership not rechecked: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("registration never resumed")
	}
}
