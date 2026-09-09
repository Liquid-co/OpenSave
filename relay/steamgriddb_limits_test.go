package relay

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// Bounding the artwork cache, and backing off when the shared key is
// rate-limited.
//
// Both matter for the same reason: one SteamGridDB key serves every user of a
// relay. That makes the cache grow with every distinct game name anyone ever
// scans, and it makes a rate limit shared by everybody — where retrying per
// request turns a brief limit into a long one.

func resetSteamGrid() {
	steamGridCache.Lock()
	steamGridCache.entries = map[string]steamGridEntry{}
	steamGridCache.Unlock()
	steamGridLimit.Lock()
	steamGridLimit.until = time.Time{}
	steamGridLimit.Unlock()
}

func TestSteamGridCache_StaysWithinItsCap(t *testing.T) {
	resetSteamGrid()
	for i := 0; i < steamGridMaxEntries+500; i++ {
		steamGridStore(fmt.Sprintf("game-%d", i), "https://example.invalid/a.png", false)
	}
	if n := steamGridCacheSize(); n > steamGridMaxEntries {
		t.Errorf("cache holds %d entries, cap is %d", n, steamGridMaxEntries)
	}
	if steamGridCacheSize() == 0 {
		t.Error("eviction emptied the cache; it should trim, not clear")
	}
}

// Expired entries are dead weight a read would reject anyway, so they must be
// dropped before anything still live.
func TestSteamGridCache_EvictsExpiredBeforeLiveEntries(t *testing.T) {
	resetSteamGrid()

	steamGridCache.Lock()
	for i := 0; i < steamGridMaxEntries-1; i++ {
		steamGridCache.entries[fmt.Sprintf("stale-%d", i)] = steamGridEntry{
			url: "https://example.invalid/a.png",
			at:  time.Now().Add(-steamGridHitTTL - time.Hour),
		}
	}
	steamGridCache.entries["fresh"] = steamGridEntry{
		url: "https://example.invalid/fresh.png",
		at:  time.Now(),
	}
	steamGridCache.Unlock()

	steamGridStore("newcomer", "https://example.invalid/new.png", false)

	if _, ok, fresh := steamGridLookupCached("fresh"); !ok || !fresh {
		t.Error("a live entry was evicted while stale ones remained")
	}
	if n := steamGridCacheSize(); n > steamGridMaxEntries {
		t.Errorf("still over cap: %d", n)
	}
}

// fakeSteamGrid stands in for SteamGridDB and counts how many requests reach
// it, which is the thing a backoff is supposed to reduce.
func fakeSteamGrid(t *testing.T, status int, retryAfter string) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if retryAfter != "" {
			w.Header().Set("Retry-After", retryAfter)
		}
		w.WriteHeader(status)
	}))
	old := steamGridAPI
	steamGridAPI = srv.URL
	t.Cleanup(func() { steamGridAPI = old; srv.Close() })
	return srv, &hits
}

// The point of the whole exercise: after a 429, stop asking.
func TestSteamGrid_RateLimitStopsFurtherRequests(t *testing.T) {
	resetSteamGrid()
	_, hits := fakeSteamGrid(t, http.StatusTooManyRequests, "")
	s := &Server{cfg: Config{SteamGridDBKey: "test-key"}}

	var out any
	if err := s.steamGridGet(steamGridAPI+"/search/autocomplete/x", &out); err == nil {
		t.Fatal("a 429 should be reported as an error")
	}
	first := hits.Load()
	if first != 1 {
		t.Fatalf("expected exactly one upstream request, got %d", first)
	}

	for i := 0; i < 20; i++ {
		if err := s.steamGridGet(steamGridAPI+"/search/autocomplete/x", &out); err != errSteamGridBackoff {
			t.Fatalf("call %d returned %v, want the backoff error", i, err)
		}
	}
	if got := hits.Load(); got != first {
		t.Errorf("%d more requests reached the upstream during the backoff; the point is to stop asking",
			got-first)
	}
}

// A 503 is the other "we are struggling" reply and must back off too.
func TestSteamGrid_ServiceUnavailableAlsoBacksOff(t *testing.T) {
	resetSteamGrid()
	_, hits := fakeSteamGrid(t, http.StatusServiceUnavailable, "")
	s := &Server{cfg: Config{SteamGridDBKey: "test-key"}}

	var out any
	_ = s.steamGridGet(steamGridAPI+"/x", &out)
	if err := s.steamGridGet(steamGridAPI+"/x", &out); err != errSteamGridBackoff {
		t.Fatalf("second call returned %v, want the backoff error", err)
	}
	if hits.Load() != 1 {
		t.Errorf("upstream saw %d requests, want 1", hits.Load())
	}
}

func TestSteamGrid_RetryAfterIsHonouredAndClamped(t *testing.T) {
	resetSteamGrid()
	fakeSteamGrid(t, http.StatusTooManyRequests, "99999")
	s := &Server{cfg: Config{SteamGridDBKey: "test-key"}}

	var out any
	_ = s.steamGridGet(steamGridAPI+"/x", &out)

	paused, until := steamGridPaused()
	if !paused {
		t.Fatal("a 429 with Retry-After did not pause")
	}
	if d := time.Until(until); d > steamGridMaxBackoff+time.Minute {
		t.Errorf("Retry-After of 99999s produced a %s pause; it must be clamped to %s",
			d, steamGridMaxBackoff)
	}
}

func TestRetryAfterDelay_ReadsBothForms(t *testing.T) {
	if got := retryAfterDelay("30"); got != 30*time.Second {
		t.Errorf("seconds form: got %s", got)
	}
	if got := retryAfterDelay(time.Now().Add(2 * time.Minute).UTC().Format(http.TimeFormat)); got <= 0 {
		t.Errorf("http-date form: got %s, want a positive delay", got)
	}
	for _, bad := range []string{"", "   ", "not-a-number", "-5", "0"} {
		if got := retryAfterDelay(bad); got != 0 {
			t.Errorf("retryAfterDelay(%q) = %s, want 0 so the default applies", bad, got)
		}
	}
}

// A pause must expire on its own.
func TestSteamGrid_BackoffExpires(t *testing.T) {
	resetSteamGrid()
	steamGridPause(10 * time.Millisecond)
	if paused, _ := steamGridPaused(); !paused {
		t.Fatal("pause did not take effect")
	}
	time.Sleep(30 * time.Millisecond)
	if paused, _ := steamGridPaused(); paused {
		t.Error("the pause did not expire")
	}
}

// A shorter pause must not shorten a longer one already in force.
func TestSteamGrid_PauseNeverShortens(t *testing.T) {
	resetSteamGrid()
	steamGridPause(10 * time.Minute)
	_, longUntil := steamGridPaused()
	steamGridPause(time.Second)
	_, after := steamGridPaused()
	if after.Before(longUntil) {
		t.Error("a short pause cut a longer one short")
	}
}

// The lookup handler must not record a backoff as "this game has no art" —
// that would blank a cover for the whole miss TTL because of a brief limit.
func TestSteamGrid_BackoffIsNotCachedAsAMiss(t *testing.T) {
	resetSteamGrid()
	fakeSteamGrid(t, http.StatusTooManyRequests, "")
	s := &Server{cfg: Config{SteamGridDBKey: "test-key"}}

	req := httptest.NewRequest(http.MethodGet, "/api/cover/lookup?name=Some+Game", nil)
	rec := httptest.NewRecorder()
	s.handleCoverLookup(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatalf("expected a non-200 while rate limited, got %d", rec.Code)
	}
	if n := steamGridCacheSize(); n != 0 {
		t.Errorf("a rate-limited lookup stored %d cache entries; it must remember nothing", n)
	}
}
