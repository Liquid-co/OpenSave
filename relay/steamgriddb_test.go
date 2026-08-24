package relay

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func coverLookupRequest(t *testing.T, s *Server, query string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/cover/lookup?"+query, nil)
	s.handleCoverLookup(rec, req)
	return rec
}

// A relay without a key is not the same as a game without art, and the client
// treats the two differently — one is worth telling an operator about, the
// other is ordinary.
func TestCoverLookupWithoutAKeySaysSo(t *testing.T) {
	s := &Server{}
	rec := coverLookupRequest(t, s, "name=Anything")
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d with no key configured, want 503", rec.Code)
	}
}

func TestCoverLookupNeedsSomethingToLookUp(t *testing.T) {
	s := &Server{cfg: Config{SteamGridDBKey: "test-key"}}
	if rec := coverLookupRequest(t, s, ""); rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d with neither name nor appId, want 400", rec.Code)
	}
}

// A search returns its nearest guesses for a title SteamGridDB has never heard
// of. Accepting one would hand a user confident, wrong art — worse than a
// blank tile, because nothing about it looks like a mistake.
func TestOnlyAnExactTitleMatchIsAccepted(t *testing.T) {
	for _, tc := range []struct {
		asked, returned string
		wantMatch       bool
	}{
		{"Vigil", "Vigil: The Longest Night", false},
		{"MOUSE", "Mouse Simulator", false},
		{"Rounds", "Rounds", true},
		{"Baldur's Gate 3", "Baldurs Gate 3", true},        // punctuation only
		{"Trackmania", "Trackmania United Forever", false}, // the real near-miss
	} {
		got := normalizeTitle(tc.asked) == normalizeTitle(tc.returned)
		if got != tc.wantMatch {
			t.Errorf("asked %q, search returned %q: match=%v, want %v",
				tc.asked, tc.returned, got, tc.wantMatch)
		}
	}
}

// A miss is the answer that costs most to recompute — two round trips that
// found nothing — and every client asks about the same popular games. Caching
// misses is what stops one artless game being looked up once per user.
func TestMissesAreCachedAsWellAsHits(t *testing.T) {
	steamGridCache.Lock()
	steamGridCache.entries = nil
	steamGridCache.Unlock()

	steamGridStore("a game|", "")
	if _, ok, fresh := steamGridLookupCached("a game|"); !fresh || ok {
		t.Errorf("a miss was not cached: fresh=%v ok=%v", fresh, ok)
	}

	steamGridStore("other|", "https://example.com/art.png")
	url, ok, fresh := steamGridLookupCached("other|")
	if !fresh || !ok || url != "https://example.com/art.png" {
		t.Errorf("a hit did not survive the cache: %q ok=%v fresh=%v", url, ok, fresh)
	}

	if _, _, fresh := steamGridLookupCached("never asked|"); fresh {
		t.Error("an entry nobody stored was reported as cached")
	}
}

// Reaching SteamGridDB and being told nothing exists is different from failing
// to reach it. Caching the second as a miss would leave every client blank for
// hours over one bad minute.
func TestAnUnreachableSourceIsNotCachedAsAMiss(t *testing.T) {
	steamGridCache.Lock()
	steamGridCache.entries = nil
	steamGridCache.Unlock()

	// A key that points nowhere: the lookup fails to reach anything.
	s := &Server{cfg: Config{SteamGridDBKey: "test-key"}}
	rec := coverLookupRequest(t, s, "name=Some%20Game")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if _, _, fresh := steamGridLookupCached("some game|"); fresh {
		t.Error("a failure to reach SteamGridDB was cached as 'this game has no art'")
	}
}
