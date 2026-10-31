package syncengine

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestTopologyMutationExcludesActiveWritersAndReaders(t *testing.T) {
	e := &Engine{}
	releaseWriter := e.Writing("game")
	if _, ok := e.TryTopologyMutation("game"); ok {
		t.Fatal("topology admitted while writer was active")
	}
	releaseWriter()
	releaseReader, ok := e.TryReading("game")
	if !ok {
		t.Fatal("could not establish reader fixture")
	}
	if _, ok := e.TryTopologyMutation("game"); ok {
		t.Fatal("topology admitted while reader was active")
	}
	releaseReader()
	releaseTopology, ok := e.TryTopologyMutation("game")
	if !ok {
		t.Fatal("topology reservation should be available after drain")
	}
	if _, ok := e.TryTopologyMutation("game"); ok {
		t.Fatal("competing topology mutation was admitted")
	}
	if done, ok := e.TryReading("game"); ok {
		done()
		t.Fatal("reader entered during topology mutation")
	}
	releaseTopology()
	releaseTopology()
	if done, ok := e.TryReading("game"); !ok {
		t.Fatal("reader remained blocked after topology release")
	} else {
		done()
	}
}

func TestTopologyMutationKeepsNewWriterOutUntilRelease(t *testing.T) {
	e := &Engine{}
	releaseTopology, ok := e.TryTopologyMutation("game")
	if !ok {
		t.Fatal("cannot reserve idle game")
	}
	acquired := make(chan struct{})
	go func() {
		releaseWrite := e.Writing("game")
		close(acquired)
		releaseWrite()
	}()
	select {
	case <-acquired:
		t.Fatal("native writer entered during topology mutation")
	case <-time.After(25 * time.Millisecond):
	}
	releaseTopology()
	select {
	case <-acquired:
	case <-time.After(2 * time.Second):
		t.Fatal("writer never resumed after topology release")
	}
}

func TestTopologyMutationQueuesSyncForRetry(t *testing.T) {
	e := &Engine{Log: func(string, string) {}}
	release, ok := e.TryTopologyMutation("game")
	if !ok {
		t.Fatal("cannot reserve idle game")
	}
	_, err := e.SyncGame(context.Background(), "game", nil)
	if !errors.Is(err, ErrSyncQueued) {
		t.Fatalf("sync during topology = %v, want queued", err)
	}
	e.mu.Lock()
	queued := e.pendingSyncs["game"]
	e.mu.Unlock()
	if !queued {
		t.Fatal("topology-refused sync intent was lost")
	}
	release()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		e.mu.Lock()
		pending := e.pendingSyncs["game"] || e.followUps["game"] > 0
		e.mu.Unlock()
		if !pending {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("queued follow-up was not drained after topology release")
}
