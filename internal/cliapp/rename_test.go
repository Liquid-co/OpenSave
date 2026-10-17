package cliapp

import (
	"io"
	"os"
	"testing"

	"github.com/opensave/opensave/internal/store"
)

func quietly(t *testing.T, f func() int) int {
	t.Helper()
	outR, outW, _ := os.Pipe()
	errR, errW, _ := os.Pipe()
	origOut, origErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outW, errW
	rc := f()
	outW.Close()
	errW.Close()
	os.Stdout, os.Stderr = origOut, origErr
	_, _ = io.ReadAll(outR)
	_, _ = io.ReadAll(errR)
	return rc
}

// `opensave rename` takes the rest of the line as the name, with no quotes
// needed, keeps the game's ID, and refuses a name no screen can show.
func TestRename(t *testing.T) {
	d := linksTestDaemon(t)
	if err := d.Store.CreateGame(store.Game{ID: "saves", Name: "Saves", SavePath: t.TempDir(), ActiveBranch: "main"}); err != nil {
		t.Fatal(err)
	}

	if rc := quietly(t, func() int { return cmdRename(d, []string{"saves", "Hollow", "Knight"}) }); rc != 0 {
		t.Fatalf("rename exited %d", rc)
	}
	g, err := d.Store.GetGame("saves")
	if err != nil {
		t.Fatalf("the game is no longer under its ID: %v", err)
	}
	if g.Name != "Hollow Knight" {
		t.Errorf("name = %q, want %q", g.Name, "Hollow Knight")
	}

	if rc := quietly(t, func() int { return cmdRename(d, []string{"saves", "   "}) }); rc == 0 {
		t.Error("an empty name was accepted")
	}
	if rc := quietly(t, func() int { return cmdRename(d, []string{"saves"}) }); rc == 0 {
		t.Error("rename with no name exited 0")
	}
	if g, _ := d.Store.GetGame("saves"); g.Name != "Hollow Knight" {
		t.Errorf("a refused rename changed the name to %q", g.Name)
	}
}
