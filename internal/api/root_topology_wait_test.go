package api

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// One actual request waits for the in-flight writer and then succeeds.
// This does not rely on an E2E helper repeatedly retrying a refused request.
func TestRootTopologySingleRequestWaitsForWriterAndPreservesFiles(t *testing.T) {
	ts := startTestServer(t)
	ts.server.topologyWaitFor = time.Second
	id := trackProbeGame(t, ts)
	dir := filepath.Join(t.TempDir(), "config")
	if err := os.Mkdir(dir, 0700); err != nil { t.Fatal(err) }
	path := filepath.Join(dir, "slot.sav")
	if err := os.WriteFile(path, []byte("ORIGINAL"), 0600); err != nil { t.Fatal(err) }
	resp, body := ts.do(t, http.MethodPost, "/api/games/"+id+"/roots", map[string]string{"name":"config","path":dir})
	if resp.StatusCode != http.StatusOK { t.Fatalf("setup failed HTTP %d: %v",resp.StatusCode,body) }
	writer := ts.daemon.P2P.Sync.Writing(id)
	result := make(chan int,1)
	go func() {
		response, _ := ts.do(t, http.MethodDelete, "/api/games/"+id+"/roots/config", nil)
		result <- response.StatusCode
	}()
	select {
	case status := <-result: t.Fatalf("mutated while writer active: HTTP %d",status)
	case <-time.After(60*time.Millisecond):
	}
	roots, err := ts.daemon.Store.GameRootPaths(id)
	if err != nil || roots["config"] != dir { t.Fatalf("mapping moved during writer: %v %v",roots,err) }
	writer()
	select {
	case status := <-result:
		if status != http.StatusOK { t.Fatalf("after drain HTTP %d",status) }
	case <-time.After(3*time.Second): t.Fatal("single API request never resumed")
	}
	bytes, err := os.ReadFile(path)
	if err != nil || string(bytes)!="ORIGINAL" { t.Fatalf("physical save modified: %s %v",bytes,err) }
}
