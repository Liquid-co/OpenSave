package syncengine

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/opensave/opensave/internal/store"
)

// A hold lookup that fails must not be treated as "not held". The game row
// is still readable, which is the shape that used to let SyncGame continue
// into FetchManifest.
func TestUnreadableProvisioningHoldDoesNotFetchManifest(t *testing.T) {
	env := setupEngine(t)
	if err := env.store.CreateHeldGame(store.Game{
		ID: "held1", Name: "Held", SavePath: env.localDir, MaxSnapshots: 10,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.store.GetGame("held1"); err != nil {
		t.Fatal(err)
	}
	env.store.SetProvisioningReadFault(errors.New("injected hold read failure"))
	if _, err := env.store.GetGame("held1"); err != nil {
		t.Fatalf("fault leaked into game reads: %v", err)
	}

	_, err := env.engine.SyncGame(context.Background(), "held1", []Peer{env.peer})
	if err == nil || !errors.Is(err, ErrProvisioningUnreadable) {
		t.Fatalf("sync err = %v, want ErrProvisioningUnreadable", err)
	}
	if env.transport.manifestCalls != 0 {
		t.Fatalf("FetchManifest was called %d times after a failed hold read", env.transport.manifestCalls)
	}
	if len(env.transport.deletedOnPeer) != 0 {
		t.Fatalf("a failed hold read deleted %v", env.transport.deletedOnPeer)
	}
}

// An unpatched 2.4.1 client classifies a manifest error with these helpers.
// A provisioning refusal must not collapse into "not found", which is how a
// peer decides the save is gone.
func TestStockClientDoesNotTreatProvisioningRefusalAsMissing(t *testing.T) {
	body := errors.New(`peer returned 409: {"error":"` + ProvisioningMessage + `"}`)
	if isGameNotFound(body) || isHeld(body) || isAwaitingFolder(body) {
		t.Fatalf("stock classifiers swallowed a provisioning refusal: notFound=%v held=%v awaiting=%v",
			isGameNotFound(body), isHeld(body), isAwaitingFolder(body))
	}
	unread := errors.New(`peer returned 503: {"error":"` + ProvisioningUnreadableMessage + `"}`)
	if isGameNotFound(unread) || strings.Contains(strings.ToLower(unread.Error()), "not found") {
		t.Fatal("an unreadable hold looks like the game is missing")
	}
}
