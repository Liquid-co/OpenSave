package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A web page must not be able to drive this API.
//
// The API is loopback-only, and loopback is not a boundary a browser respects:
// a page on any site the user has open runs on this machine too. With
// Access-Control-Allow-Origin: * it could read the settings — node ID, room
// code, relay URL — untrack games, restore an old snapshot over a current
// save, and point the relay setting anywhere. The default port is 8383, so
// nothing had to be discovered first.
//
// These send exactly what a browser sends: an Origin header naming the page.

func doFrom(t *testing.T, ts *testServer, origin, method, path string, body any) *http.Response {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		rdr = strings.NewReader(string(raw))
	}
	req, err := http.NewRequest(method, ts.base+path, rdr)
	if err != nil {
		t.Fatal(err)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestAPI_AWebPageCannotReadSettings(t *testing.T) {
	ts := startTestServer(t)
	resp := doFrom(t, ts, "https://evil.example", http.MethodGet, "/api/settings", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("a page on another site read /api/settings: HTTP %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("CORS header handed to an unknown origin: %q", got)
	}
}

// The case CORS headers alone cannot stop: a POST with no body and no custom
// header is a "simple request" and executes whether or not the browser lets
// the page read the reply. It has to be refused, not just unanswered.
func TestAPI_AWebPageCannotFireASimplePost(t *testing.T) {
	ts := startTestServer(t)
	if err := os.WriteFile(filepath.Join(ts.saveDir, "s.sav"), []byte("x"), 0o666); err != nil {
		t.Fatal(err)
	}
	_, raw := ts.do(t, http.MethodPost, "/api/games", map[string]string{"name": "Target", "savePath": ts.saveDir})
	var gameID string
	_ = json.Unmarshal(raw["id"], &gameID)

	// Untrack, as a browser page would attempt it.
	resp := doFrom(t, ts, "https://evil.example", http.MethodDelete, "/api/games/"+gameID, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("cross-origin DELETE answered %d, want 403", resp.StatusCode)
	}
	if _, err := ts.daemon.Store.GetGame(gameID); err != nil {
		t.Fatal("a page on another site untracked a game")
	}

	// The bare-POST shape, which needs no preflight.
	resp = doFrom(t, ts, "https://evil.example", http.MethodPost, "/api/games/"+gameID+"/sync", nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("a cross-origin simple POST was executed: HTTP %d", resp.StatusCode)
	}
}

// The app's own window must keep working, on every platform it runs on.
func TestAPI_TheAppsOwnOriginIsAllowed(t *testing.T) {
	ts := startTestServer(t)
	for _, origin := range []string{"http://wails.localhost", "wails://wails"} {
		resp := doFrom(t, ts, origin, http.MethodGet, "/api/settings", nil)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("the app's own origin %s was refused: HTTP %d", origin, resp.StatusCode)
		}
		if got := resp.Header.Get("Access-Control-Allow-Origin"); got != origin {
			t.Errorf("origin %s: Access-Control-Allow-Origin = %q, want it echoed back exactly", origin, got)
		}
		if got := resp.Header.Get("Access-Control-Allow-Origin"); got == "*" {
			t.Error("the wildcard is back; every page on the web could call this API again")
		}
	}
	// And the preflight a browser sends before a JSON POST.
	req, _ := http.NewRequest(http.MethodOptions, ts.base+"/api/settings", nil)
	req.Header.Set("Origin", "http://wails.localhost")
	req.Header.Set("Access-Control-Request-Method", "POST")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("preflight from the app's origin answered %d, want 204", resp.StatusCode)
	}
}

// The CLI and the Steam Deck plugin are not browsers and send no Origin.
// They must pass exactly as before.
func TestAPI_NoOriginIsNotABrowserAndPasses(t *testing.T) {
	ts := startTestServer(t)
	resp := doFrom(t, ts, "", http.MethodGet, "/api/status", nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("a request with no Origin (CLI, Deck plugin) was refused: HTTP %d", resp.StatusCode)
	}
}

// A cover is sent with a week-long public cache lifetime, and the browser keeps
// one copy per URL. The app shows a cover two ways: an <img>, which sends no
// Origin and so gets no CORS headers back, and a fetch (the sidebar, the scan
// tiles — they need the explicit-cover header), which does. Without "Vary:
// Origin" on the first, the browser hands that header-less copy to the next
// fetch of the same URL, the fetch fails its CORS check, and so does every
// retry, for a week. The sidebar showed initials instead of art this way while
// the daemon served every cover perfectly well.
func TestAPI_ACachedResponseIsNotSharedAcrossOrigins(t *testing.T) {
	h := corsLocalhost(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeCover(w, []byte("jpeg"))
	}))
	for _, origin := range []string{"", "http://wails.localhost"} {
		req := httptest.NewRequest(http.MethodGet, "/api/cover?appId=1", nil)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("origin %q: HTTP %d", origin, rec.Code)
		}
		if !strings.Contains(rec.Header().Get("Cache-Control"), "max-age") {
			t.Fatalf("origin %q: the cover is no longer cacheable, so this test proves nothing", origin)
		}
		if !varyIncludes(rec.Header(), "Origin") {
			t.Errorf("origin %q: a cacheable response without Vary: Origin (got %q) — the browser will reuse it for a request from another origin",
				origin, rec.Header().Values("Vary"))
		}
	}
}

func varyIncludes(h http.Header, name string) bool {
	for _, v := range h.Values("Vary") {
		for _, part := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(part), name) {
				return true
			}
		}
	}
	return false
}
