//go:build windows

package store

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// A Windows mutex is recursive on the owning OS thread, not on the owning
// Go goroutine. The import claim must exclude an independent Go caller even
// when the runtime would otherwise reuse that same native thread.
func TestWindowsImportClaimNeverRecursivelyAdmitsAnotherCaller(t *testing.T) {
	db := filepath.Join(t.TempDir(), "native.db")
	s, err := Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	for round := 0; round < 8; round++ {
		claim, err := s.BeginImportPath(filepath.Join(t.TempDir(), "pending"), "incoming")
		if err != nil {
			t.Fatalf("round %d: begin import: %v", round, err)
		}
		// In particular, the ordinary same-goroutine caller cannot silently
		// inherit a recursively owned Windows mutex.
		if done, err := s.claimLock(); err == nil {
			done()
			claim.Release()
			t.Fatalf("round %d: competing same-goroutine claim was admitted", round)
		} else if !errors.Is(err, ErrGamePathOwned) {
			claim.Release()
			t.Fatalf("round %d: wrong error from denied claim: %v", round, err)
		}

		const contenders = 6
		errs := make(chan error, contenders)
		var wg sync.WaitGroup
		for i := 0; i < contenders; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				done, err := s.claimLock()
				if err == nil {
					done()
					errs <- fmt.Errorf("competing goroutine acquired an active import claim")
					return
				}
				errs <- err
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if !errors.Is(err, ErrGamePathOwned) {
				claim.Release()
				t.Fatalf("round %d: competing claim was not refused: %v", round, err)
			}
		}

		// The release caller need not be on the native mutex-owning thread.
		// A synchronous acknowledgement makes the next admission deterministic.
		released := make(chan struct{})
		go func() {
			claim.Release()
			close(released)
		}()
		select {
		case <-released:
		case <-time.After(5 * time.Second):
			t.Fatalf("round %d: release deadlocked across OS threads", round)
		}
		done, err := s.claimLock()
		if err != nil {
			t.Fatalf("round %d: mutex remained locked after release: %v", round, err)
		}
		done()
		claim.Release() // repeated release is harmless
	}
}
