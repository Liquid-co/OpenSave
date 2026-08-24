package relay

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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

const steamGridAPI = "https://www.steamgriddb.com/api/v2"

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
)

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
	steamGridCache.entries[key] = steamGridEntry{url: url, nsfw: nsfw, at: time.Now()}
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
