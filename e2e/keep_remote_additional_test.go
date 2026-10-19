package e2e

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/opensave/opensave/testutil"
)

// These cases exercise explicit authority without changing ordinary
// bidirectional reconciliation, using disposable real loopback peers.
func TestKeepRemoteEqualMtimeExclusionsAndIdempotence(t *testing.T) {
	a, b, gameID, aConfig, bConfig := twoLocationPair(t, "KeepRemoteEqual",
		map[string]string{"settings.ini": "BASE"})
	pauseAutoSync(t, a, b, gameID)
	setIgnore(t, a, gameID, "device.ini")
	writeIn(t, aConfig, "settings.ini", "LOCAL")
	writeIn(t, bConfig, "settings.ini", "REMOTE")
	writeIn(t, aConfig, "local-only.ini", "LOCAL ONLY")
	writeIn(t, bConfig, "remote-only.ini", "REMOTE ONLY")
	writeIn(t, aConfig, "device.ini", "DEVICE SECRET")
	ts := time.Now().Add(-time.Hour)
	for _, p := range []string{filepath.Join(aConfig, "settings.ini"), filepath.Join(bConfig, "settings.ini")} {
		if err := os.Chtimes(p, ts, ts); err != nil {
			t.Fatal(err)
		}
	}
	primaryBefore := a.ReadSave("save.sav")
	time.Sleep(syncSettleWindow)
	syncTo(a, gameID, b.NodeID())
	if !testutil.WaitFor(30*time.Second, func() bool {
		return len(a.Daemon.P2P.Sync.ActiveRootConflicts()) > 0
	}) {
		t.Fatal("no conflict")
	}
	if err := a.Daemon.P2P.Sync.ResolveRootConflict(context.Background(), gameID, b.NodeID(), "config", "keep-remote"); err != nil {
		t.Fatal(err)
	}
	for p, want := range map[string]string{
		"settings.ini": "REMOTE", "remote-only.ini": "REMOTE ONLY", "device.ini": "DEVICE SECRET",
	} {
		if got := readIn(aConfig, p); got != want {
			t.Errorf("%s got %q want %q", p, got, want)
		}
	}
	if _, err := os.Stat(filepath.Join(aConfig, "local-only.ini")); !os.IsNotExist(err) {
		t.Errorf("local-only survived: %v", err)
	}
	if got := a.ReadSave("save.sav"); got != primaryBefore {
		t.Errorf("primary root modified: %q", got)
	}
	if got := readIn(bConfig, "settings.ini"); got != "REMOTE" {
		t.Errorf("peer bytes changed %q", got)
	}
	if got := len(a.Daemon.P2P.Sync.ActiveRootConflicts()); got != 0 {
		t.Errorf("conflict remained after verified adoption: %d", got)
	}
	syncTo(a, gameID, b.NodeID())
	if got := readIn(aConfig, "settings.ini"); got != "REMOTE" {
		t.Errorf("ordinary re-sync undid explicit choice: %q", got)
	}
}

func TestKeepRemoteCancelledBeforeMutationKeepsState(t *testing.T) {
	a, b, gameID, aConfig, bConfig := twoLocationPair(t, "KeepRemoteCancelled",
		map[string]string{"settings.ini": "BASE"})
	pauseAutoSync(t, a, b, gameID)
	writeIn(t, aConfig, "settings.ini", "LOCAL")
	writeIn(t, bConfig, "settings.ini", "REMOTE")
	time.Sleep(syncSettleWindow)
	syncTo(a, gameID, b.NodeID())
	if !testutil.WaitFor(30*time.Second, func() bool {
		return len(a.Daemon.P2P.Sync.ActiveRootConflicts()) > 0
	}) {
		t.Fatal("no conflict")
	}
	base := a.Daemon.Store.GetAgreedHashForRoot(gameID, b.NodeID(), "config")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := a.Daemon.P2P.Sync.ResolveRootConflict(ctx, gameID, b.NodeID(), "config", "keep-remote"); err == nil {
		t.Error("cancelled resolution returned success")
	}
	if got := readIn(aConfig, "settings.ini"); got != "LOCAL" {
		t.Errorf("local changed: %q", got)
	}
	if got := a.Daemon.Store.GetAgreedHashForRoot(gameID, b.NodeID(), "config"); got != base {
		t.Errorf("base mutated %q -> %q", base, got)
	}
	if len(a.Daemon.P2P.Sync.ActiveRootConflicts()) == 0 {
		t.Error("conflict lost")
	}
}

func TestKeepRemoteRestoresSelectedPeerAfterLocalDelete(t *testing.T) {
	a, b, gameID, aConfig, bConfig := twoLocationPair(t, "KeepRemoteDeleted",
		map[string]string{"settings.ini": "BASE", "shared.ini": "ORIGINAL"})
	pauseAutoSync(t, a, b, gameID)
	writeIn(t, aConfig, "settings.ini", "LOCAL")
	writeIn(t, bConfig, "settings.ini", "REMOTE")
	if err := os.Remove(filepath.Join(aConfig, "shared.ini")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(syncSettleWindow)
	syncTo(a, gameID, b.NodeID())
	if !testutil.WaitFor(30*time.Second, func() bool {
		return len(a.Daemon.P2P.Sync.ActiveRootConflicts()) > 0
	}) {
		t.Fatal("expected conflict")
	}
	if err := a.Daemon.P2P.Sync.ResolveRootConflict(context.Background(), gameID, b.NodeID(), "config", "keep-remote"); err != nil {
		t.Fatal(err)
	}
	if got := readIn(aConfig, "shared.ini"); got != "ORIGINAL" {
		t.Errorf("selected peer's deleted file not restored: %q", got)
	}
	if got := readIn(aConfig, "settings.ini"); got != "REMOTE" {
		t.Errorf("chosen peer lost: %q", got)
	}
	if got := readIn(bConfig, "settings.ini"); got != "REMOTE" {
		t.Errorf("peer modified: %q", got)
	}
}

// An excluded file inside a directory the peer does not have must stay, and
// must not make the verified adoption fail. The directory cannot be removed
// while the file is there; treating that leftover directory as a hash mismatch
// applied the peer's files and then reported failure.
func TestKeepRemoteKeepsExcludedFileInLocalOnlyDirectory(t *testing.T) {
	a, b, gameID, aConfig, bConfig := twoLocationPair(t, "KeepRemoteExclDir",
		map[string]string{"settings.ini": "BASE"})
	pauseAutoSync(t, a, b, gameID)
	setIgnore(t, a, gameID, "device.ini")
	writeIn(t, aConfig, "settings.ini", "LOCAL")
	writeIn(t, bConfig, "settings.ini", "REMOTE")
	writeIn(t, aConfig, "device-local/device.ini", "DEVICE SECRET")
	writeIn(t, aConfig, "only-local.ini", "UNSHARED")
	time.Sleep(syncSettleWindow)
	syncTo(a, gameID, b.NodeID())
	if !testutil.WaitFor(30*time.Second, func() bool {
		return len(a.Daemon.P2P.Sync.ActiveRootConflicts()) > 0
	}) {
		t.Fatal("no conflict")
	}
	if err := a.Daemon.P2P.Sync.ResolveRootConflict(context.Background(), gameID, b.NodeID(), "config", "keep-remote"); err != nil {
		t.Fatal(err)
	}
	if got := readIn(aConfig, "settings.ini"); got != "REMOTE" {
		t.Errorf("settings.ini = %q, want REMOTE", got)
	}
	if got := readIn(aConfig, "device-local/device.ini"); got != "DEVICE SECRET" {
		t.Errorf("excluded file lost: %q", got)
	}
	if _, err := os.Stat(filepath.Join(aConfig, "only-local.ini")); !os.IsNotExist(err) {
		t.Errorf("local-only file survived: %v", err)
	}
	if got := readIn(bConfig, "settings.ini"); got != "REMOTE" {
		t.Errorf("peer changed: %q", got)
	}
	if _, err := os.Stat(filepath.Join(bConfig, "device-local", "device.ini")); !os.IsNotExist(err) {
		t.Errorf("excluded file was pushed to the peer: %v", err)
	}
}

// After an explicit remote choice, ordinary resync must preserve the selected
// peer's files, the device-local exclusion, and the resolved conflict.
func TestKeepRemoteExcludedDirectoryThenOrdinaryResync(t *testing.T) {
	a, b, gameID, aConfig, bConfig := twoLocationPair(t, "KeepRemoteExcludedResync", map[string]string{"settings.ini": "BASE"})
	pauseAutoSync(t, a, b, gameID)
	setIgnore(t, a, gameID, "device.ini")
	writeIn(t, aConfig, "settings.ini", "LOCAL")
	writeIn(t, bConfig, "settings.ini", "REMOTE")
	writeIn(t, aConfig, "device-local/device.ini", "DEVICE SECRET")
	time.Sleep(syncSettleWindow)
	syncTo(a, gameID, b.NodeID())
	if !testutil.WaitFor(30*time.Second, func() bool { return len(a.Daemon.P2P.Sync.ActiveRootConflicts()) > 0 }) {
		t.Fatal("no conflict")
	}
	if err := a.Daemon.P2P.Sync.ResolveRootConflict(context.Background(), gameID, b.NodeID(), "config", "keep-remote"); err != nil {
		t.Fatal(err)
	}
	syncTo(a, gameID, b.NodeID())
	if got := readIn(aConfig, "settings.ini"); got != "REMOTE" {
		t.Fatalf("local reverted: %q", got)
	}
	if got := readIn(bConfig, "settings.ini"); got != "REMOTE" {
		t.Fatalf("remote overwritten: %q", got)
	}
	if got := readIn(aConfig, "device-local/device.ini"); got != "DEVICE SECRET" {
		t.Fatalf("excluded changed: %q", got)
	}
	if _, err := os.Stat(filepath.Join(bConfig, "device-local", "device.ini")); !os.IsNotExist(err) {
		t.Fatalf("excluded file propagated: %v", err)
	}
	if len(a.Daemon.P2P.Sync.ActiveRootConflicts()) != 0 {
		t.Fatal("conflict after resync")
	}
}

func TestKeepRemoteUsesExistingUnicodeDirectorySpelling(t *testing.T) {
	a, b, gameID, aConfig, bConfig := twoLocationPair(t, "KeepRemoteUnicode",
		map[string]string{"settings.ini": "BASE"})
	pauseAutoSync(t, a, b, gameID)
	writeIn(t, aConfig, "settings.ini", "LOCAL")
	writeIn(t, bConfig, "settings.ini", "REMOTE")
	writeIn(t, aConfig, "cafe\u0301/local-only.ini", "LOCAL ONLY")
	writeIn(t, bConfig, "caf\u00e9/selected.ini", "SELECTED")
	time.Sleep(syncSettleWindow)
	syncTo(a, gameID, b.NodeID())
	if !testutil.WaitFor(30*time.Second, func() bool {
		return len(a.Daemon.P2P.Sync.ActiveRootConflicts()) > 0
	}) {
		t.Fatal("no conflict")
	}
	if err := a.Daemon.P2P.Sync.ResolveRootConflict(
		context.Background(), gameID, b.NodeID(), "config", "keep-remote"); err != nil {
		t.Fatal(err)
	}
	if got := readIn(aConfig, "cafe\u0301/selected.ini"); got != "SELECTED" {
		t.Fatalf("selected Unicode path = %q, want SELECTED", got)
	}
	if _, err := os.Stat(filepath.Join(aConfig, "caf\u00e9")); !os.IsNotExist(err) {
		t.Fatalf("created duplicate normalized directory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(aConfig, "cafe\u0301", "local-only.ini")); !os.IsNotExist(err) {
		t.Fatalf("local-only Unicode path survived: %v", err)
	}
}
