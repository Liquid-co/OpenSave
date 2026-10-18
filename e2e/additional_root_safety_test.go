package e2e

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/opensave/opensave/testutil"
)

// Regression tests for stock OpenSave extra-root safety. Two test peers exchange data
// over loopback HTTP. No actual user saves, peers or network credentials.
func auditBlockBackupE2E(t *testing.T, d *testutil.TestDaemon) {
	t.Helper()
	settings, err := d.Daemon.Store.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "not-a-backup-directory")
	if err := os.WriteFile(file, []byte("block"), 0600); err != nil {
		t.Fatal(err)
	}
	settings.BackupsDir = file
	if err := d.Daemon.Store.UpdateSettings(settings); err != nil {
		t.Fatal(err)
	}
}

// F03: A one-sided remote update must not overwrite the local extra root
// when the protective snapshot cannot be written.
func TestAuditLive_ExtraRootPullRefusesToOverwriteWithoutSafetyBackup(t *testing.T) {
	a, b, gameID, aConfig, bConfig := twoLocationPair(t, "AuditExtraRootPull", map[string]string{"settings.ini": "BASE"})
	pauseAutoSync(t, a, b, gameID)
	writeIn(t, aConfig, "settings.ini", "REMOTE NEW")
	time.Sleep(syncSettleWindow)
	auditBlockBackupE2E(t, b)
	syncTo(b, gameID, a.NodeID())
	if got := readIn(bConfig, "settings.ini"); got != "BASE" {
		t.Errorf("DATA LOSS: extra-root config overwritten without usable safety backup: %q", got)
	}
}

// F03: Propagated deletions need the same safety guard as file replacement.
func TestAuditLive_ExtraRootDeleteRefusesWithoutSafetyBackup(t *testing.T) {
	a, b, gameID, aConfig, bConfig := twoLocationPair(t, "AuditExtraRootDeletion",
		map[string]string{"settings.ini": "KEEP", "critical.ini": "BASE"})
	pauseAutoSync(t, a, b, gameID)
	if err := os.Remove(filepath.Join(aConfig, "critical.ini")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(syncSettleWindow)
	auditBlockBackupE2E(t, b)
	syncTo(b, gameID, a.NodeID())
	if got := readIn(bConfig, "critical.ini"); got != "BASE" {
		t.Errorf("DATA LOSS: extra-root config deleted without usable safety backup: %q", got)
	}
}

// F04: keep-remote must not discard unique local bytes after backup failure.
func TestAuditLive_KeepRemoteConflictMustStopWhenBackupFails(t *testing.T) {
	a, b, gameID, aConfig, bConfig := twoLocationPair(t, "AuditExtraRootConflict",
		map[string]string{"settings.ini": "BASE"})
	pauseAutoSync(t, a, b, gameID)
	writeIn(t, aConfig, "settings.ini", "LOCAL UNIQUE")
	writeIn(t, bConfig, "settings.ini", "PEER UNIQUE")
	time.Sleep(syncSettleWindow)
	syncTo(a, gameID, b.NodeID())
	if !testutil.WaitFor(30*time.Second,
		func() bool { return len(a.Daemon.P2P.Sync.ActiveRootConflicts()) > 0 }) {
		t.Fatal("precondition: two-sided root conflict not raised")
	}
	auditBlockBackupE2E(t, a)
	err := a.Daemon.P2P.Sync.ResolveRootConflict(context.Background(), gameID, b.NodeID(), "config", "keep-remote")
	if err == nil {
		t.Error("UNSAFE: keep-remote succeeded despite failed backup")
	}
	if got := readIn(aConfig, "settings.ini"); got != "LOCAL UNIQUE" {
		t.Errorf("DATA LOSS: unique local config overwritten without safety snapshot: %q", got)
	}
}

// F04: keep-local must also respect a failed requested undo snapshot before
// changing peer reconciliation metadata. Immediate local-byte loss is not claimed.
func TestAuditLive_KeepLocalConflictRequiresSafetySnapshot(t *testing.T) {
	a, b, gameID, aConfig, bConfig := twoLocationPair(t, "AuditExtraRootLocal",
		map[string]string{"settings.ini": "BASE"})
	pauseAutoSync(t, a, b, gameID)
	writeIn(t, aConfig, "settings.ini", "LOCAL UNIQUE")
	writeIn(t, bConfig, "settings.ini", "PEER UNIQUE")
	time.Sleep(syncSettleWindow)
	syncTo(a, gameID, b.NodeID())
	if !testutil.WaitFor(30*time.Second,
		func() bool { return len(a.Daemon.P2P.Sync.ActiveRootConflicts()) > 0 }) {
		t.Fatal("precondition: root conflict not raised")
	}
	auditBlockBackupE2E(t, a)
	err := a.Daemon.P2P.Sync.ResolveRootConflict(context.Background(), gameID, b.NodeID(), "config", "keep-local")
	if err == nil {
		t.Error("UNSAFE: keep-local resolved and advanced state despite failed backup")
	}
}

func TestAuditLive_KeepRemoteFetchFailureMustRetainConflictAndLineage(t *testing.T) {
	a, b, gameID, aConfig, bConfig := twoLocationPair(t, "AuditRootFetchFailure",
		map[string]string{"settings.ini": "BASE"})
	pauseAutoSync(t, a, b, gameID)
	writeIn(t, aConfig, "settings.ini", "LOCAL UNIQUE")
	writeIn(t, bConfig, "settings.ini", "PEER UNIQUE")
	time.Sleep(syncSettleWindow)
	syncTo(a, gameID, b.NodeID())
	if !testutil.WaitFor(30*time.Second,
		func() bool { return len(a.Daemon.P2P.Sync.ActiveRootConflicts()) > 0 }) {
		t.Fatal("precondition: root conflict not raised")
	}
	beforeBase := a.Daemon.Store.GetAgreedHashForRoot(gameID, b.NodeID(), "config")
	beforeFiles, beforeDirs, err := a.Daemon.Store.GetSyncStateForRoot(gameID, b.NodeID(), "config")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := a.Daemon.P2P.Sync.ResolveRootConflict(ctx, gameID, b.NodeID(), "config", "keep-remote"); err == nil {
		t.Error("cancelled peer fetch unexpectedly succeeded")
	}
	if got := len(a.Daemon.P2P.Sync.ActiveRootConflicts()); got == 0 {
		t.Error("a failed remote fetch erased the active conflict")
	}
	if got := a.Daemon.Store.GetAgreedHashForRoot(gameID, b.NodeID(), "config"); got != beforeBase {
		t.Errorf("agreed base changed after cancelled fetch: before %q; after %q", beforeBase, got)
	}
	afterFiles, afterDirs, err := a.Daemon.Store.GetSyncStateForRoot(gameID, b.NodeID(), "config")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(beforeFiles, afterFiles) || !reflect.DeepEqual(beforeDirs, afterDirs) {
		t.Error("root lineage changed after cancelled fetch")
	}
	if got := readIn(aConfig, "settings.ini"); got != "LOCAL UNIQUE" {
		t.Errorf("local bytes changed after cancelled fetch: %q", got)
	}
}

func TestAuditLive_KeepLocalFetchFailureRetainsConflictAndLineage(t *testing.T) {
	a, b, gameID, aConfig, bConfig := twoLocationPair(t, "AuditRootKeepLocalFetchFailure",
		map[string]string{"settings.ini": "BASE"})
	pauseAutoSync(t, a, b, gameID)
	writeIn(t, aConfig, "settings.ini", "LOCAL UNIQUE")
	writeIn(t, bConfig, "settings.ini", "PEER UNIQUE")
	time.Sleep(syncSettleWindow)
	syncTo(a, gameID, b.NodeID())
	if !testutil.WaitFor(30*time.Second,
		func() bool { return len(a.Daemon.P2P.Sync.ActiveRootConflicts()) > 0 }) {
		t.Fatal("precondition: root conflict not raised")
	}
	beforeBase := a.Daemon.Store.GetAgreedHashForRoot(gameID, b.NodeID(), "config")
	beforeFiles, beforeDirs, err := a.Daemon.Store.GetSyncStateForRoot(gameID, b.NodeID(), "config")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := a.Daemon.P2P.Sync.ResolveRootConflict(ctx, gameID, b.NodeID(), "config", "keep-local"); err == nil {
		t.Error("cancelled peer fetch unexpectedly succeeded")
	}
	if got := len(a.Daemon.P2P.Sync.ActiveRootConflicts()); got == 0 {
		t.Error("a failed local-resolution fetch erased the active conflict")
	}
	if got := a.Daemon.Store.GetAgreedHashForRoot(gameID, b.NodeID(), "config"); got != beforeBase {
		t.Errorf("agreed base changed after cancelled fetch: before %q; after %q", beforeBase, got)
	}
	afterFiles, afterDirs, err := a.Daemon.Store.GetSyncStateForRoot(gameID, b.NodeID(), "config")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(beforeFiles, afterFiles) || !reflect.DeepEqual(beforeDirs, afterDirs) {
		t.Error("root lineage changed after cancelled local-resolution fetch")
	}
	if got := readIn(aConfig, "settings.ini"); got != "LOCAL UNIQUE" {
		t.Errorf("local bytes changed after cancelled fetch: %q", got)
	}
}

func TestAuditLive_ExtraRootNewFileNeedsNoSafetySnapshot(t *testing.T) {
	a, b, gameID, aConfig, bConfig := twoLocationPair(t, "AuditNewRootFile",
		map[string]string{"existing.ini": "BASE"})
	pauseAutoSync(t, a, b, gameID)
	writeIn(t, aConfig, "new.ini", "NEW FILE")
	time.Sleep(syncSettleWindow)
	auditBlockBackupE2E(t, b)
	syncTo(b, gameID, a.NodeID())
	if got := readIn(bConfig, "new.ini"); got != "NEW FILE" {
		t.Errorf("new file incorrectly blocked by unavailable safety snapshot: %q", got)
	}
}
