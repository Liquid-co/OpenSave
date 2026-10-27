package presets

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

// The weekly download puts a new index in place without anyone rebuilding it,
// and what read the old one (device settings) learns of it by the generation.
func TestCatalogueGeneration_MovesWithADownloadedIndex(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("Hollow Game:\n  files:\n    <home>/HollowGame/options.ini:\n      tags: [config]\n"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	sc := &Scanner{CacheFile: filepath.Join(dir, "steam-app-cache.json"), GOOS: "windows", ManifestURL: srv.URL}
	yamlPath, _ := sc.manifestPaths()
	before := CatalogueGeneration()
	sc.downloadManifest(yamlPath)
	if CatalogueGeneration() == before {
		t.Error("a downloaded index left the generation where it was")
	}
}
