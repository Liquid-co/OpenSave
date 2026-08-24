//go:build windows

package snapshot

import (
	"archive/zip"
	"os"
	"strings"
	"testing"

	"github.com/opensave/opensave/internal/winreg"
)

// A registry-only game: nothing in its save folder, ever. 303 games are this.
func TestProbeRegistryOnlyGame(t *testing.T) {
	env := setup(t)
	keyPath, sub := scratchRegistryKey(t)
	setRegistryValue(t, sub, "progress", 55)

	if err := env.store.SetGameRegistryKeys("game1", []string{keyPath}); err != nil {
		t.Fatal(err)
	}
	regDir := RegistryDir(env.backups, "game1")
	os.MkdirAll(regDir, 0o777)
	if err := env.store.AddGameRoot("game1", winreg.LocationName, regDir); err != nil {
		t.Fatal(err)
	}
	// saveDir exists but is EMPTY — no files at all, which is the real shape.

	snap, err := env.mgr.Create("game1", "registry only", false)
	t.Logf("Create with an empty save folder: err=%v", err)
	if err != nil {
		t.Fatalf("a registry-only game could not be snapshotted: %v", err)
	}
	zr, zerr := zip.OpenReader(snap.ZipPath)
	if zerr != nil {
		t.Fatal(zerr)
	}
	names := []string{}
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	zr.Close()
	t.Logf("archive entries: %v", names)

	found := false
	for _, n := range names {
		if strings.Contains(n, winreg.FileName) {
			found = true
		}
	}
	if !found {
		t.Error("a registry-only game's snapshot holds no registry capture — nothing was backed up")
	}

	// And the restore has to put it back.
	setRegistryValue(t, sub, "progress", 1)
	if _, err := env.mgr.Restore("game1", snap.ID); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if got, ok := readRegistryValue(t, sub, "progress"); !ok || got != 55 {
		t.Errorf("progress = %d (ok=%v) after restore, want 55", got, ok)
	}
}
