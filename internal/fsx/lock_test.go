package fsx

import (
	"path/filepath"
	"testing"
)

func TestTryLockIsExclusiveUntilReleased(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock")
	unlock, ok, err := TryLock(path)
	if err != nil || !ok {
		t.Fatalf("first lock: ok=%v err=%v", ok, err)
	}
	if _, ok, err := TryLock(path); err != nil || ok {
		t.Fatalf("second lock while held: ok=%v err=%v, want refused", ok, err)
	}
	unlock()
	again, ok, err := TryLock(path)
	if err != nil || !ok {
		t.Fatalf("lock after release: ok=%v err=%v", ok, err)
	}
	again()
}
