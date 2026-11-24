package daemon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/opensave/opensave/internal/store"
)

func TestNativeOwnershipReadFailureCannotAuthorizeTrack(t *testing.T) {
	d := newTestDaemon(t)
	root := filepath.Join(t.TempDir(), "Progress")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	// Once a native inventory read is unavailable, a requested path must
	// not be called "free" or passed to Store.CreateGame.
	if err := d.Store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ValidateSavePath(root); err == nil {
		t.Fatal("ValidateSavePath treated failed ListGames as an empty library")
	}
	if _, overlap, err := d.overlappingGame(root); err == nil || overlap {
		t.Fatalf("overlappingGame treated inventory failure as no overlap: %t, %v", overlap, err)
	}
	if _, err := d.TrackGame(store.Game{Name: "Should Refuse", SavePath: root}); err == nil {
		t.Fatal("TrackGame admitted path without a readable native inventory")
	}
}
