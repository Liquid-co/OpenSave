package syncengine

import (
	"context"
	"fmt"
	"testing"
)

// "Keep mine" against a peer holding part of the save, where the peer never
// gets to pull (a pull that times out, a device that goes away). The next
// sync must not read the peer's missing files as deletions and remove them
// from the device whose version the person chose to keep.
func TestKeepLocal_PeerThatNeverPulled_DeletesNothingHere(t *testing.T) {
	env := setupEngine(t)
	for i := 0; i < 300; i++ {
		write(t, env.localDir, fmt.Sprintf("world/map_%04d.bin", i), fmt.Sprintf("chunk %d", i))
	}
	for i := 0; i < 100; i++ {
		write(t, env.remoteDir, fmt.Sprintf("world/map_%04d.bin", i), fmt.Sprintf("chunk %d", i))
	}

	// The person answers the conflict with "Keep mine".
	if err := env.engine.markResolvedLocal(context.Background(), "game1", env.peer); err != nil {
		t.Fatal(err)
	}
	// The peer's pull never happens; its folder stays as it was.

	if _, err := env.engine.SyncWithPeer(context.Background(), "game1", env.peer); err != nil {
		t.Fatalf("SyncWithPeer: %v", err)
	}
	if n := countFiles(t, env.localDir); n != 300 {
		t.Fatalf("after \"keep mine\" this device went from 300 to %d files — the peer's partial copy was read as deletions", n)
	}
}
