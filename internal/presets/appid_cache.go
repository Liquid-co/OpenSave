package presets

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// loadAppCache reads the persisted AppID->name cache; errors yield an
// empty cache (best-effort, same as JS).
func loadAppCache(cacheFile string) map[string]string {
	cache := map[string]string{}
	if cacheFile == "" {
		return cache
	}
	raw, err := os.ReadFile(cacheFile)
	if err != nil {
		return cache
	}
	_ = json.Unmarshal(raw, &cache)
	return cache
}

// saveAppCache persists the cache, best-effort.
func saveAppCache(cacheFile string, cache map[string]string) {
	if cacheFile == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(cacheFile), 0o777); err != nil {
		return
	}
	raw, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(cacheFile, raw, 0o666)
}

var steamAPIClient = &http.Client{Timeout: 3 * time.Second}

// steamStoreAPI is where App IDs are looked up. A variable so a test can stand
// in for Steam and exercise each of the answers LookupSteamApp gives.
var steamStoreAPI = "https://store.steampowered.com/api/appdetails"

// ErrSteamAppUnknown means Steam answered and has no such App ID.
//
// Kept apart from a failure to reach Steam at all, because the two call for
// opposite advice: a typo wants "check the number in the store URL", and a
// blocked network wants "this is not the ID's fault". Collapsing both into an
// empty string is what left someone who had typed a correct ID staring at a
// blank cover with no idea which of the two had happened.
var ErrSteamAppUnknown = errors.New("no Steam app has this App ID")

// LookupSteamApp asks the Steam Store API what an App ID is.
//
// Returns the store title on success, ErrSteamAppUnknown when Steam has no
// such app, and any other error when Steam could not be asked.
func LookupSteamApp(appID string) (string, error) {
	url := fmt.Sprintf("%s?appids=%s&filters=basic", steamStoreAPI, appID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	// The Store API rejects requests without a browser-like User-Agent.
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := steamAPIClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("steam store api: %s", resp.Status)
	}

	var payload map[string]struct {
		Success bool `json:"success"`
		Data    struct {
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	entry, ok := payload[appID]
	if !ok || !entry.Success || entry.Data.Name == "" {
		return "", ErrSteamAppUnknown
	}
	return entry.Data.Name, nil
}

// fetchSteamAppName queries the Steam Store API for an AppID's title.
// Returns "" on any failure — the caller keeps the placeholder name.
func fetchSteamAppName(appID string) string {
	name, err := LookupSteamApp(appID)
	if err != nil {
		return ""
	}
	return name
}
