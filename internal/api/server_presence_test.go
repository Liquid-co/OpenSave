package api

import (
	"path/filepath"
	"testing"

	"github.com/opensave/opensave/internal/fsx"
)

// The address file is advisory, but the running HTTP daemon must hold a
// stable cross-process owner lock even when its address file is absent.
func TestServerPresenceLockLivesUntilShutdown(t *testing.T) {
	ts := startTestServer(t)
	path := filepath.Join(ts.daemon.Paths.HomeDir, "daemon-presence.lock")
	unlock, ok, err := fsx.TryLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		unlock()
		t.Fatal("running native HTTP server did not hold the presence lock")
	}
	ts.server.Stop()
	unlock, ok, err = fsx.TryLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("daemon-presence lock remained held after server shutdown")
	}
	unlock()
}
