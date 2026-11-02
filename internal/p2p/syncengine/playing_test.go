package syncengine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// A game being played here is not synced either way until the session ends:
// nothing is taken for it, and callers are not told it failed.
func TestPlaying_NotSyncedWhilePlayedHere(t *testing.T) {
	env := setupEngine(t)
	write(t, env.localDir, "slot1.sav", "being played")
	write(t, env.remoteDir, "slot1.sav", "another device's")
	playing := true
	env.engine.Playing = func(string) bool { return playing }

	_, err := env.engine.SyncGame(context.Background(), "game1", []Peer{env.peer})
	if !errors.Is(err, ErrPlayingHere) || !errors.Is(err, ErrHeld) {
		t.Fatalf("err = %v, want ErrPlayingHere, quiet as ErrHeld", err)
	}
	if got, _ := os.ReadFile(filepath.Join(env.localDir, "slot1.sav")); string(got) != "being played" {
		t.Fatalf("the save of a running game was replaced: %q", got)
	}

	playing = false
	if _, err := env.engine.SyncGame(context.Background(), "game1", []Peer{env.peer}); err != nil {
		t.Fatalf("after the session: %v", err)
	}
}

// Another device playing the game answers "playing": nothing is compared or
// stamped, and asking does not count as a failure.
func TestPlaying_PeerPlayingIsWaitedFor(t *testing.T) {
	env := setupEngine(t)
	write(t, env.localDir, "slot1.sav", "here")
	env.transport.manifestErr = errors.New(PlayingMessage)

	res, err := env.engine.SyncWithPeer(context.Background(), "game1", env.peer)
	if err != nil || res.Status != "peer_playing" {
		t.Fatalf("status = %q, %v; want peer_playing", res.Status, err)
	}
	// A device on an older build reads the same answer as "busy".
	if !isSettling(errors.New(PlayingMessage)) {
		t.Error("the playing answer is not recognisable as busy by older builds")
	}
}
