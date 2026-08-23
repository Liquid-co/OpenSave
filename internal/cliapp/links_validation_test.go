package cliapp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opensave/opensave/internal/daemon"
	"github.com/opensave/opensave/internal/store"
)

// cmdLinks reads nothing but the store, so a daemon holding one is enough.
func linksTestDaemon(t *testing.T) *daemon.Daemon {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return &daemon.Daemon{Store: s}
}

// Listing aliases for an id nobody tracks returns an empty set, which read
// back as "No other game is linked to X" and exited 0 — a typo confirmed as an
// answer. Every sibling command (snapshots, locations, ignore, branch) refuses
// an unknown id, and this one has to agree with them.
func TestLinksRejectsAnUnknownGame(t *testing.T) {
	d := linksTestDaemon(t)

	r, w, _ := os.Pipe()
	orig := os.Stderr
	os.Stderr = w
	rc := cmdLinks(d, []string{"no-such-game-xyz"})
	w.Close()
	os.Stderr = orig
	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	msg := string(buf[:n])

	if rc == 0 {
		t.Error("cmdLinks returned 0 for an id that is not tracked — a mistyped id " +
			"reads back as a confirmed 'no links'")
	}
	// Naming the id is what makes the refusal useful: the user typed it from
	// memory and needs to see which part was wrong.
	if !strings.Contains(msg, "no-such-game-xyz") {
		t.Errorf("the refusal should quote the id that was typed; got: %s", msg)
	}
}
