package relay

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Artwork lookup for games Steam's own CDN has nothing for.
//
// A cover is fetched from Steam by AppID, which covers most of a library and
// none of the rest: a game sold only on GOG or itch has no AppID at all, and a
// game found under a folder name the manifest does not recognise has none we
// could resolve. SteamGridDB catalogues artwork for both, searchable by name.
//
// It goes through the relay rather than the client because it needs an API
// key. A key compiled into an open-source binary is a key anyone can lift out
// of it, and the first person to do so gets it rate-limited for every user —
// so the relay holds it, exactly as it already holds the Google client secret
// for the OAuth proxy.
//
// The relay answers with a URL, not with image bytes. The client already knows
// how to fetch an image and how to fall back to the image proxy when a network
// blocks the source, so handing it a URL reuses all of that and keeps the
// relay out of the business of serving everybody's cover art.

// steamGridClient is separate from the sync transport: artwork is best-effort
// and must never hold a connection long enough to matter to a sync.
var steamGridClient = &http.Client{Timeout: 8 * time.Second}

// steamGridAPI is a var, not a const, only so a test can point it at a fake
// SteamGridDB. Nothing in production reassigns it.
var steamGridAPI = "https://www.steamgriddb.com/api/v2"

// steamGridCache remembers lookups, including the misses.
//
// Every client that scans asks about the same popular games, and a miss is the
// answer that costs most to recompute — it is two round trips that found
// nothing. Caching misses is what stops a game nobody has art for from being
// looked up once per user per scan.
var steamGridCache struct {
	sync.Mutex
	entries map[string]steamGridEntry
}

type steamGridEntry struct {
	url  string
	nsfw bool
	at   time.Time
}

const (
	steamGridHitTTL  = 24 * time.Hour
	steamGridMissTTL = 6 * time.Hour

	// steamGridMaxEntries bounds the cache.
	//
	// Every distinct game name anyone ever scans used to stay resident for
	// the life of the process: entries past their TTL were ignored on read
	// but never removed. One key serves every OpenSave user through this
	// relay, so "how many game names exist" was the only limit, against a
	// service unit that caps the process at 512 MB.
	//
	// Five thousand covers a very large shared library many times over while
	// costing a few hundred kilobytes.
	steamGridMaxEntries = 5000

	// steamGridDefaultBackoff is how long to stop asking after a rate-limit
	// reply that carries no Retry-After.
	steamGridDefaultBackoff = time.Minute
	// steamGridMaxBackoff caps what a Retry-After can ask for, so a hostile
	// or mistaken header cannot disable artwork for a day.
	steamGridMaxBackoff = 15 * time.Minute
)

// errSteamGridBackoff is returned instead of making a request while the
// upstream has asked us to stop.
//
// It is an error rather than an empty result on purpose: handleCoverLookup
// caches results and deliberately does not cache errors, and "we did not ask"
// must never be stored as "this game has no art" — that would blank a cover
// for six hours because of a momentary limit.
var errSteamGridBackoff = errors.New("steamgriddb: backing off after a rate limit")

// steamGridLimit is when it is safe to call SteamGridDB again.
//
// One key serves every user of this relay, so a rate limit is shared too, and
// the response to one is to stop asking rather than to retry per request. With
// no backoff a limited key means every client's every miss becomes another
// request against a service already refusing them, which is how a brief limit
// becomes a long one.
var steamGridLimit struct {
	sync.Mutex
	until time.Time
}

// steamGridPaused reports whether a backoff is in force, and until when.
func steamGridPaused() (bool, time.Time) {
	steamGridLimit.Lock()
	defer steamGridLimit.Unlock()
	return time.Now().Before(steamGridLimit.until), steamGridLimit.until
}

// steamGridPause stops requests for d, never shortening a longer pause already
// in force.
func steamGridPause(d time.Duration) {
	if d <= 0 {
		d = steamGridDefaultBackoff
	}
	if d > steamGridMaxBackoff {
		d = steamGridMaxBackoff
	}
	until := time.Now().Add(d)
	steamGridLimit.Lock()
	defer steamGridLimit.Unlock()
	if until.After(steamGridLimit.until) {
		steamGridLimit.until = until
	}
}

// retryAfterDelay reads a Retry-After header in either form the RFC allows:
// a number of seconds, or an HTTP date. Zero when absent or unreadable, which
// the caller turns into the default.
func retryAfterDelay(h string) time.Duration {
	h = strings.TrimSpace(h)
	if h == "" {
		return 0
	}
	if secs, err := strconv.Atoi(h); err == nil {
		if secs <= 0 {
			return 0
		}
		return time.Duration(secs) * time.Second
	}
	if when, err := http.ParseTime(h); err == nil {
		if d := time.Until(when); d > 0 {
			return d
		}
	}
	return 0
}

// handleCoverLookup answers "where can I get art for this game?".
//
// GET /api/cover/lookup?name=<game name>[&appId=<numeric>]
//
// 200 with {"url": "..."} when something was found, 404 when nothing was, and
// 503 when this relay has no key configured — which is a different thing from
// "this game has no art" and the client treats it differently.
func (s *Server) handleCoverLookup(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.cfg.SteamGridDBKey == "" {
		writeJSONStatus(w, http.StatusServiceUnavailable, map[string]string{
			"error": "This relay has no SteamGridDB key configured.",
		})
		return
	}

	name := strings.TrimSpace(r.URL.Query().Get("name"))
	appID := strings.TrimSpace(r.URL.Query().Get("appId"))
	if name == "" && appID == "" {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{
			"error": "name or appId is required",
		})
		return
	}

	key := strings.ToLower(name) + "|" + appID
	if cached, ok, fresh := steamGridLookupCached(key); fresh {
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		writeJSONStatus(w, http.StatusOK, map[string]any{"url": cached.url, "nsfw": cached.nsfw})
		return
	}

	found, nsfw, err := s.steamGridArtURL(name, appID)
	if err != nil {
		// A failure to reach SteamGridDB is not "this game has no art", and
		// caching it as one would leave every client blank until the entry
		// expired. Say nothing was found, remember nothing.
		w.WriteHeader(http.StatusNotFound)
		return
	}
	steamGridStore(key, found, nsfw)
	if found == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	writeJSONStatus(w, http.StatusOK, map[string]any{"url": found, "nsfw": nsfw})
}

func steamGridLookupCached(key string) (entry steamGridEntry, ok bool, fresh bool) {
	steamGridCache.Lock()
	defer steamGridCache.Unlock()
	e, seen := steamGridCache.entries[key]
	if !seen {
		return steamGridEntry{}, false, false
	}
	ttl := steamGridHitTTL
	if e.url == "" {
		ttl = steamGridMissTTL
	}
	if time.Since(e.at) > ttl {
		return steamGridEntry{}, false, false
	}
	return e, e.url != "", true
}

func steamGridStore(key, url string, nsfw bool) {
	steamGridCache.Lock()
	defer steamGridCache.Unlock()
	if steamGridCache.entries == nil {
		steamGridCache.entries = map[string]steamGridEntry{}
	}
	if len(steamGridCache.entries) >= steamGridMaxEntries {
		steamGridEvictLocked()
	}
	steamGridCache.entries[key] = steamGridEntry{url: url, nsfw: nsfw, at: time.Now()}
}

// steamGridEvictLocked brings the cache back under its cap.
//
// Expired entries go first: they are dead weight that a read would reject
// anyway, and dropping them is free of any cost to hit rate. Only if that is
// not enough does it drop live entries, oldest first, and it trims below the
// cap rather than to it so a cache sitting exactly at the limit does not
// evict on every single insert.
func steamGridEvictLocked() {
	now := time.Now()
	for k, e := range steamGridCache.entries {
		ttl := steamGridHitTTL
		if e.url == "" {
			ttl = steamGridMissTTL
		}
		if now.Sub(e.at) > ttl {
			delete(steamGridCache.entries, k)
		}
	}

	target := steamGridMaxEntries * 3 / 4
	if len(steamGridCache.entries) <= target {
		return
	}
	type aged struct {
		key string
		at  time.Time
	}
	all := make([]aged, 0, len(steamGridCache.entries))
	for k, e := range steamGridCache.entries {
		all = append(all, aged{k, e.at})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].at.Before(all[j].at) })
	for _, a := range all {
		if len(steamGridCache.entries) <= target {
			return
		}
		delete(steamGridCache.entries, a.key)
	}
}

// steamGridCacheSize reports how many entries are held, for /health and tests.
func steamGridCacheSize() int {
	steamGridCache.Lock()
	defer steamGridCache.Unlock()
	return len(steamGridCache.entries)
}

// steamGridArtURL resolves a game to one artwork URL, or "" when SteamGridDB
// knows the game but has no art for it. nsfw reports that the only art
// available is explicit.
func (s *Server) steamGridArtURL(name, appID string) (artURL string, nsfw bool, err error) {
	gameID, err := s.steamGridGameID(name, appID)
	if err != nil || gameID == 0 {
		return "", false, err
	}

	// Grids are the shelf art a cover slot wants. Asked for in the sizes a
	// tile actually uses, so the relay is not handing back a 4K image to be
	// scaled down on every client.
	// Explicit art is fetched, not filtered out — and reported as explicit, so
	// the client can show it blurred until someone asks to see it. Refusing it
	// outright would leave an adult game with a blank tile forever; showing it
	// unannounced would put it on a shelf someone might have open in company.
	// Blurring is the answer that serves both.
	//
	// Joke art is filtered out entirely: nobody is looking for it, and unlike
	// explicit art there is no case where it is the game's real cover.
	endpoint := fmt.Sprintf(
		"%s/grids/game/%d?dimensions=460x215,920x430&types=static&humor=false",
		steamGridAPI, gameID)
	var payload struct {
		Success bool `json:"success"`
		Data    []struct {
			URL  string `json:"url"`
			NSFW bool   `json:"nsfw"`
		} `json:"data"`
	}
	if err := s.steamGridGet(endpoint, &payload); err != nil {
		return "", false, err
	}
	if !payload.Success || len(payload.Data) == 0 {
		return "", false, nil
	}
	// Prefer art nobody needs to be warned about: an explicit cover is only
	// used when it is all the game has.
	for _, g := range payload.Data {
		if !g.NSFW {
			return g.URL, false, nil
		}
	}
	return payload.Data[0].URL, true, nil
}

// steamGridGameID finds SteamGridDB's id for a game, by Steam AppID when there
// is one and by name otherwise.
//
// The AppID path is exact. The name path is a search, and only an exact
// normalised match is accepted: a search for a game SteamGridDB has never
// heard of still returns its nearest guesses, and showing a user confidently
// wrong art is worse than showing none.
func (s *Server) steamGridGameID(name, appID string) (int, error) {
	if appID != "" {
		var payload struct {
			Success bool `json:"success"`
			Data    struct {
				ID int `json:"id"`
			} `json:"data"`
		}
		err := s.steamGridGet(steamGridAPI+"/games/steam/"+url.PathEscape(appID), &payload)
		if err == nil && payload.Success && payload.Data.ID != 0 {
			return payload.Data.ID, nil
		}
		if name == "" {
			return 0, err
		}
	}

	var payload struct {
		Success bool `json:"success"`
		Data    []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := s.steamGridGet(steamGridAPI+"/search/autocomplete/"+url.PathEscape(name), &payload); err != nil {
		return 0, err
	}
	if !payload.Success {
		return 0, nil
	}
	want := normalizeTitle(name)
	for _, g := range payload.Data {
		if normalizeTitle(g.Name) == want {
			return g.ID, nil
		}
	}
	return 0, nil
}

func (s *Server) steamGridGet(endpoint string, out any) error {
	// Every SteamGridDB call in this file goes through here, which is why the
	// backoff lives here rather than at the handler: one gate covers the
	// search and the artwork fetch, and a lookup that makes two calls cannot
	// slip a second one past it.
	if paused, _ := steamGridPaused(); paused {
		return errSteamGridBackoff
	}

	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.cfg.SteamGridDBKey)
	req.Header.Set("Accept", "application/json")
	resp, err := steamGridClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		// SteamGridDB does not know this game. Not an error: the caller wants
		// to cache that as a miss rather than retry it.
		return nil
	}
	// 429 is the explicit "stop asking" reply, and 503 means the service is
	// already struggling; continuing to ask in either case makes a shared
	// key's problem worse for every user of this relay.
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
		steamGridPause(retryAfterDelay(resp.Header.Get("Retry-After")))
		return errSteamGridBackoff
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("steamgriddb: %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// normalizeTitle reduces a title to letters and digits so punctuation and
// trademark marks cannot make one game look like two.
//
// Deliberately strict about nothing else: this is the check that stops a
// search's nearest guess being accepted as the game asked for.
func normalizeTitle(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		}
	}
	return b.String()
}

// steamGridPausedSeconds is how long the backoff still has to run, 0 when not
// paused. Reported by /health.
func steamGridPausedSeconds() int {
	paused, until := steamGridPaused()
	if !paused {
		return 0
	}
	return int(time.Until(until).Seconds()) + 1
}
