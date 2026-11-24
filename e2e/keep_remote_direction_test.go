package e2e

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/opensave/opensave/testutil"
)

// A deliberate a deliberate "keep remote" choice must not be
// reinterpreted as ordinary mtime-based bidirectional reconciliation.
func TestKeepRemoteWinsDespiteNewerLocalMtime(t *testing.T) {
	a, b, gameID, aConfig, bConfig := twoLocationPair(t, "KeepRemoteMtime",
		map[string]string{"settings.ini": "BASE"})
	pauseAutoSync(t, a, b, gameID)
	writeIn(t, aConfig, "settings.ini", "A-LOCAL")
	writeIn(t, bConfig, "settings.ini", "B-REMOTE")
	writeIn(t, aConfig, "only-local.ini", "UNSHARED-LOCAL")
	later := time.Now().Add(2 * time.Hour)
	if err := os.Chtimes(filepath.Join(aConfig, "settings.ini"), later, later); err != nil {
		t.Fatal(err)
	}
	time.Sleep(syncSettleWindow)
	syncTo(a, gameID, b.NodeID())
	if !testutil.WaitFor(30*time.Second, func() bool {
		return len(a.Daemon.P2P.Sync.ActiveRootConflicts()) > 0
	}) {
		t.Fatal("setup: expected an additional-root conflict")
	}

	err := a.Daemon.P2P.Sync.ResolveRootConflict(
		context.Background(), gameID, b.NodeID(), "config", "keep-remote")
	if err != nil {
		t.Fatalf("keeping remote failed: %v", err)
	}
	if got := readIn(aConfig, "settings.ini"); got != "B-REMOTE" {
		t.Errorf("keep-remote selected local newer mtime instead: got %q, want B-REMOTE", got)
	}
	if _, err := os.Stat(filepath.Join(aConfig, "only-local.ini")); !os.IsNotExist(err) {
		t.Errorf("keep-remote left local-only file instead of adopting remote exact folder; stat=%v", err)
	}
	if got := readIn(bConfig, "settings.ini"); got != "B-REMOTE" {
		t.Errorf("keep-remote unexpectedly changed peer's authoritative bytes: %q", got)
	}
}
