package api

import (
	"net/http"
	"strings"
	"testing"
)

// A blank device name is refused and nothing is stored; one with spaces
// around it is kept without them; other settings still save on their own.
func TestSettingsRefusesABlankDeviceName(t *testing.T) {
	ts := startTestServer(t)
	_, before := ts.do(t, http.MethodGet, "/api/settings", nil)

	for _, blank := range []string{"", "   "} {
		resp, body := ts.do(t, http.MethodPost, "/api/settings", map[string]any{"deviceName": blank})
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d for %q, want 400", resp.StatusCode, blank)
		}
		if !strings.Contains(string(body["error"]), "name") {
			t.Errorf("the refusal should say what is wrong, got %s", body["error"])
		}
	}
	_, after := ts.do(t, http.MethodGet, "/api/settings", nil)
	if string(after["deviceName"]) != string(before["deviceName"]) {
		t.Errorf("deviceName changed to %s by a refused request", after["deviceName"])
	}

	if resp, _ := ts.do(t, http.MethodPost, "/api/settings", map[string]any{"deviceName": "  Living room PC "}); resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d for a real name, want 200", resp.StatusCode)
	}
	_, after = ts.do(t, http.MethodGet, "/api/settings", nil)
	if string(after["deviceName"]) != `"Living room PC"` {
		t.Errorf("deviceName = %s, want it trimmed", after["deviceName"])
	}

	// A request that does not mention the name is not held up by it.
	if resp, _ := ts.do(t, http.MethodPost, "/api/settings", map[string]any{"autoSync": true}); resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d for an unrelated setting, want 200", resp.StatusCode)
	}
}
