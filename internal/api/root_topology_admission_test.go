package api

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// Root API mutations must observe native admission for the duration of the
// Store write, not merely sample an active-writer count.
func TestRootTopologyRefusesActiveNativeWriter(t *testing.T) {
	ts := startTestServer(t)
	id := trackProbeGame(t, ts)
	dir := filepath.Join(t.TempDir(), "config")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	resp, body := ts.do(t, http.MethodPost, "/api/games/"+id+"/roots",
		map[string]string{"name": "config", "path": dir})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("cannot establish mapped root: HTTP %d %v", resp.StatusCode, body)
	}

	releaseWriter := ts.daemon.P2P.Sync.Writing(id)
	resp, _ = ts.do(t, http.MethodDelete, "/api/games/"+id+"/roots/config", nil)
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("RemoveRoot during native write: got HTTP %d, want 409", resp.StatusCode)
	}
	other := filepath.Join(t.TempDir(), "other")
	if err := os.MkdirAll(other, 0700); err != nil {
		t.Fatal(err)
	}
	resp, _ = ts.do(t, http.MethodPost, "/api/games/"+id+"/roots",
		map[string]string{"name": "config", "path": other})
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("AddRoot remap during native write: got HTTP %d, want 409", resp.StatusCode)
	}
	roots, err := ts.daemon.Store.GameRootPaths(id)
	if err != nil || roots["config"] != dir {
		t.Fatalf("mapping changed during writer: roots=%v err=%v", roots, err)
	}
	releaseWriter()

	resp, body = ts.do(t, http.MethodDelete, "/api/games/"+id+"/roots/config", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("safe removal after writer drained: HTTP %d %v", resp.StatusCode, body)
	}
	roots, err = ts.daemon.Store.GameRootPaths(id)
	if err != nil || roots["config"] != "" {
		t.Fatalf("root still mapped after successful removal: %v err=%v", roots, err)
	}
}
