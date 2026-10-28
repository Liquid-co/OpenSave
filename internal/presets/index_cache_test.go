package presets

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// A peer's request checks the catalogue (CatalogueSaveLocation), and the peer
// gives up after thirty seconds; indexing the full catalogue takes about that
// long. So the check never waits on a build, and a build is shared.

const newerGame = `
Newer Game:
  files:
    <winAppData>/NewStudio/NewerGame:
      tags: [save]
      when:
        - os: windows
`

// slowBuilds holds every index build, once it has read the manifest, until
// release; and counts them.
func slowBuilds(t *testing.T, sc *Scanner) (release func(), builds *atomic.Int32) {
	t.Helper()
	gate := make(chan struct{})
	builds = new(atomic.Int32)
	real := buildIndex
	buildIndex = func(yamlPath string) []indexedGame {
		games := real(yamlPath)
		builds.Add(1)
		<-gate
		return games
	}
	var once sync.Once
	release = func() { once.Do(func() { close(gate) }) }
	t.Cleanup(func() {
		release()
		buildsDone(sc)
		buildIndex = real
	})
	return release, builds
}

// buildsDone waits for a running build of sc's index to end.
func buildsDone(sc *Scanner) {
	_, indexPath := sc.manifestPaths()
	c := indexCacheFor(indexPath)
	c.mu.Lock()
	running := c.building
	c.mu.Unlock()
	if running != nil {
		<-running
	}
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for " + what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestCatalogueSaveLocation_DoesNotWaitForANewCatalogue(t *testing.T) {
	sc, appdata := verifyScanner(t)
	folder := filepath.Join(appdata, "MoonStudio", "HollowGame")
	if !sc.CatalogueSaveLocation("Hollow Game", "", folder) {
		t.Fatal("the game's folder was not vouched for to begin with")
	}

	// A newer catalogue lands (the weekly download), and takes a while to index.
	release, _ := slowBuilds(t, sc)
	yamlPath, _ := sc.manifestPaths()
	if err := os.WriteFile(yamlPath, []byte(verifyManifest+newerGame), 0o666); err != nil {
		t.Fatal(err)
	}

	answer := make(chan bool, 1)
	start := time.Now()
	go func() { answer <- sc.CatalogueSaveLocation("Hollow Game", "", folder) }()
	select {
	case ok := <-answer:
		if !ok {
			t.Error("while the new catalogue was indexed, the one before it did not answer")
		}
		if took := time.Since(start); took > catalogueWait+3*time.Second {
			t.Errorf("the check took %v", took)
		}
	case <-time.After(catalogueWait + 10*time.Second):
		t.Fatal("the check waited for the new catalogue to be indexed, as a peer would")
	}

	release()
	buildsDone(sc)
	if !sc.CatalogueSaveLocation("Newer Game", "", filepath.Join(appdata, "NewStudio", "NewerGame")) {
		t.Error("the new catalogue did not answer once indexed")
	}
}

func TestManifestIndex_OneBuildForEveryone(t *testing.T) {
	sc := manifestScanner(t, verifyManifest)
	release, builds := slowBuilds(t, sc)

	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if len(sc.loadManifestIndex()) == 0 {
				t.Error("a caller got no index")
			}
		}()
	}
	waitUntil(t, "the build to start", func() bool { return builds.Load() > 0 })
	time.Sleep(200 * time.Millisecond) // the other callers reach it
	release()
	wg.Wait()

	for i := 0; i < 3; i++ {
		sc.loadManifestIndex()
	}
	if n := builds.Load(); n != 1 {
		t.Errorf("the manifest was indexed %d times, want once", n)
	}
}

func TestManifestIndex_AManifestReplacedWhileReadIsReadAgain(t *testing.T) {
	sc := manifestScanner(t, verifyManifest)
	release, builds := slowBuilds(t, sc)
	yamlPath, indexPath := sc.manifestPaths()

	got := make(chan []indexedGame, 1)
	go func() { got <- sc.loadManifestIndex() }()
	waitUntil(t, "the build to start", func() bool { return builds.Load() > 0 })

	// The download lands while the old copy is read.
	if err := os.WriteFile(yamlPath, []byte(verifyManifest+newerGame), 0o666); err != nil {
		t.Fatal(err)
	}
	// Dated a little back, so the index written next is plainly newer however
	// coarse the filesystem's clock.
	landed := time.Now().Add(-5 * time.Second)
	if err := os.Chtimes(yamlPath, landed, landed); err != nil {
		t.Fatal(err)
	}
	release()

	if !hasGame(<-got, "Newer Game") {
		t.Error("the caller got the index of the manifest that had been replaced")
	}
	// Nor is the old one left on disk as the new one's index, for the next run.
	indexCaches.Delete(indexPath)
	before := builds.Load()
	if !hasGame(sc.loadManifestIndex(), "Newer Game") {
		t.Error("the index on disk is the replaced manifest's")
	}
	if builds.Load() != before {
		t.Error("the index on disk was not taken as current")
	}
}

func TestManifestIndex_NoIndexIsLeftForAManifestItWasNotBuiltFrom(t *testing.T) {
	sc := manifestScanner(t, verifyManifest)
	release, builds := slowBuilds(t, sc)
	yamlPath, indexPath := sc.manifestPaths()

	done := make(chan struct{})
	go func() { sc.loadManifestIndex(); close(done) }()
	waitUntil(t, "the build to read the manifest", func() bool { return builds.Load() > 0 })

	// Replaced while read, by a copy that does not parse.
	if err := os.WriteFile(yamlPath, []byte("Broken Game: [unclosed\n"), 0o666); err != nil {
		t.Fatal(err)
	}
	landed := time.Now().Add(-5 * time.Second)
	if err := os.Chtimes(yamlPath, landed, landed); err != nil {
		t.Fatal(err)
	}
	release()
	<-done

	// An index file newer than the manifest is taken to be its index.
	yamlInfo, _ := os.Stat(yamlPath)
	if idx, err := os.Stat(indexPath); err == nil && idx.ModTime().After(yamlInfo.ModTime()) {
		t.Error("the replaced manifest's index was left on disk as the new copy's")
	}
}

func hasGame(games []indexedGame, name string) bool {
	for _, g := range games {
		if g.Name == name {
			return true
		}
	}
	return false
}
