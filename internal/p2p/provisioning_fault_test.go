package p2p

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/opensave/opensave/internal/store"
	"github.com/opensave/opensave/internal/syncpause"
)

// SyncAllGames used to ignore a failed hold-set read and sync every AutoSync
// game. With a peer online and the game row still readable, that is a transfer.
// Sync is nil here: reaching it panics, which is the failure this test wants
// if the early return is removed.
func TestSyncAllGamesStopsWhenHoldSetUnreadable(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "opensave.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.EnsureDefaultSettings(t.TempDir(), t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateGame(store.Game{
		ID: "live", Name: "Live", SavePath: t.TempDir(), ActiveBranch: "main",
		AutoSync: true, MaxSnapshots: 5,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertPeer(store.Peer{
		ID: "peer", Name: "Peer", Address: "127.0.0.1", Port: 9, Status: "online",
	}); err != nil {
		t.Fatal(err)
	}
	s.SetProvisioningReadFault(errors.New("injected hold read failure"))
	e := &Engine{Store: s, Pause: syncpause.New(), Log: func(string, string) {}}
	e.SyncAllGames(context.Background())
}
