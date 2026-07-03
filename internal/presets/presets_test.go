package presets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePath_SubstitutesEnvVars(t *testing.T) {
	appdata := t.TempDir()
	t.Setenv("APPDATA", appdata)

	got := ResolvePath("%APPDATA%/Ryujinx/bis/user/save")
	want := filepath.Join(appdata, "Ryujinx", "bis", "user", "save")
	if got != want {
		t.Errorf("ResolvePath = %q, want %q", got, want)
	}
}

func TestResolvePath_CaseInsensitiveVars(t *testing.T) {
	appdata := t.TempDir()
	t.Setenv("APPDATA", appdata)

	got := ResolvePath("%appdata%/RetroArch/saves")
	want := filepath.Join(appdata, "RetroArch", "saves")
	if got != want {
		t.Errorf("ResolvePath (lowercase var) = %q, want %q", got, want)
	}
}

// isolateEnv points every scan location at empty temp dirs so tests never
// touch the real machine's Steam/emulator installs.
func isolateEnv(t *testing.T) {
	t.Helper()
	t.Setenv("APPDATA", filepath.Join(t.TempDir(), "appdata"))
	t.Setenv("PUBLIC", filepath.Join(t.TempDir(), "public"))
	t.Setenv("PROGRAMDATA", filepath.Join(t.TempDir(), "programdata"))
	t.Setenv("LOCALAPPDATA", filepath.Join(t.TempDir(), "localappdata"))
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
}

// hermeticScanner returns a Scanner that cannot reach the machine's real
// Steam userdata or the network.
func hermeticScanner(steamPaths ...string) *Scanner {
	if steamPaths == nil {
		steamPaths = []string{}
	}
	return &Scanner{SteamUserdataPaths: steamPaths}
}

func TestScan_DetectsEmulatorPreset(t *testing.T) {
	isolateEnv(t)
	appdata := t.TempDir()
	t.Setenv("APPDATA", appdata)

	ryujinxSaves := filepath.Join(appdata, "Ryujinx", "bis", "user", "save")
	if err := os.MkdirAll(ryujinxSaves, 0o777); err != nil {
		t.Fatal(err)
	}

	sc := hermeticScanner()
	found := sc.Scan(nil)

	var hit *DiscoveredSave
	for i := range found {
		if found[i].ID == "ryujinx" {
			hit = &found[i]
			break
		}
	}
	if hit == nil {
		t.Fatalf("Ryujinx preset not discovered; got %+v", found)
	}
	if hit.Type != "emulator" || hit.SavePath != ryujinxSaves {
		t.Errorf("Ryujinx discovery wrong: %+v", hit)
	}
}

func TestScan_WrapperSubfoldersBecomeGames(t *testing.T) {
	isolateEnv(t)
	appdata := t.TempDir()
	t.Setenv("APPDATA", appdata)

	goldberg := filepath.Join(appdata, "Goldberg SteamEmu Saves")
	for _, sub := range []string{"1245620", "settings", "MyIndieGame"} {
		if err := os.MkdirAll(filepath.Join(goldberg, sub), 0o777); err != nil {
			t.Fatal(err)
		}
	}

	sc := hermeticScanner()
	found := sc.Scan(nil)

	byID := map[string]DiscoveredSave{}
	for _, d := range found {
		byID[d.ID] = d
	}

	appIDEntry, ok := byID["goldberg-1245620"]
	if !ok {
		t.Fatalf("goldberg AppID subfolder not discovered; got %v", found)
	}
	if appIDEntry.AppID != "1245620" {
		t.Errorf("numeric wrapper subfolder should carry its AppID, got %+v", appIDEntry)
	}
	// AppID 1245620 is in the offline dictionary -> real title, no network.
	if appIDEntry.Name != "Elden Ring" {
		t.Errorf("offline dictionary should resolve 1245620 to Elden Ring, got %q", appIDEntry.Name)
	}

	if _, ok := byID["goldberg-settings"]; ok {
		t.Error("wrapper system dir 'settings' must be excluded")
	}
	if _, ok := byID["goldberg-MyIndieGame"]; !ok {
		t.Error("non-numeric wrapper subfolder should still be discovered")
	}
}

func TestScan_SteamUserdata(t *testing.T) {
	isolateEnv(t)

	userdata := filepath.Join(t.TempDir(), "userdata")
	if err := os.MkdirAll(filepath.Join(userdata, "123456789", "413150"), 0o777); err != nil {
		t.Fatal(err)
	}

	sc := hermeticScanner(userdata)
	found := sc.Scan(nil)

	var hit *DiscoveredSave
	for i := range found {
		if found[i].ID == "steam-123456789-413150" {
			hit = &found[i]
			break
		}
	}
	if hit == nil {
		t.Fatalf("steam userdata game not discovered; got %+v", found)
	}
	if hit.AppID != "413150" {
		t.Errorf("AppID = %q, want 413150", hit.AppID)
	}
	if hit.Name != "Stardew Valley" {
		t.Errorf("offline dictionary should resolve name, got %q", hit.Name)
	}
}

func TestScan_CustomPathsAndNameInference(t *testing.T) {
	isolateEnv(t)

	custom := filepath.Join(t.TempDir(), "GameSaves")
	if err := os.MkdirAll(filepath.Join(custom, "Elden Ring"), 0o777); err != nil {
		t.Fatal(err)
	}

	sc := hermeticScanner()
	found := sc.Scan([]string{custom})

	var hit *DiscoveredSave
	for i := range found {
		if found[i].SavePath == filepath.Join(custom, "Elden Ring") {
			hit = &found[i]
			break
		}
	}
	if hit == nil {
		t.Fatalf("custom path subfolder not discovered; got %+v", found)
	}
	// Name "Elden Ring" should infer AppID 1245620 via the name index.
	if hit.AppID != "1245620" {
		t.Errorf("name-based AppID inference failed: %+v", hit)
	}
}

func TestResolveNames_UsesResolverAndCachesResult(t *testing.T) {
	isolateEnv(t)

	userdata := filepath.Join(t.TempDir(), "userdata")
	// Unknown AppID (not in offline dictionary).
	if err := os.MkdirAll(filepath.Join(userdata, "1", "99999999"), 0o777); err != nil {
		t.Fatal(err)
	}

	cacheFile := filepath.Join(t.TempDir(), "steam-app-cache.json")
	calls := 0
	sc := &Scanner{
		CacheFile:          cacheFile,
		SteamUserdataPaths: []string{userdata},
		ResolveAppName: func(appID string) string {
			calls++
			if appID == "99999999" {
				return "Obscure Test Game"
			}
			return ""
		},
	}

	found := sc.Scan(nil)
	var hit *DiscoveredSave
	for i := range found {
		if found[i].AppID == "99999999" {
			hit = &found[i]
		}
	}
	if hit == nil || hit.Name != "Obscure Test Game" {
		t.Fatalf("resolver name not applied: %+v", hit)
	}
	if calls != 1 {
		t.Errorf("resolver called %d times, want 1", calls)
	}

	// Second scan must hit the disk cache, not the resolver.
	found = sc.Scan(nil)
	for i := range found {
		if found[i].AppID == "99999999" && found[i].Name != "Obscure Test Game" {
			t.Errorf("cached name not used on second scan: %+v", found[i])
		}
	}
	if calls != 1 {
		t.Errorf("resolver should not be called again after caching, total calls = %d", calls)
	}
}
