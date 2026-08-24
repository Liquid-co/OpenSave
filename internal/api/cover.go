package api

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// httpStatusError distinguishes a "the CDN answered, but not 200" failure
// (e.g. a 404 because the game has no cover art) from a connectivity failure
// (the network reset/blocked the connection). Only the latter is worth a
// systemic warning.
type httpStatusError struct{ status string }

func (e *httpStatusError) Error() string { return "cover fetch: " + e.status }

// errNoSteamID marks "this game has no App ID to ask Steam about", which is a
// different thing from a fetch that failed — one is a fact about the game, the
// other is a fact about the network, and only the second is worth telling
// anyone about.
var errNoSteamID = errors.New("no steam app id for this game")

// A cover is a small image on a best-effort path: fail fast rather than tie
// up a connection, since a scan can request hundreds at once.
var coverClient = &http.Client{Timeout: 6 * time.Second}

// coverFetchSem bounds how many cover fetches hit the network at once. A big
// scan renders hundreds of tiles simultaneously; without this the daemon
// opens a socket per tile and starves the rest of the local API (including
// the scan request itself, which shares the browser's per-origin connection
// budget).
var coverFetchSem = make(chan struct{}, 6)

// coverFailOnce logs the first cover-fetch *connectivity* failure (missing
// covers 404 without warning) so a systemic problem — the daemon having no
// route to Steam's CDN or the proxy (e.g. a campus/ISP firewall) — is
// diagnosable from the Activity Log without flooding it.
var coverFailOnce sync.Once

// steamCDNHosts are the CDN URL templates tried in order (host + path style
// have both shifted over Steam's lifetime); the first that returns 200 wins,
// so covers keep working regardless of which is currently live.
var steamCDNHosts = []string{
	"https://cdn.cloudflare.steamstatic.com/steam/apps/%s/%s",
	"https://shared.cloudflare.steamstatic.com/store_item_assets/steam/apps/%s/%s",
	"https://cdn.akamai.steamstatic.com/steam/apps/%s/%s",
	"https://shared.akamai.steamstatic.com/store_item_assets/steam/apps/%s/%s",
}

// coverSource remembers which fetch strategy last worked, so we don't pay the
// full "try every dead host, then the proxy" walk for every single image.
// On a network that blocks Steam that walk costs ~5 timeouts per cover, which
// is what made large scans crawl.
var coverSource struct {
	sync.Mutex
	host    int  // index into steamCDNHosts that last succeeded
	known   bool // host/useProxy below are meaningful
	viaProx bool // direct hosts are unreachable; go straight to the proxy
}

// coverMisses remembers App IDs with no art (unreleased/non-Steam titles), so
// a rescan doesn't re-attempt every one of them over the network.
var coverMisses sync.Map // appID+portrait -> time.Time

const coverMissTTL = 6 * time.Hour

func coverMissKey(appID string, portrait bool) string {
	if portrait {
		return appID + "_p"
	}
	return appID
}

func recentCoverMiss(key string) bool {
	if v, ok := coverMisses.Load(key); ok {
		if at, ok := v.(time.Time); ok && time.Since(at) < coverMissTTL {
			return true
		}
		coverMisses.Delete(key)
	}
	return false
}

// handleCover proxies (and disk-caches) Steam CDN cover art through the local
// daemon. The embedded webview can't reliably hotlink an external CDN, but it
// can always reach the local API. So covers load from localhost and keep
// working offline once cached.
//
// GET /api/cover?appId=<numeric>[&portrait=1]
func (s *Server) handleCover(w http.ResponseWriter, r *http.Request) {
	appID := r.URL.Query().Get("appId")
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	// A game with no App ID can still have art, under its name. Steam cannot
	// answer about it — its CDN is keyed on App ID — but the second source can.
	if !isNumericID(appID) {
		appID = ""
	}
	if appID == "" && name == "" {
		writeError(w, http.StatusBadRequest, "appId must be numeric, or a name must be given")
		return
	}
	portrait := r.URL.Query().Get("portrait") == "1"

	// One cache key per game, whichever source ends up answering.
	cacheKey := appID
	if cacheKey == "" {
		cacheKey = coverKeyForName(name)
	}

	// Serve straight from the disk cache without touching the network.
	if data, err := os.ReadFile(s.coverCachePath(cacheKey, portrait)); err == nil && len(data) > 0 {
		s.writeCoverMaybeExplicit(w, cacheKey, data)
		return
	}

	// Known to have no art — don't re-walk the network for it on every scan.
	missKey := coverMissKey(cacheKey, portrait)
	if recentCoverMiss(missKey) {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	coverFetchSem <- struct{}{}
	defer func() { <-coverFetchSem }()

	// Another request may have fetched it while we waited for a slot.
	if data, err := os.ReadFile(s.coverCachePath(cacheKey, portrait)); err == nil && len(data) > 0 {
		s.writeCoverMaybeExplicit(w, cacheKey, data)
		return
	}

	// Steam first, always: exact, no third party, and the answer for most of a
	// library. Only when it has nothing does the second source get asked.
	var data []byte
	var err error
	// A game with no App ID never asks Steam at all. That is not a failure to
	// reach anything, so it must not be reported as one: errNoSteamID is
	// carried separately from a fetch error for exactly that reason.
	err = errNoSteamID
	if appID != "" {
		data, err = s.fetchCover(appID, portrait)
	}
	if err != nil {
		if fb, fbErr := s.fetchFallbackCover(cacheKey, name, appID, portrait); fbErr == nil {
			s.writeCoverMaybeExplicit(w, cacheKey, fb)
			return
		}
	}
	if err != nil {
		// A non-200 (e.g. 404) just means this game has no cover art — normal.
		// Only warn when the network itself couldn't be reached, since that's
		// what makes *every* cover blank.
		var statusErr *httpStatusError
		switch {
		case errors.Is(err, errNoSteamID), errors.As(err, &statusErr):
			// Either the CDN said "no such image", or there was no App ID to
			// ask about — and the second source had nothing either. This game
			// simply has no art, which is not worth a warning.
			coverMisses.Store(missKey, time.Now())
		default:
			coverFailOnce.Do(func() {
				s.Daemon.Log.Log("warn", "cover art can't be loaded — this network can't reach "+
					"Steam's CDN or the image-proxy fallback ("+err.Error()+")")
			})
		}
		w.WriteHeader(http.StatusNotFound)
		return
	}
	s.writeCoverMaybeExplicit(w, cacheKey, data)
}

func writeCover(w http.ResponseWriter, data []byte) {
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=604800")
	_, _ = w.Write(data)
}

// writeCoverMaybeExplicit is writeCover plus the header a caller needs to know
// whether to blur what it just received.
//
// An <img src> cannot read a header, which is why the flag is also on the game
// payload — this is for callers that fetch the bytes themselves.
func (s *Server) writeCoverMaybeExplicit(w http.ResponseWriter, cacheKey string, data []byte) {
	if s.CoverIsExplicit(cacheKey) {
		w.Header().Set("X-Cover-Explicit", "1")
	}
	writeCover(w, data)
}

func (s *Server) coverCachePath(appID string, portrait bool) string {
	name := appID + ".jpg"
	if portrait {
		name = appID + "_p.jpg"
	}
	return filepath.Join(s.Daemon.Paths.HomeDir, "covers", name)
}

// writeCoverCache stores art under a key, for whichever source produced it.
// Cached the same way regardless of where it came from: a cover on disk is a
// cover, and a later request should not care which source answered first.
func (s *Server) writeCoverCache(key string, portrait bool, data []byte) {
	if len(data) == 0 {
		return
	}
	path := s.coverCachePath(key, portrait)
	_ = os.MkdirAll(filepath.Dir(path), 0o777)
	_ = os.WriteFile(path, data, 0o666)
}

// isNumericID guards against SSRF: only all-digit App IDs ever reach the CDN
// URL template.
func isNumericID(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// fetchCover fetches cover bytes for an App ID, caching on success. It tries
// the Steam CDN hosts directly first; if every one fails (e.g. a campus/ISP
// firewall that blocks Steam), it automatically retries through a public
// image proxy, which fetches the image server-side and hands back a copy.
// Portrait requests fall back to the landscape header when a title has no
// library art.
func (s *Server) fetchCover(appID string, portrait bool) ([]byte, error) {
	var files []string
	if portrait {
		files = []string{"library_600x900.jpg", "header.jpg"}
	} else {
		files = []string{"header.jpg"}
	}

	var lastErr error
	try := func(rawURL string) ([]byte, bool) {
		data, err := fetchImage(rawURL)
		if err != nil {
			lastErr = err
			return nil, false
		}
		s.writeCoverCache(appID, portrait, data)
		return data, true
	}

	coverSource.Lock()
	knownHost, known, viaProxy := coverSource.host, coverSource.known, coverSource.viaProx
	coverSource.Unlock()

	rememberDirect := func(i int) {
		coverSource.Lock()
		coverSource.host, coverSource.known, coverSource.viaProx = i, true, false
		coverSource.Unlock()
	}

	// Fast path: reuse whatever worked last time rather than re-walking every
	// dead host for each image.
	if known {
		for _, f := range files {
			if viaProxy {
				if data, ok := try(imageProxyURL(fmt.Sprintf(steamCDNHosts[0], appID, f))); ok {
					return data, nil
				}
			} else if data, ok := try(fmt.Sprintf(steamCDNHosts[knownHost], appID, f)); ok {
				return data, nil
			}
		}
		// A 404 here means this title has no art — no point re-probing every
		// other source, they'd 404 too.
		var statusErr *httpStatusError
		if errors.As(lastErr, &statusErr) {
			return nil, lastErr
		}
	}

	// Full walk: every direct host, then the proxy. Whatever succeeds becomes
	// the remembered source for subsequent covers.
	for _, f := range files {
		for i, tmpl := range steamCDNHosts {
			if data, ok := try(fmt.Sprintf(tmpl, appID, f)); ok {
				rememberDirect(i)
				return data, nil
			}
		}
	}
	for _, f := range files {
		if data, ok := try(imageProxyURL(fmt.Sprintf(steamCDNHosts[0], appID, f))); ok {
			coverSource.Lock()
			coverSource.known, coverSource.viaProx = true, true
			coverSource.Unlock()
			return data, nil
		}
	}
	return nil, lastErr
}

// imageProxyURL wraps a source image URL in the weserv.nl image proxy, which
// fetches it server-side and serves a copy — so covers work on networks that
// block Steam directly.
func imageProxyURL(src string) string {
	return "https://images.weserv.nl/?url=" + url.QueryEscape(src)
}

func fetchImage(rawURL string) ([]byte, error) {
	resp, err := coverClient.Get(rawURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &httpStatusError{status: resp.Status}
	}
	return io.ReadAll(io.LimitReader(resp.Body, 8<<20)) // 8 MB cap
}
