package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// The second source of cover art, for the games Steam's CDN has nothing for.
//
// Steam is asked first and always: it is exact, it needs no third party, and
// it answers for most of a library. This runs only when that came back with
// nothing — a game sold on GOG or itch has no AppID for Steam to answer about,
// and a game found under a folder name the manifest does not recognise has
// none we could resolve.
//
// The lookup goes through the relay because SteamGridDB needs an API key, and a
// key compiled into an open-source client is one anyone can lift out of it —
// the relay holds it, as it already holds the Google client secret. The relay
// answers with a URL rather than image bytes, so the fetch below reuses the
// existing image path, including its fall back to the image proxy on a network
// that blocks the source.

// artLookupClient is deliberately impatient. A cover is decoration: a scan
// renders hundreds of tiles, and a slow relay must never be what makes a scan
// feel broken.
var artLookupClient = &http.Client{Timeout: 8 * time.Second}

// fallbackArtURL asks the relay where art for this game can be found.
//
// Returns "" for every failure — no relay configured, no key on the relay, the
// game unknown, the network down. A cover is the one thing in this program
// that is allowed to simply not appear.
func (s *Server) fallbackArtURL(name, appID string) string {
	settings, err := s.Daemon.Store.GetSettings()
	if err != nil || strings.TrimSpace(settings.RelayURL) == "" {
		return ""
	}
	base := strings.TrimRight(settings.RelayURL, "/")
	// The relay speaks WebSocket for sync and HTTP for everything else; the
	// stored URL may be either scheme.
	base = strings.Replace(base, "wss://", "https://", 1)
	base = strings.Replace(base, "ws://", "http://", 1)

	q := url.Values{}
	if name != "" {
		q.Set("name", name)
	}
	if appID != "" {
		q.Set("appId", appID)
	}
	resp, err := artLookupClient.Get(base + "/api/cover/lookup?" + q.Encode())
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var payload struct {
		URL string `json:"url"`
	}
	if json.NewDecoder(resp.Body).Decode(&payload) != nil {
		return ""
	}
	if !strings.HasPrefix(payload.URL, "https://") {
		// The relay is another machine's answer, and this one is about to
		// fetch whatever it names. Only https, and never a scheme that could
		// reach something local.
		return ""
	}
	return payload.URL
}

// fetchFallbackCover retrieves art from the second source and caches it under
// the same key the Steam path uses, so a later request is served from disk
// without asking anyone.
func (s *Server) fetchFallbackCover(cacheKey, name, appID string, portrait bool) ([]byte, error) {
	artURL := s.fallbackArtURL(name, appID)
	if artURL == "" {
		return nil, fmt.Errorf("no fallback art for %q", name)
	}
	data, err := fetchImage(artURL)
	if err != nil {
		// Same reasoning as the Steam path: a network that blocks the source
		// directly can still reach it through the image proxy.
		data, err = fetchImage(imageProxyURL(artURL))
		if err != nil {
			return nil, err
		}
	}
	s.writeCoverCache(cacheKey, portrait, data)
	return data, nil
}

// coverKeyForName turns a game name into a cache key that is safe as a
// filename and cannot collide with a numeric App ID.
//
// Hashed rather than slugified: a title can contain anything, including path
// separators, and this value becomes a file name.
func coverKeyForName(name string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(name))))
	return "n" + hex.EncodeToString(sum[:8])
}
