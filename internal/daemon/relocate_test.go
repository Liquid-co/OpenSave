package daemon

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/opensave/opensave/internal/store"
)

// freeDriveLetter returns a drive letter not mounted here, or "".
func freeDriveLetter() string {
	for c := 'Z'; c >= 'D'; c-- {
		if _, err := os.Stat(string(c) + `:\`); err != nil {
			return string(c)
		}
	}
	return ""
}

// A game tracked elsewhere with its save on a drive this device lacks, while
// the identical path exists on another drive here: the folder is offered, not
// switched to — it could be an old copy — and the game is switched only when
// the user accepts that folder.
func TestRelocateToOtherDrive_IsOfferedThenAccepted(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("drive letters are a Windows thing")
	}
	free := freeDriveLetter()
	if free == "" {
		t.Skip("no free drive letter")
	}
	d := newTestDaemon(t)
	real := filepath.Join(t.TempDir(), "SteamLibrary", "steamapps", "common", "Game", "save")
	if err := os.MkdirAll(real, 0o777); err != nil {
		t.Fatal(err)
	}
	elsewhere := free + ":" + real[2:] // same path, absent drive
	if err := d.Store.CreateGame(store.Game{ID: "g", Name: "Game", SavePath: elsewhere, ActiveBranch: "main", AutoSync: true}); err != nil {
		t.Fatal(err)
	}

	d.noteMissing("g", "Game", elsewhere, true)

	g, _ := d.Store.GetGame("g")
	if g.SavePath != elsewhere {
		t.Fatalf("save path switched to %s without asking", g.SavePath)
	}
	if got := d.RelocateSuggestion("g"); !strings.EqualFold(got, real) {
		t.Fatalf("offered %q, want %s", got, real)
	}
	d.missingMu.Lock()
	missing := d.missing["g"]
	d.missingMu.Unlock()
	if !missing {
		t.Error("the game is not marked missing while the offer is open")
	}

	if err := d.AcceptRelocation("g", `C:\somewhere\else`); err == nil {
		t.Error("accepted a folder that was not the one offered")
	}
	if err := d.AcceptRelocation("g", real); err != nil {
		t.Fatal(err)
	}
	g, _ = d.Store.GetGame("g")
	if !strings.EqualFold(g.SavePath, real) {
		t.Fatalf("save path = %s after accepting, want %s", g.SavePath, real)
	}
	if d.RelocateSuggestion("g") != "" {
		t.Error("still offered after being accepted")
	}
}

// "Not this one": the folder is not offered again.
func TestRelocateToOtherDrive_Dismissed(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("drive letters are a Windows thing")
	}
	free := freeDriveLetter()
	if free == "" {
		t.Skip("no free drive letter")
	}
	d := newTestDaemon(t)
	real := filepath.Join(t.TempDir(), "Backup", "Game", "save")
	if err := os.MkdirAll(real, 0o777); err != nil {
		t.Fatal(err)
	}
	elsewhere := free + ":" + real[2:]
	if err := d.Store.CreateGame(store.Game{ID: "b", Name: "Backup Game", SavePath: elsewhere, ActiveBranch: "main", AutoSync: true}); err != nil {
		t.Fatal(err)
	}
	d.noteMissing("b", "Backup Game", elsewhere, true)
	if d.RelocateSuggestion("b") == "" {
		t.Fatal("setup: nothing offered")
	}
	if err := d.DismissRelocation("b"); err != nil {
		t.Fatal(err)
	}
	d.missingMu.Lock()
	delete(d.relocateChecked, "b") // as if relocateRecheck had passed
	d.missingMu.Unlock()
	d.noteMissing("b", "Backup Game", elsewhere, true)
	if got := d.RelocateSuggestion("b"); got != "" {
		t.Errorf("offered %s again after it was dismissed", got)
	}
	g, _ := d.Store.GetGame("b")
	if g.SavePath != elsewhere {
		t.Errorf("save path changed to %s", g.SavePath)
	}
}

// Not found: said once, and not searched again on every reconcile.
func TestRelocateNotFound_IsNotRepeated(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("drive letters are a Windows thing")
	}
	free := freeDriveLetter()
	if free == "" {
		t.Skip("no free drive letter")
	}
	d := newTestDaemon(t)
	path := free + `:\NoSuchLibrary\steamapps\common\Nothing\save`
	if err := d.Store.CreateGame(store.Game{ID: "n", Name: "Nothing", SavePath: path, ActiveBranch: "main", AutoSync: true}); err != nil {
		t.Fatal(err)
	}
	if d.suggestOtherDrive("n", "Nothing", path) {
		t.Fatal("relocated to a folder that does not exist")
	}
	first := d.relocateChecked["n"]
	d.suggestOtherDrive("n", "Nothing", path)
	if !d.relocateChecked["n"].Equal(first) {
		t.Error("searched again straight away; it should wait relocateRecheck")
	}
}

// A folder set by hand while an offer is open is the answer that counts:
// accepting the old offer is refused, the hand-set folder stays, and the
// offer is gone.
func TestRelocateToOtherDrive_AnOfferIsStaleOnceTheFolderIsSetByHand(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("drive letters are a Windows thing")
	}
	free := freeDriveLetter()
	if free == "" {
		t.Skip("no free drive letter")
	}
	d := newTestDaemon(t)
	real := filepath.Join(t.TempDir(), "SteamLibrary", "steamapps", "common", "Game", "save")
	if err := os.MkdirAll(real, 0o777); err != nil {
		t.Fatal(err)
	}
	elsewhere := free + ":" + real[2:]
	if err := d.Store.CreateGame(store.Game{ID: "g", Name: "Game", SavePath: elsewhere, ActiveBranch: "main", AutoSync: true}); err != nil {
		t.Fatal(err)
	}
	d.noteMissing("g", "Game", elsewhere, true)
	if got := d.RelocateSuggestion("g"); !strings.EqualFold(got, real) {
		t.Fatalf("setup: offered %q, want %s", got, real)
	}

	byHand := t.TempDir()
	g, _ := d.Store.GetGame("g")
	g.SavePath = byHand
	if err := d.Store.UpdateGame(g); err != nil {
		t.Fatal(err)
	}

	if err := d.AcceptRelocation("g", real); err == nil {
		t.Error("an offer made for the old folder was accepted after the folder was set by hand")
	}
	g, _ = d.Store.GetGame("g")
	if g.SavePath != byHand {
		t.Errorf("save path = %s, want the one set by hand, %s", g.SavePath, byHand)
	}
	if d.RelocateSuggestion("g") != "" {
		t.Error("the stale offer is still offered")
	}
}

// A drive that is there but not ready, an empty card reader, does not hold the
// folder. It read as holding it: a second match, so nothing was ever offered.
func TestRelocateToOtherDrive_ADriveNotReadyIsNoMatch(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("drive letters are a Windows thing")
	}
	free := freeDriveLetter()
	if free == "" {
		t.Skip("no free drive letter")
	}
	real := filepath.Join(t.TempDir(), "SteamLibrary", "save")
	if err := os.MkdirAll(real, 0o777); err != nil {
		t.Fatal(err)
	}
	statSaveFolder = func(p string) (os.FileInfo, error) {
		if strings.HasPrefix(strings.ToUpper(p), free+":") {
			return nil, errors.New("The device is not ready.")
		}
		return os.Stat(p)
	}
	t.Cleanup(func() { statSaveFolder = os.Stat })

	// Looked for from a third letter, as a peer's path arrives.
	gone := "?"
	for c := 'Y'; c >= 'D'; c-- {
		l := string(c)
		if l == free || strings.EqualFold(l, real[:1]) {
			continue
		}
		if _, err := os.Stat(l + ":" + string(filepath.Separator)); err != nil {
			gone = l
			break
		}
	}
	if gone == "?" {
		t.Skip("no second free drive letter")
	}
	found := relocateCandidates(gone + ":" + real[2:])
	var mine []string
	for _, f := range found {
		if strings.EqualFold(f, real) || strings.EqualFold(f[:1], free) {
			mine = append(mine, f)
		}
	}
	if len(mine) != 1 || !strings.EqualFold(mine[0], real) {
		t.Errorf("found %v, want only %s", mine, real)
	}
}
