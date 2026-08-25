package presets

import (
	"path/filepath"
	"sync"
	"testing"
)

func resetNameIndex(t *testing.T) {
	t.Helper()
	nameIndexMu.Lock()
	nameIndexCache = nil
	nameIndexMu.Unlock()
	t.Cleanup(func() {
		nameIndexMu.Lock()
		nameIndexCache = nil
		nameIndexMu.Unlock()
	})
}

// A machine that has downloaded a fresh manifest matched names against the
// snapshot compiled in at build time: the 17 MB download drove path detection
// while naming and cover art stayed frozen at release. Games added to the
// manifest since could not resolve at all, and that set only grows between
// releases.
func TestAFreshManifestIsUsedForNaming(t *testing.T) {
	resetNameIndex(t)

	// Stand in for a downloaded manifest holding more than the embedded copy.
	games := make([]indexedGame, 0, len(loadEmbeddedIndex())+1)
	games = append(games, loadEmbeddedIndex()...)
	games = append(games, indexedGame{
		Name: "A Game Released After This Build", SteamID: "999999",
	})
	adoptManifestForNaming(games)

	if got := inferAppIDFromName("A Game Released After This Build", nameToAppIDIndex()); got != "999999" {
		t.Errorf("a game present only in the downloaded manifest resolved to %q, want 999999", got)
	}
}

// A hermetic test elsewhere in the package loads a two-entry manifest. That
// must not blank out naming for everything else in the same process.
func TestASmallerManifestDoesNotReplaceALargerIndex(t *testing.T) {
	resetNameIndex(t)
	before := len(manifestNameIndex())
	if before == 0 {
		t.Skip("no embedded index in this build")
	}

	adoptManifestForNaming([]indexedGame{{Name: "Tiny", SteamID: "1"}})

	if after := len(manifestNameIndex()); after < before {
		t.Errorf("index shrank from %d to %d — a small manifest replaced a larger one", before, after)
	}
}

// An empty load must leave the index alone rather than clearing it.
func TestAnEmptyManifestIsIgnored(t *testing.T) {
	resetNameIndex(t)
	before := len(manifestNameIndex())
	adoptManifestForNaming(nil)
	if after := len(manifestNameIndex()); after != before {
		t.Errorf("index changed from %d to %d on an empty load", before, after)
	}
}

// The scan's worker pool reads this index while a load may still be adopting
// one.
func TestNameIndexIsSafeUnderConcurrentUse(t *testing.T) {
	resetNameIndex(t)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = manifestNameIndex()
			adoptManifestForNaming([]indexedGame{{Name: "Race Me", SteamID: "2"}})
			_ = manifestCompactIndex()
		}()
	}
	wg.Wait()
}

// The adoption has to actually happen during a scan. Testing
// adoptManifestForNaming directly leaves the call site unverified, which is
// how a correct function ends up wired to nothing — the same gap the wrapper
// rule had.
func TestAScanAdoptsTheManifestItLoaded(t *testing.T) {
	resetNameIndex(t)

	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))

	// A game the embedded snapshot cannot know about.
	const name = "A Game Added After This Build Shipped"
	sc := manifestScanner(t, `
`+name+`:
  files:
    "<home>/Saved Games/`+name+`":
      tags: [save]
      when:
        - os: windows
  steam:
    id: 4242424
`)
	sc.SteamRoots = []string{t.TempDir()}
	sc.SteamUserdataPaths = []string{}

	// Start from a known-empty index rather than an unset one. Adoption
	// compares against whatever is in use and refuses anything smaller, and
	// what is in use is never empty — manifestNameIndex seeds itself from the
	// embedded copy. Against 48,000 embedded names a one-game fixture is
	// correctly refused, which would test the refusal rather than the wiring.
	//
	// An empty-but-present index is the one state where this fixture can win,
	// and it is a real state: a build whose embedded index failed to parse.
	nameIndexMu.Lock()
	nameIndexCache = map[string]string{}
	nameIndexMu.Unlock()

	sc.Scan(nil)
	if got := inferAppIDFromName(name, nameToAppIDIndex()); got != "4242424" {
		t.Errorf("after a scan the name resolved to %q, want 4242424 — the loaded "+
			"manifest was not adopted for naming", got)
	}
}
