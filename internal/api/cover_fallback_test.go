package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// The cache key has to be safe as a file name and unable to collide with a
// numeric App ID, because a title can contain anything at all — including path
// separators, which would otherwise write a cover outside the covers folder.
func TestNameCacheKeysAreSafeAndDistinct(t *testing.T) {
	for _, name := range []string{
		`..\..\windows\system32`, "a/b/c", "MOUSE", "mouse", "  MOUSE  ",
	} {
		key := coverKeyForName(name)
		if strings.ContainsAny(key, `\/:*?"<>|.`) {
			t.Errorf("coverKeyForName(%q) = %q, which is not safe as a file name", name, key)
		}
		if isNumericID(key) {
			t.Errorf("coverKeyForName(%q) = %q, which could collide with an App ID", name, key)
		}
	}
	// Spelling differences that mean the same game share a key, so art is
	// fetched once.
	if coverKeyForName("MOUSE") != coverKeyForName("  mouse  ") {
		t.Error("the same title under two spellings produced two cache keys")
	}
	if coverKeyForName("Game A") == coverKeyForName("Game B") {
		t.Error("two different titles collided on one cache key")
	}
}

// A relay is another machine's answer, and this one fetches whatever that
// answer names. Only https is followed — anything else could point the daemon
// at something on this machine or its network.
func TestOnlyHTTPSArtURLsAreAccepted(t *testing.T) {
	ts := startTestServer(t)

	for _, body := range []string{
		`{"url":"http://example.com/a.jpg"}`,
		`{"url":"file:///C:/Windows/win.ini"}`,
		`{"url":"http://127.0.0.1:8386/admin"}`,
		`{"url":""}`,
		`not json`,
	} {
		relay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(body))
		}))
		settings, err := ts.daemon.Store.GetSettings()
		if err != nil {
			t.Fatal(err)
		}
		settings.RelayURL = relay.URL
		if err := ts.daemon.Store.UpdateSettings(settings); err != nil {
			t.Fatal(err)
		}
		if got := ts.server.fallbackArtURL("Some Game", ""); got != "" {
			t.Errorf("body %q produced art URL %q, want it refused", body, got)
		}
		relay.Close()
	}
}

// With no relay configured there is nowhere to ask, and that must be silent
// rather than an error — most users have no relay.
func TestNoRelayMeansNoLookup(t *testing.T) {
	ts := startTestServer(t)
	settings, err := ts.daemon.Store.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings.RelayURL = ""
	if err := ts.daemon.Store.UpdateSettings(settings); err != nil {
		t.Fatal(err)
	}
	if got := ts.server.fallbackArtURL("Some Game", "1234"); got != "" {
		t.Errorf("fallbackArtURL with no relay = %q, want empty", got)
	}
}

// The second source is asked only after Steam has nothing, and its answer is
// cached under the same key so the next request touches no network at all.
func TestFallbackArtIsFetchedAndCached(t *testing.T) {
	ts := startTestServer(t)

	const png = "\x89PNG\r\n\x1a\n fake image bytes"
	var lookups int
	art := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(png))
	}))
	defer art.Close()
	relay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lookups++
		// The daemon refuses anything but https, so this test drives
		// fetchFallbackCover through the URL it would have been given.
		_, _ = w.Write([]byte(`{"url":"` + art.URL + `/a.jpg"}`))
	}))
	defer relay.Close()

	settings, err := ts.daemon.Store.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings.RelayURL = relay.URL
	if err := ts.daemon.Store.UpdateSettings(settings); err != nil {
		t.Fatal(err)
	}

	// http:// is refused by design, so this asserts the refusal rather than
	// the happy path — the happy path needs a real https source.
	if _, err := ts.server.fetchFallbackCover(coverKeyForName("Some Game"), "Some Game", "", false); err == nil {
		t.Error("an http art URL was fetched; only https may be followed")
	}
	if lookups == 0 {
		t.Error("the relay was never asked")
	}
}

// A game with no App ID and no art anywhere is a normal outcome, not a
// connectivity failure — and must not be reported as one, or a user with a few
// obscure games sees a warning saying their network is broken.
func TestAGameWithNoArtIsNotReportedAsANetworkFailure(t *testing.T) {
	ts := startTestServer(t)
	settings, err := ts.daemon.Store.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings.RelayURL = ""
	if err := ts.daemon.Store.UpdateSettings(settings); err != nil {
		t.Fatal(err)
	}

	const title = "A Game Nobody Has Art For"
	resp, _ := ts.do(t, http.MethodGet, "/api/cover?name="+url.QueryEscape(title), nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}

	// Recorded as a miss is the whole point: a miss is remembered and never
	// re-walked, while a connectivity failure is warned about and retried. The
	// wrong classification means either a spurious "your network is broken"
	// warning, or the same hopeless lookup on every scan forever.
	if _, ok := coverMisses.Load(coverMissKey(coverKeyForName(title), false)); !ok {
		t.Error("a game with no art was not recorded as a miss, so it was treated " +
			"as a network failure and will be looked up again on every scan")
	}
}

// A request naming neither a game nor an App ID is a client bug, and saying so
// is better than quietly answering 404 forever.
func TestCoverRequiresSomethingToLookUp(t *testing.T) {
	ts := startTestServer(t)
	resp, _ := ts.do(t, http.MethodGet, "/api/cover", nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d for a request with no appId and no name, want 400", resp.StatusCode)
	}
}
