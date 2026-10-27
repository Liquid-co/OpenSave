package presets

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

// The daemon reads the index while it starts, before the window opens. An
// index due a rebuild is used as it is, and rebuilt behind it: the window
// waited a minute for the rebuild at the first start after the index format
// changed.
func TestIndexNoWait_UsesTheIndexOnDiskAndRebuildsBehindIt(t *testing.T) {
	sc := manifestScanner(t, `
New Game:
  files:
    "<winAppData>/New/save":
      tags: [save]
`)
	yamlPath, indexPath := sc.manifestPaths()
	waitForRebuild(t, indexPath)

	// An index from before the manifest's last download.
	old := []indexedGame{{Name: "Old Game", Paths: []string{"<winAppData>/Old"}}}
	raw, _ := json.Marshal(old)
	if err := os.WriteFile(indexPath, raw, 0o666); err != nil {
		t.Fatal(err)
	}
	// The manifest downloaded half an hour ago, after the index was built.
	// Both in the past: a filesystem with coarse timestamps can give the
	// rebuilt index the same time as a manifest written moments before it,
	// and it would never count as newer.
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(indexPath, past, past); err != nil {
		t.Fatal(err)
	}
	downloaded := time.Now().Add(-30 * time.Minute)
	if err := os.Chtimes(yamlPath, downloaded, downloaded); err != nil {
		t.Fatal(err)
	}

	got := sc.loadManifestIndexNoWait()
	if len(got) != 1 || got[0].Name != "Old Game" {
		t.Fatalf("got %+v, want the index on disk as it is", got)
	}

	// Rebuilt in the background; what the next start reads.
	deadline := time.Now().Add(30 * time.Second)
	for {
		yamlInfo, err := os.Stat(yamlPath)
		if err != nil {
			t.Fatal(err)
		}
		if games, fresh := indexOnDisk(indexPath, yamlInfo); fresh {
			if len(games) != 1 || games[0].Name != "New Game" {
				t.Fatalf("rebuilt index: %+v", games)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the index was not rebuilt")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got := sc.loadManifestIndexNoWait(); len(got) != 1 || got[0].Name != "New Game" {
		t.Errorf("after the rebuild: %+v", got)
	}
	waitForRebuild(t, indexPath)
}

// waitForRebuild waits for a background index build to be over, so the next
// test can start one of its own.
func waitForRebuild(t *testing.T, indexPath string) {
	t.Helper()
	c := indexCacheFor(indexPath)
	deadline := time.Now().Add(30 * time.Second)
	for {
		c.mu.Lock()
		building := c.building
		c.mu.Unlock()
		if building == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the background rebuild did not finish")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// indexOnDisk is the index file's games, and whether it is newer than the
// manifest it was built from.
func indexOnDisk(indexPath string, yamlInfo os.FileInfo) ([]indexedGame, bool) {
	idxInfo, err := os.Stat(indexPath)
	if err != nil {
		return nil, false
	}
	return readIndexFile(indexPath), idxInfo.ModTime().After(yamlInfo.ModTime())
}

// With no index at all, nothing is waited for either: the embedded one stands
// in (none in a hermetic test), and the index is built behind it.
func TestIndexNoWait_NoIndexYet(t *testing.T) {
	sc := manifestScanner(t, `
New Game:
  files:
    "<winAppData>/New/save":
      tags: [save]
`)
	yamlPath, indexPath := sc.manifestPaths()
	waitForRebuild(t, indexPath)
	downloaded := time.Now().Add(-30 * time.Minute)
	if err := os.Chtimes(yamlPath, downloaded, downloaded); err != nil {
		t.Fatal(err)
	}
	if got := sc.loadManifestIndexNoWait(); len(got) != 0 {
		t.Fatalf("got %+v, want nothing until the index is built", got)
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		if _, err := os.Stat(indexPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the index was not built")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got := sc.loadManifestIndexNoWait(); len(got) != 1 {
		t.Errorf("after the build: %+v", got)
	}
	waitForRebuild(t, indexPath)
}
