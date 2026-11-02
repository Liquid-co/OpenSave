package api

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// A transport-level success with an empty map is indistinguishable from a
// legitimately empty library and must not authorize external Protect callers.
func TestGamesInventoryReadErrorIsNotEmptySuccess(t *testing.T) {
	ts := startTestServer(t)
	resp, body := ts.do(t, http.MethodGet, "/api/games", nil)
	if resp.StatusCode != http.StatusOK || body == nil {
		t.Fatalf("empty but healthy inventory: HTTP %d, %v", resp.StatusCode, body)
	}
	if err := ts.daemon.Store.Close(); err != nil {
		t.Fatal(err)
	}
	resp, body = ts.do(t, http.MethodGet, "/api/games", nil)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("failed inventory must report 503, got HTTP %d, %v", resp.StatusCode, body)
	}
	if strings.Contains(strings.ToLower(fmt.Sprint(body)), "sql:") || strings.Contains(strings.ToLower(fmt.Sprint(body)), "database is closed") {
		t.Fatalf("failed inventory leaked database details: %v", body)
	}
}
