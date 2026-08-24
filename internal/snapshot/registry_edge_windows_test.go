//go:build windows

package snapshot

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows/registry"

	"github.com/opensave/opensave/internal/winreg"
)

// trackRegistryGame sets a game up the way the daemon does.
func trackRegistryGame(t *testing.T, env *testEnv, keyPath string) string {
	t.Helper()
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
	return regDir
}

func archiveNames(t *testing.T, zipPath string) []string {
	t.Helper()
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	var out []string
	for _, f := range zr.File {
		out = append(out, f.Name)
	}
	return out
}

// The case the whole feature exists for: 303 games keep their save nowhere but
// the registry, so the save folder is empty forever and a file-only backup of
// one captures nothing at all.
func TestARegistryOnlyGameIsFullyBackedUpAndRestored(t *testing.T) {
	env := setup(t)
	keyPath, sub := scratchRegistryKey(t)
	setRegistryValue(t, sub, "progress", 55)
	trackRegistryGame(t, env, keyPath)
	// Save folder deliberately left empty.

	snap, err := env.mgr.Create("game1", "registry only", false)
	if err != nil {
		t.Fatalf("a registry-only game could not be snapshotted: %v", err)
	}
	names := archiveNames(t, snap.ZipPath)
	found := false
	for _, n := range names {
		if strings.Contains(n, winreg.FileName) {
			found = true
		}
	}
	if !found {
		t.Fatalf("nothing was backed up for a registry-only game; archive holds %v", names)
	}

	setRegistryValue(t, sub, "progress", 1)
	if _, err := env.mgr.Restore("game1", snap.ID); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if got, ok := readRegistryValue(t, sub, "progress"); !ok || got != 55 {
		t.Errorf("progress = %d (present %v), want 55", got, ok)
	}
}

// A snapshot can arrive from a peer that has the registry location while this
// device does not. Restoring it must not fail, and must not half-write the
// registry on the way past.
func TestRestoringASnapshotWhoseRegistryLocationIsUnmappedIsSafe(t *testing.T) {
	env := setup(t)
	keyPath, sub := scratchRegistryKey(t)
	setRegistryValue(t, sub, "level", 3)
	trackRegistryGame(t, env, keyPath)
	writeSave(t, env.saveDir, "save.dat", "files")

	snap, err := env.mgr.Create("game1", "with registry", false)
	if err != nil {
		t.Fatal(err)
	}

	// The device forgets where the registry location lives, as a peer that
	// never had one would be.
	if err := env.store.AddGameRoot("game1", winreg.LocationName, ""); err != nil {
		t.Fatal(err)
	}
	setRegistryValue(t, sub, "level", 9)

	if _, err := env.mgr.Restore("game1", snap.ID); err != nil {
		t.Fatalf("restoring with an unmapped registry location failed: %v", err)
	}
	// The files came back; the registry legitimately did not, and the value has
	// to be untouched rather than partly written.
	if got, _ := readRegistryValue(t, sub, "level"); got != 9 {
		t.Errorf("level = %d — an unmapped location must leave the registry alone", got)
	}
}

// The default (unnamed) value is a real place to keep a save, and an empty
// value name is the easiest one to drop.
func TestTheDefaultValueRoundTrips(t *testing.T) {
	env := setup(t)
	keyPath, sub := scratchRegistryKey(t)
	h, _, err := registry.CreateKey(registry.CURRENT_USER, sub, registry.WRITE)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.SetStringValue("", "the default value"); err != nil {
		t.Fatal(err)
	}
	h.Close()
	trackRegistryGame(t, env, keyPath)

	snap, err := env.mgr.Create("game1", "default value", false)
	if err != nil {
		t.Fatal(err)
	}
	hw, _ := registry.OpenKey(registry.CURRENT_USER, sub, registry.WRITE)
	_ = hw.SetStringValue("", "clobbered")
	hw.Close()

	if _, err := env.mgr.Restore("game1", snap.ID); err != nil {
		t.Fatal(err)
	}
	hr, err := registry.OpenKey(registry.CURRENT_USER, sub, registry.READ)
	if err != nil {
		t.Fatal(err)
	}
	defer hr.Close()
	if got, _, _ := hr.GetStringValue(""); got != "the default value" {
		t.Errorf("default value = %q, want it restored", got)
	}
}

// Save data is not ASCII. A player name in another script has to survive the
// JSON round trip and come back unchanged.
func TestUnicodeValuesSurvive(t *testing.T) {
	env := setup(t)
	keyPath, sub := scratchRegistryKey(t)
	name := "プレイヤー"
	h, _, _ := registry.CreateKey(registry.CURRENT_USER, sub, registry.WRITE)
	_ = h.SetStringValue("playerName", name)
	h.Close()
	trackRegistryGame(t, env, keyPath)

	snap, err := env.mgr.Create("game1", "unicode", false)
	if err != nil {
		t.Fatal(err)
	}
	hw, _ := registry.OpenKey(registry.CURRENT_USER, sub, registry.WRITE)
	_ = hw.SetStringValue("playerName", "wiped")
	hw.Close()

	if _, err := env.mgr.Restore("game1", snap.ID); err != nil {
		t.Fatal(err)
	}
	hr, _ := registry.OpenKey(registry.CURRENT_USER, sub, registry.READ)
	defer hr.Close()
	if got, _, _ := hr.GetStringValue("playerName"); got != name {
		t.Errorf("playerName = %q, want %q", got, name)
	}
}

// Uninstalling a game removes its key, and restoring a save is exactly what
// someone does next. A deleted key has to be recreated, not skipped.
func TestARestoreRecreatesAKeyThatWasDeleted(t *testing.T) {
	env := setup(t)
	keyPath, sub := scratchRegistryKey(t)
	setRegistryValue(t, sub, "coins", 500)
	trackRegistryGame(t, env, keyPath)

	snap, err := env.mgr.Create("game1", "before uninstall", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.DeleteKey(registry.CURRENT_USER, sub); err != nil {
		t.Fatal(err)
	}
	if _, ok := readRegistryValue(t, sub, "coins"); ok {
		t.Fatal("the key was not actually deleted")
	}

	if _, err := env.mgr.Restore("game1", snap.ID); err != nil {
		t.Fatalf("Restore after the key was deleted: %v", err)
	}
	if got, ok := readRegistryValue(t, sub, "coins"); !ok || got != 500 {
		t.Errorf("coins = %d (present %v) — a deleted key must be recreated", got, ok)
	}
}

// Games share one key between saves and settings. A restore puts back what was
// captured; it does not make the key identical to the moment of capture, or it
// would roll back a setting nobody asked about.
func TestRestoreLeavesValuesItNeverCapturedAlone(t *testing.T) {
	env := setup(t)
	keyPath, sub := scratchRegistryKey(t)
	setRegistryValue(t, sub, "save", 1)
	trackRegistryGame(t, env, keyPath)

	snap, err := env.mgr.Create("game1", "before", false)
	if err != nil {
		t.Fatal(err)
	}
	setRegistryValue(t, sub, "newSetting", 77)

	if _, err := env.mgr.Restore("game1", snap.ID); err != nil {
		t.Fatal(err)
	}
	if got, ok := readRegistryValue(t, sub, "newSetting"); !ok || got != 77 {
		t.Errorf("newSetting = %d (present %v) — a restore must not delete what it never captured", got, ok)
	}
}

// A corrupt capture must not read as "this game had no registry saves".
// Reporting nothing would look like a successful restore of a lost save.
func TestACorruptCaptureInAnArchiveIsNotSilent(t *testing.T) {
	env := setup(t)
	keyPath, _ := scratchRegistryKey(t)
	regDir := trackRegistryGame(t, env, keyPath)

	if err := os.WriteFile(filepath.Join(regDir, winreg.FileName), []byte("{ truncated"), 0o644); err != nil {
		t.Fatal(err)
	}
	if warnings := RestoreRegistryCapture("Game One", regDir); len(warnings) == 0 {
		t.Error("a corrupt capture restored without a word")
	}
}

// Two snapshots of the same game must capture the registry as it was at each
// moment, so rolling back to an older one really does go back.
func TestEachSnapshotHoldsTheRegistryAsItWasThen(t *testing.T) {
	env := setup(t)
	keyPath, sub := scratchRegistryKey(t)
	trackRegistryGame(t, env, keyPath)

	setRegistryValue(t, sub, "chapter", 1)
	first, err := env.mgr.Create("game1", "chapter one", false)
	if err != nil {
		t.Fatal(err)
	}
	setRegistryValue(t, sub, "chapter", 2)
	second, err := env.mgr.Create("game1", "chapter two", false)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := env.mgr.Restore("game1", first.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := readRegistryValue(t, sub, "chapter"); got != 1 {
		t.Errorf("chapter = %d after restoring the first snapshot, want 1", got)
	}
	if _, err := env.mgr.Restore("game1", second.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := readRegistryValue(t, sub, "chapter"); got != 2 {
		t.Errorf("chapter = %d after restoring the second snapshot, want 2", got)
	}
}

// Switching branches restores the incoming branch's files. The registry has to
// come with them: leaving it holding the branch you just left pairs one
// branch's files with another branch's registry state, which is a save the
// game never had.
func TestSwitchingBranchesBringsTheRegistryToo(t *testing.T) {
	env := setup(t)
	keyPath, sub := scratchRegistryKey(t)
	trackRegistryGame(t, env, keyPath)

	// main: chapter 1
	setRegistryValue(t, sub, "chapter", 1)
	writeSave(t, env.saveDir, "save.dat", "main branch")
	if _, err := env.mgr.Create("game1", "on main", false); err != nil {
		t.Fatal(err)
	}

	// A second branch, and actually switched onto it — CreateBranch makes the
	// branch without moving the game onto it, so snapshots would otherwise
	// keep landing on main and the test would prove nothing.
	if _, err := env.mgr.CreateBranch("game1", "experiment", true); err != nil {
		t.Fatal(err)
	}
	if err := env.mgr.SwitchBranch("game1", "experiment"); err != nil {
		t.Fatal(err)
	}
	setRegistryValue(t, sub, "chapter", 2)
	writeSave(t, env.saveDir, "save.dat", "experiment branch")
	if _, err := env.mgr.Create("game1", "on experiment", false); err != nil {
		t.Fatal(err)
	}

	// Back to main: both halves of the save must be main's.
	if err := env.mgr.SwitchBranch("game1", "main"); err != nil {
		t.Fatalf("SwitchBranch: %v", err)
	}
	if got, _ := readRegistryValue(t, sub, "chapter"); got != 1 {
		t.Errorf("chapter = %d after switching to main — the files came back but the "+
			"registry still holds the branch that was left", got)
	}
}
