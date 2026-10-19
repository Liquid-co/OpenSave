package store

import "testing"

// Identical-value SQLite UPSERTs must not be treated as an untracked game.
// Repeated explicit adoption can legitimately write exactly the same state.
func TestSetResolvedRootSyncStateIdempotentRetry(t *testing.T) {
	s := rootsTestStore(t)
	files := []string{"settings.ini"}
	dirs := []string{"userdata"}
	for attempt := 0; attempt < 3; attempt++ {
		if err := s.SetResolvedRootSyncState("elden-ring", "peer-a", "config", "chosen-hash", files, dirs); err != nil {
			t.Fatalf("idempotent attempt %d failed: %v", attempt+1, err)
		}
	}
	if hash := s.GetAgreedHashForRoot("elden-ring", "peer-a", "config"); hash != "chosen-hash" {
		t.Fatalf("agreed hash changed: %q", hash)
	}
	gotFiles, gotDirs, err := s.GetSyncStateForRoot("elden-ring", "peer-a", "config")
	if err != nil || len(gotFiles) != 1 || gotFiles[0] != files[0] || len(gotDirs) != 1 || gotDirs[0] != dirs[0] {
		t.Fatalf("lineage changed: files=%v dirs=%v err=%v", gotFiles, gotDirs, err)
	}
	if err := s.DeleteGame("elden-ring"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetResolvedRootSyncState("elden-ring", "peer-a", "config", "chosen-hash", files, dirs); err == nil {
		t.Fatal("resolution of an untracked game must be refused")
	}
}
