package syncengine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

// A transfer that stops part-way must never turn into deletions.
//
// Seen in 2.3.1: a device that had received part of a 240k-file save offered
// its partial copy back, the missing files were read as "deleted over there",
// and 23k files were deleted on the device that still had them. 2.4 records
// as shared only what both manifests showed (persistLineage), which should
// rule that out; these hold it there, for the per-file and the batched pull.

func countFiles(t *testing.T, dir string) int {
	t.Helper()
	n := 0
	_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			n++
		}
		return nil
	})
	return n
}

func writeSmallFiles(t *testing.T, dir string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		write(t, dir, fmt.Sprintf("world/map_%04d.bin", i), fmt.Sprintf("chunk %d", i))
	}
}

func interruptedThenResumed(t *testing.T, env *engineEnv, total, stopAfter int) {
	t.Helper()
	writeSmallFiles(t, env.remoteDir, total)

	ctx, cancel := context.WithCancel(context.Background())
	var fetched atomic.Int64
	env.transport.onFetchBlocks = func() {
		if fetched.Add(1) == int64(stopAfter) {
			cancel()
		}
	}
	if _, err := env.engine.SyncWithPeer(ctx, "game1", env.peer); err == nil {
		t.Fatal("setup: the first sync was meant to be interrupted")
	}
	env.transport.onFetchBlocks = nil
	cancel()

	got := countFiles(t, env.localDir)
	if got == 0 || got >= total {
		t.Fatalf("setup: after the interruption this side has %d of %d files; want a partial copy", got, total)
	}

	// Resume. Nothing may be deleted anywhere, and everything must arrive.
	if _, err := env.engine.SyncWithPeer(context.Background(), "game1", env.peer); err != nil {
		t.Fatalf("resumed sync: %v", err)
	}
	if len(env.transport.deletedOnPeer) > 0 {
		t.Fatalf("the partial copy was propagated as %d deletions on the peer, e.g. %s",
			len(env.transport.deletedOnPeer), env.transport.deletedOnPeer[0])
	}
	if n := countFiles(t, env.remoteDir); n != total {
		t.Fatalf("peer has %d files, want all %d", n, total)
	}
	if n := countFiles(t, env.localDir); n != total {
		t.Fatalf("this side has %d files after resuming, want %d", n, total)
	}
}

func TestPartialPull_InterruptedThenResumed(t *testing.T) {
	env := setupEngine(t)
	interruptedThenResumed(t, env, 200, 60)
}

func TestPartialPull_InterruptedThenResumed_Batched(t *testing.T) {
	// One batch in flight, so the first batch of 256 is written before the
	// interruption lands in the second: a deterministic partial copy.
	old := batchWorkers
	batchWorkers = 1
	t.Cleanup(func() { batchWorkers = old })
	env, _ := setupBatchEngine(t, ProtoBatchFiles)
	// Batches fetch through the same fake reader, one call per file; the
	// 300th is inside the second batch.
	interruptedThenResumed(t, env, 600, 300)
}

// The other side of the same situation: this device has everything, the peer
// holds only part of it and never synced. The part it lacks is new to it, not
// deleted by it.
func TestPartialPeer_NeverReadAsDeletions(t *testing.T) {
	env := setupEngine(t)
	writeSmallFiles(t, env.localDir, 300)
	for i := 0; i < 80; i++ {
		write(t, env.remoteDir, fmt.Sprintf("world/map_%04d.bin", i), fmt.Sprintf("chunk %d", i))
	}

	if _, err := env.engine.SyncWithPeer(context.Background(), "game1", env.peer); err != nil {
		t.Fatalf("SyncWithPeer: %v", err)
	}
	if n := countFiles(t, env.localDir); n != 300 {
		t.Fatalf("this side went from 300 to %d files — the peer's partial copy was read as deletions", n)
	}
	if len(env.transport.deletedOnPeer) > 0 {
		t.Fatalf("deleted on peer: %v", env.transport.deletedOnPeer[:1])
	}
}
