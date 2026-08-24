//go:build windows

package snapshot

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows/registry"

	"github.com/opensave/opensave/internal/winreg"
)

// scratchRegistryKey creates a key of our own under HKCU and removes it
// afterwards. No test ever writes to a key a real game owns.
func scratchRegistryKey(t *testing.T) (fullPath, sub string) {
	t.Helper()
	sub = `Software\OpenSaveTest\snap` + strconv.FormatInt(time.Now().UnixNano(), 36)
	k, _, err := registry.CreateKey(registry.CURRENT_USER, sub, registry.WRITE)
	if err != nil {
		t.Fatalf("creating scratch key: %v", err)
	}
	k.Close()
	t.Cleanup(func() { _ = registry.DeleteKey(registry.CURRENT_USER, sub) })
	return `HKEY_CURRENT_USER\` + sub, sub
}

func setRegistryValue(t *testing.T, sub, name string, val uint32) {
	t.Helper()
	h, _, err := registry.CreateKey(registry.CURRENT_USER, sub, registry.WRITE)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	if err := h.SetDWordValue(name, val); err != nil {
		t.Fatal(err)
	}
}

func readRegistryValue(t *testing.T, sub, name string) (uint64, bool) {
	t.Helper()
	h, err := registry.OpenKey(registry.CURRENT_USER, sub, registry.READ)
	if err != nil {
		return 0, false
	}
	defer h.Close()
	v, _, err := h.GetIntegerValue(name)
	if err != nil {
		return 0, false
	}
	return v, true
}

// The whole point of the feature: a game whose progress lives in the registry
// gets that progress into its snapshot and back out again. 303 games in the
// manifest have nowhere else to keep a save.
func TestRegistrySaveSurvivesSnapshotAndRestore(t *testing.T) {
	env := setup(t)
	keyPath, sub := scratchRegistryKey(t)
	setRegistryValue(t, sub, "level", 7)

	// Tracked the way the daemon sets it up: keys recorded, and a location
	// pointing at the directory the capture is materialised into.
	if err := env.store.SetGameRegistryKeys("game1", []string{keyPath}); err != nil {
		t.Fatal(err)
	}
	regDir := RegistryDir(env.backups, "game1")
	if err := os.MkdirAll(regDir, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := env.store.AddGameRoot("game1", winreg.LocationName, regDir); err != nil {
		t.Fatal(err)
	}
	writeSave(t, env.saveDir, "save.dat", "file half")

	snap, err := env.mgr.Create("game1", "with registry", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// The capture has to be inside the archive, under the locations prefix.
	found := false
	zr, err := zip.OpenReader(snap.ZipPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range zr.File {
		if strings.Contains(f.Name, winreg.FileName) {
			found = true
		}
	}
	zr.Close()
	if !found {
		t.Fatal("the snapshot archive holds no registry capture")
	}

	// The game moves on, then the user rolls back.
	setRegistryValue(t, sub, "level", 99)
	if _, err := env.mgr.Restore("game1", snap.ID); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	got, ok := readRegistryValue(t, sub, "level")
	if !ok {
		t.Fatal("the registry value is gone after a restore")
	}
	if got != 7 {
		t.Errorf("level = %d after restoring a snapshot taken at 7 — the registry half was not restored", got)
	}
}

// A game with no registry keys must produce exactly the archive it always did.
func TestAGameWithNoRegistryKeysIsUnchanged(t *testing.T) {
	env := setup(t)
	writeSave(t, env.saveDir, "save.dat", "just files")

	snap, err := env.mgr.Create("game1", "no registry", false)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(snap.ZipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if strings.Contains(f.Name, winreg.FileName) {
			t.Errorf("a game with no registry keys gained a capture entry: %s", f.Name)
		}
	}
}

// The capture must be refreshed at snapshot time. A stale file would archive
// whatever the registry held when the game was tracked, not when it was played.
func TestTheCaptureIsRefreshedAtSnapshotTime(t *testing.T) {
	env := setup(t)
	keyPath, sub := scratchRegistryKey(t)
	setRegistryValue(t, sub, "level", 1)

	if err := env.store.SetGameRegistryKeys("game1", []string{keyPath}); err != nil {
		t.Fatal(err)
	}
	regDir := RegistryDir(env.backups, "game1")
	if err := os.MkdirAll(regDir, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := env.store.AddGameRoot("game1", winreg.LocationName, regDir); err != nil {
		t.Fatal(err)
	}
	writeSave(t, env.saveDir, "save.dat", "x")
	if _, err := env.mgr.Create("game1", "first", false); err != nil {
		t.Fatal(err)
	}

	// Play a bit more, snapshot again, and the second capture must differ.
	setRegistryValue(t, sub, "level", 2)
	if _, err := env.mgr.Create("game1", "second", false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(regDir, winreg.FileName))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"data": "2"`) {
		t.Errorf("the capture was not refreshed before the second snapshot: %s", raw)
	}
}
