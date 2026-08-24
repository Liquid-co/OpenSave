package api

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func writeLocalArt(t *testing.T, cacheDir, appID, name, content string) {
	t.Helper()
	dir := filepath.Join(cacheDir, appID)
	if err := os.MkdirAll(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Art Steam has already downloaded is on this disk, and using it costs nothing:
// no network, no third party, no rate limit, and it works offline.
func TestLocalSteamArtIsUsedBeforeTheNetwork(t *testing.T) {
	ts := startTestServer(t)
	cache := t.TempDir()
	ts.server.SteamCacheDirs = []string{cache}
	writeLocalArt(t, cache, "1091500", "header.jpg", "local header bytes")

	resp, _ := ts.do(t, http.MethodGet, "/api/cover?appId=1091500", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 from the local cache", resp.StatusCode)
	}
}

// A portrait request prefers the tall image and falls back to the header — the
// caller would rather have the wrong shape than nothing, which is what the
// fetching path already does.
func TestPortraitPrefersTheTallImageThenFallsBack(t *testing.T) {
	ts := startTestServer(t)
	cache := t.TempDir()
	ts.server.SteamCacheDirs = []string{cache}

	writeLocalArt(t, cache, "1091500", "library_600x900.jpg", "tall")
	if got := string(ts.server.localSteamCover("1091500", true)); got != "tall" {
		t.Errorf("portrait = %q, want the tall image", got)
	}

	// Only a header available: still better than nothing.
	writeLocalArt(t, cache, "2222", "header.jpg", "wide")
	if got := string(ts.server.localSteamCover("2222", true)); got != "wide" {
		t.Errorf("portrait with no tall image = %q, want the header", got)
	}

	// A landscape request must never be answered with the tall image.
	writeLocalArt(t, cache, "3333", "library_600x900.jpg", "tall only")
	if got := ts.server.localSteamCover("3333", false); got != nil {
		t.Errorf("landscape = %q, want nothing rather than the wrong shape", got)
	}
}

// A game with no local art must fall through rather than be reported as having
// none — the network path is what covers everything Steam has not pre-fetched.
func TestNoLocalArtFallsThrough(t *testing.T) {
	ts := startTestServer(t)
	ts.server.SteamCacheDirs = []string{t.TempDir()}
	if got := ts.server.localSteamCover("999999", false); got != nil {
		t.Errorf("localSteamCover found %d bytes in an empty cache", len(got))
	}
}

// The App ID becomes a path segment, so anything but digits must be refused —
// a crafted value could otherwise read a file outside the cache.
func TestLocalArtRefusesANonNumericAppID(t *testing.T) {
	ts := startTestServer(t)
	cache := t.TempDir()
	ts.server.SteamCacheDirs = []string{cache}
	writeLocalArt(t, cache, "1091500", "header.jpg", "local")

	for _, bad := range []string{`..\..\windows\win.ini`, "../../etc/passwd", "abc", ""} {
		if got := ts.server.localSteamCover(bad, false); got != nil {
			t.Errorf("localSteamCover(%q) returned data; only digits may be used as a path", bad)
		}
	}
}
