package presets

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Scanner performs the full auto-detection sweep. Zero value is not
// usable; construct with NewScanner.
type Scanner struct {
	// CacheFile is where resolved Steam AppID names persist across runs
	// (~/.opensave/steam-app-cache.json).
	CacheFile string
	// ResolveAppName resolves a Steam AppID to a display name (network
	// call in production, injectable for tests). Empty string = not found.
	ResolveAppName func(appID string) string
	// SteamUserdataPaths overrides the default Steam install locations
	// when non-nil (tests use this to stay hermetic on machines that have
	// a real Steam library).
	SteamUserdataPaths []string
}

// NewScanner builds a production Scanner using the Steam Store API
// resolver.
func NewScanner(cacheFile string) *Scanner {
	return &Scanner{
		CacheFile:      cacheFile,
		ResolveAppName: fetchSteamAppName,
	}
}

// Scan sweeps every known save convention and returns the discovered
// locations with names resolved as well as possible (offline dictionary
// -> disk cache -> Steam Store API).
func (sc *Scanner) Scan(customScanPaths []string) []DiscoveredSave {
	var discovered []DiscoveredSave

	// 1 & 2. Emulator presets and repack wrapper folders.
	for _, p := range presetDefs {
		resolved := ResolvePath(p.Path)
		if !dirExists(resolved) {
			continue
		}
		if p.IsWrapper {
			for _, sub := range listSubdirs(resolved) {
				if wrapperSystemDirs[toLowerASCII(sub)] {
					continue
				}
				d := DiscoveredSave{
					ID:       p.ID + "-" + sub,
					Name:     fmt.Sprintf("%s - Game ID: %s", p.Name, sub),
					Type:     p.Type,
					SavePath: filepath.Join(resolved, sub),
				}
				if isAppID(sub) {
					d.AppID = sub
				}
				discovered = append(discovered, d)
			}
		} else {
			discovered = append(discovered, DiscoveredSave{
				ID:       p.ID,
				Name:     p.Name,
				Type:     p.Type,
				SavePath: resolved,
			})
		}
	}

	// 3. Steam userdata folders (Windows + Linux/SteamOS/Flatpak).
	discovered = append(discovered, sc.scanSteamUserdata(dedupSet(discovered))...)

	// 4. Epic "Saved Games" and GOG "My Games" wrapper folders.
	for _, w := range []struct{ id, name, path string }{
		{"epic-savedgames", "Epic / Saved Games", "%USERPROFILE%/Saved Games"},
		{"gog-mygames", "GOG / My Games", "%USERPROFILE%/Documents/My Games"},
	} {
		resolved := ResolvePath(w.path)
		if !dirExists(resolved) {
			continue
		}
		for _, sub := range listSubdirs(resolved) {
			discovered = append(discovered, DiscoveredSave{
				ID:       w.id + "-" + sanitizeID(sub),
				Name:     sub,
				Type:     "game",
				SavePath: filepath.Join(resolved, sub),
			})
		}
	}

	// 5. Unreal Engine saves: %LOCALAPPDATA%/<Game>/Saved/SaveGames.
	localAppData := ResolvePath("%LOCALAPPDATA%")
	for _, dir := range listSubdirs(localAppData) {
		saveGames := filepath.Join(localAppData, dir, "Saved", "SaveGames")
		if !dirExists(saveGames) {
			continue
		}
		entries, err := os.ReadDir(saveGames)
		if err != nil || len(entries) == 0 {
			continue
		}
		discovered = append(discovered, DiscoveredSave{
			ID:       "ue-" + sanitizeID(dir),
			Name:     dir + " (Epic/Unreal Save)",
			Type:     "game",
			SavePath: saveGames,
		})
	}

	// 6. User-configured custom scan paths (each subfolder = a candidate).
	for _, customPath := range customScanPaths {
		resolved, err := filepath.Abs(customPath)
		if err != nil || !dirExists(resolved) {
			continue
		}
		base := sanitizeID(filepath.Base(resolved))
		for _, sub := range listSubdirs(resolved) {
			discovered = append(discovered, DiscoveredSave{
				ID:       "custom-" + base + "-" + sanitizeID(sub),
				Name:     sub,
				Type:     "game",
				SavePath: filepath.Join(resolved, sub),
			})
		}
	}

	// Infer AppIDs from names for entries that lack one.
	nameIndex := nameToAppIDIndex()
	for i := range discovered {
		if discovered[i].AppID == "" {
			discovered[i].AppID = inferAppIDFromName(discovered[i].Name, nameIndex)
		}
	}

	sc.resolveNames(discovered)
	return discovered
}

// scanSteamUserdata walks Steam's userdata/<user>/<appId> layout across
// all known install locations.
func (sc *Scanner) scanSteamUserdata(seen map[string]bool) []DiscoveredSave {
	steamPaths := sc.SteamUserdataPaths
	if steamPaths == nil {
		home, _ := os.UserHomeDir()
		steamPaths = []string{
			`C:\Program Files (x86)\Steam\userdata`,
			`C:\Program Files\Steam\userdata`,
			filepath.Join(home, ".local/share/Steam/userdata"),
			filepath.Join(home, ".steam/steam/userdata"),
			filepath.Join(home, ".var/app/com.valvesoftware.Steam/.local/share/Steam/userdata"),
		}
	}

	var found []DiscoveredSave
	for _, steamPath := range steamPaths {
		if !dirExists(steamPath) {
			continue
		}
		for _, user := range listSubdirs(steamPath) {
			userPath := filepath.Join(steamPath, user)
			for _, game := range listSubdirs(userPath) {
				gamePath := filepath.Join(userPath, game)
				normalized, err := filepath.Abs(gamePath)
				if err != nil {
					continue
				}
				if seen[normalized] {
					continue
				}
				seen[normalized] = true

				d := DiscoveredSave{
					ID:       fmt.Sprintf("steam-%s-%s", user, game),
					Name:     fmt.Sprintf("Steam User %s - AppID: %s", user, game),
					Type:     "game",
					SavePath: gamePath,
				}
				if isAppID(game) {
					d.AppID = game
				}
				found = append(found, d)
			}
		}
	}
	return found
}

// resolveNames upgrades "Game ID: 12345"-style names to real titles:
// offline dictionary first, then the on-disk cache, then the injected
// resolver (Steam Store API) — concurrently, mirroring the JS
// Promise.allSettled batch.
func (sc *Scanner) resolveNames(discovered []DiscoveredSave) {
	cache := loadAppCache(sc.CacheFile)

	var mu sync.Mutex
	var wg sync.WaitGroup
	cacheDirty := false

	for i := range discovered {
		if discovered[i].AppID == "" {
			continue
		}
		appID := discovered[i].AppID

		if name, ok := popularSteamGames[appID]; ok {
			discovered[i].Name = name
			continue
		}

		mu.Lock()
		cachedName := cache[appID]
		mu.Unlock()
		if cachedName != "" {
			discovered[i].Name = cachedName
			continue
		}

		if sc.ResolveAppName == nil {
			continue
		}
		wg.Add(1)
		go func(i int, appID string) {
			defer wg.Done()
			name := sc.ResolveAppName(appID)
			if name == "" {
				return
			}
			mu.Lock()
			cache[appID] = name
			cacheDirty = true
			mu.Unlock()
			discovered[i].Name = name
		}(i, appID)
	}
	wg.Wait()

	if cacheDirty {
		saveAppCache(sc.CacheFile, cache)
	}
}

func dedupSet(items []DiscoveredSave) map[string]bool {
	seen := make(map[string]bool, len(items))
	for _, d := range items {
		if abs, err := filepath.Abs(d.SavePath); err == nil {
			seen[abs] = true
		}
	}
	return seen
}

func toLowerASCII(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}
