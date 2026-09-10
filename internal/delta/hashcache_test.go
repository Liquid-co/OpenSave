package delta

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// writeAt writes content and forces an exact modification time, so a test can
// control the stamp the cache keys on rather than hoping the clock cooperates.
func writeAt(t *testing.T, path, content string, mtime time.Time) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatalf("chtimes %s: %v", path, err)
	}
}

func hashOf(t *testing.T, root, rel string) string {
	t.Helper()
	m, err := BuildManifest(root)
	if err != nil {
		t.Fatalf("BuildManifest: %v", err)
	}
	e, ok := m.Files[rel]
	if !ok {
		t.Fatalf("%s missing from manifest (have %v)", rel, m.Files)
	}
	return e.Hash
}

// The cache must actually prevent a re-read. Proven by changing the bytes
// behind its back while keeping size and mtime identical: if the second build
// returns the original hash, it did not open the file.
//
// This also documents the exact staleness window the design accepts, so a
// later reader can see it was chosen rather than overlooked.
func TestHashCache_ReusesEntryWhenSizeAndMtimeAreUnchanged(t *testing.T) {
	ClearHashCache()
	dir := t.TempDir()
	path := filepath.Join(dir, "save.dat")
	stamp := time.Now().Add(-time.Hour).Truncate(time.Second)

	writeAt(t, path, "AAAA", stamp)
	first := hashOf(t, dir, "save.dat")

	// Same length, same mtime, different bytes.
	writeAt(t, path, "BBBB", stamp)
	second := hashOf(t, dir, "save.dat")

	if second != first {
		t.Fatalf("expected the cached hash to be reused, got a fresh read (%s -> %s)", first, second)
	}

	// And the escape hatch works.
	InvalidateRoot(dir)
	third := hashOf(t, dir, "save.dat")
	if third == first {
		t.Errorf("InvalidateRoot did not force a re-read: still %s", third)
	}
}

func TestHashCache_RehashesWhenSizeChanges(t *testing.T) {
	ClearHashCache()
	dir := t.TempDir()
	path := filepath.Join(dir, "save.dat")
	stamp := time.Now().Add(-time.Hour).Truncate(time.Second)

	writeAt(t, path, "AAAA", stamp)
	first := hashOf(t, dir, "save.dat")

	// Different length, deliberately the same mtime: size alone must be
	// enough to invalidate.
	writeAt(t, path, "AAAAA", stamp)
	if got := hashOf(t, dir, "save.dat"); got == first {
		t.Errorf("a size change was served from cache: %s", got)
	}
}

func TestHashCache_RehashesWhenMtimeChanges(t *testing.T) {
	ClearHashCache()
	dir := t.TempDir()
	path := filepath.Join(dir, "save.dat")

	writeAt(t, path, "AAAA", time.Now().Add(-2*time.Hour).Truncate(time.Second))
	first := hashOf(t, dir, "save.dat")

	// Same length, different mtime, different bytes.
	writeAt(t, path, "BBBB", time.Now().Add(-time.Hour).Truncate(time.Second))
	if got := hashOf(t, dir, "save.dat"); got == first {
		t.Errorf("an mtime change was served from cache: %s", got)
	}
}

// InvalidateRoot must clear the subtree, not just the folder named.
func TestHashCache_InvalidateRootReachesNestedFiles(t *testing.T) {
	ClearHashCache()
	dir := t.TempDir()
	nested := filepath.Join(dir, "profiles", "slot1")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(nested, "save.dat")
	stamp := time.Now().Add(-time.Hour).Truncate(time.Second)

	writeAt(t, path, "AAAA", stamp)
	rel := filepath.ToSlash(filepath.Join("profiles", "slot1", "save.dat"))
	first := hashOf(t, dir, rel)

	writeAt(t, path, "BBBB", stamp)
	InvalidateRoot(dir)

	if got := hashOf(t, dir, rel); got == first {
		t.Errorf("nested file survived InvalidateRoot on its ancestor: %s", got)
	}
}

// A sibling whose name merely starts with the same characters must not be
// swept away by a prefix match — "…/Game" must not match "…/GameOther".
func TestHashCache_InvalidateRootDoesNotMatchSiblingPrefix(t *testing.T) {
	ClearHashCache()
	base := t.TempDir()
	keep := filepath.Join(base, "GameOther")
	drop := filepath.Join(base, "Game")
	for _, d := range []string{keep, drop} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	stamp := time.Now().Add(-time.Hour).Truncate(time.Second)
	writeAt(t, filepath.Join(keep, "save.dat"), "AAAA", stamp)
	writeAt(t, filepath.Join(drop, "save.dat"), "AAAA", stamp)

	keepFirst := hashOf(t, keep, "save.dat")
	hashOf(t, drop, "save.dat")

	// Change both, then invalidate only "Game".
	writeAt(t, filepath.Join(keep, "save.dat"), "BBBB", stamp)
	writeAt(t, filepath.Join(drop, "save.dat"), "BBBB", stamp)
	InvalidateRoot(drop)

	if got := hashOf(t, keep, "save.dat"); got != keepFirst {
		t.Errorf("GameOther was evicted by InvalidateRoot(Game): %s", got)
	}
	if got := hashOf(t, drop, "save.dat"); got == keepFirst {
		t.Errorf("Game was not evicted by its own InvalidateRoot")
	}
}

// An entry older than cacheMaxAge must be re-read however unchanged it looks.
// That periodic re-read is the only thing that catches a writer which
// preserves both size and mtime.
func TestHashCache_ExpiredEntryIsReRead(t *testing.T) {
	ClearHashCache()
	dir := t.TempDir()
	path := filepath.Join(dir, "save.dat")
	stamp := time.Now().Add(-time.Hour).Truncate(time.Second)

	writeAt(t, path, "AAAA", stamp)
	first := hashOf(t, dir, "save.dat")

	// Age the stored entry past the limit without waiting an hour.
	key := cacheKeyFor(path)
	hashCache.Lock()
	e, ok := hashCache.entries[key]
	if !ok {
		hashCache.Unlock()
		t.Fatal("expected the file to be cached after the first build")
	}
	e.storedAt = time.Now().Add(-cacheMaxAge - time.Minute)
	hashCache.entries[key] = e
	hashCache.Unlock()

	writeAt(t, path, "BBBB", stamp)
	if got := hashOf(t, dir, "save.dat"); got == first {
		t.Errorf("an entry past cacheMaxAge was still served from cache: %s", got)
	}
}

// A missing file must not leave an entry that later reports it as present.
func TestHashCache_FailedReadDropsTheEntry(t *testing.T) {
	ClearHashCache()
	dir := t.TempDir()
	path := filepath.Join(dir, "save.dat")
	writeAt(t, path, "AAAA", time.Now().Add(-time.Hour).Truncate(time.Second))
	hashOf(t, dir, "save.dat")

	if n, _ := hashCacheStats(); n == 0 {
		t.Fatal("expected an entry after the first build")
	}
	if _, err := cachedHashFile(filepath.Join(dir, "gone.dat"), fakeInfo{}); err == nil {
		t.Fatal("expected an error hashing a file that does not exist")
	}
	// The real file's entry must survive; only the failed one is dropped.
	if _, ok := lookup(cacheKeyFor(path)); !ok {
		t.Error("hashing a missing file evicted an unrelated entry")
	}
}

// The cache is shared by every sync goroutine, so concurrent use must be safe
// and must not corrupt the byte accounting.
func TestHashCache_ConcurrentUseIsSafe(t *testing.T) {
	ClearHashCache()
	dir := t.TempDir()
	stamp := time.Now().Add(-time.Hour).Truncate(time.Second)
	for i := range make([]struct{}, 12) {
		writeAt(t, filepath.Join(dir, string(rune('a'+i))+".dat"), "content", stamp)
	}

	var wg sync.WaitGroup
	for range make([]struct{}, 8) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range make([]struct{}, 10) {
				if _, err := BuildManifest(dir); err != nil {
					t.Errorf("BuildManifest: %v", err)
					return
				}
				InvalidateRoot(dir)
			}
		}()
	}
	wg.Wait()

	_, bytes := hashCacheStats()
	if bytes < 0 {
		t.Errorf("byte accounting went negative: %d", bytes)
	}
}

// Eviction must keep the cache under its cap rather than growing without
// bound — the footprint of this cache is the whole reason it exists.
func TestHashCache_StaysWithinItsBudget(t *testing.T) {
	ClearHashCache()
	// Insert far more than the budget directly: writing 64MB of real files
	// would make this test cost more than it proves.
	big := cacheEntry{
		stamp:    cacheStamp{size: 1, mtimeNs: 1},
		cost:     cacheMaxBytes / 8,
		storedAt: time.Now(),
	}
	for i := range make([]struct{}, 40) {
		big.usedAt = time.Now().Add(time.Duration(i) * time.Millisecond)
		store(filepath.Join("root", string(rune('a'+i))), big)
	}
	n, bytes := hashCacheStats()
	if bytes > cacheMaxBytes {
		t.Errorf("cache is over budget: %d bytes in %d entries (cap %d)", bytes, n, cacheMaxBytes)
	}
	if n == 0 {
		t.Error("eviction emptied the cache entirely; it should trim, not clear")
	}
}

func lookup(key string) (cacheEntry, bool) {
	hashCache.Lock()
	defer hashCache.Unlock()
	e, ok := hashCache.entries[key]
	return e, ok
}

// fakeInfo stands in for a FileInfo on the error path, where the value is
// never read because the read fails first.
type fakeInfo struct{ os.FileInfo }

func (fakeInfo) Size() int64        { return 0 }
func (fakeInfo) ModTime() time.Time { return time.Time{} }

// The budget has to follow the library, because too small is the expensive
// direction: the cache evicts entries it is about to want and starts
// re-reading saves, which is the behaviour it exists to remove.
func TestHashCacheBudget_ScalesWithLibrarySize(t *testing.T) {
	t.Cleanup(func() { SetHashCacheBudgetForGames(0) })

	SetHashCacheBudgetForGames(0)
	if got := HashCacheBudget(); got != cacheMinBytes {
		t.Errorf("empty library budget = %d, want the floor %d", got, cacheMinBytes)
	}
	SetHashCacheBudgetForGames(10)
	if got := HashCacheBudget(); got != cacheMinBytes {
		t.Errorf("small library budget = %d, want the floor %d", got, cacheMinBytes)
	}

	SetHashCacheBudgetForGames(350) // the reported library size
	big := HashCacheBudget()
	if big <= cacheMinBytes {
		t.Errorf("350 games got %d, no more than the floor — the case this exists for", big)
	}
	if big > cacheMaxCeiling {
		t.Errorf("350 games got %d, over the ceiling %d", big, cacheMaxCeiling)
	}

	SetHashCacheBudgetForGames(100000)
	if got := HashCacheBudget(); got != cacheMaxCeiling {
		t.Errorf("an absurd library got %d, want the ceiling %d", got, cacheMaxCeiling)
	}
}

// Lowering the budget must take effect immediately, not at the next insert.
func TestHashCacheBudget_ShrinkingEvictsNow(t *testing.T) {
	ClearHashCache()
	SetHashCacheBudgetForGames(100000) // a large ceiling to fill against
	t.Cleanup(func() { SetHashCacheBudgetForGames(0); ClearHashCache() })

	big := cacheEntry{stamp: cacheStamp{size: 1, mtimeNs: 1}, cost: 8 << 20, storedAt: time.Now()}
	for i := range make([]struct{}, 30) {
		big.usedAt = time.Now().Add(time.Duration(i) * time.Millisecond)
		store(filepath.Join("root", string(rune('a'+i))), big)
	}
	if _, bytes := hashCacheStats(); bytes == 0 {
		t.Fatal("setup: nothing was cached")
	}

	SetHashCacheBudgetForGames(0) // drop to the floor
	if _, bytes := hashCacheStats(); bytes > cacheMinBytes {
		t.Errorf("cache holds %d bytes after the budget dropped to %d", bytes, cacheMinBytes)
	}
}

// A file rewritten within one timestamp tick must not keep its old hash.
//
// Size and mtime identify content only when a second write cannot leave both
// unchanged, and within a filesystem's timestamp granularity it can. Games
// write fixed-size saves, and two writes in quick succession are ordinary — a
// save followed by an immediate re-save, or a write and a fix-up. Handing back
// the first write's hash for the second tells the sync nothing changed, for a
// save that did. Found on Linux, where the kernel tick makes the window wide;
// it passed on Windows only by luck of the clock.
//
// The same-tick condition is forced with Chtimes rather than raced, so the
// test is deterministic on every platform: both writes are stamped with the
// same current time, which is what two writes inside one tick look like.
//
// Deliberately NOT tested here: a program that rewrites a file and then puts
// the OLD mtime back. That defeats any stamp-based cache by construction, and
// is what cacheMaxAge — the hourly forced re-read — exists for.
func TestHashCache_ARewriteInTheSameTickIsNotServedStale(t *testing.T) {
	ClearHashCache()
	dir := t.TempDir()
	path := filepath.Join(dir, "player.sav")
	tick := time.Now()

	if err := os.WriteFile(path, []byte("fullscreen=1"), 0o666); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, tick, tick); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	first, err := cachedHashFile(path, info)
	if err != nil {
		t.Fatal(err)
	}

	// Same size, same stamp, different bytes.
	if err := os.WriteFile(path, []byte("fullscreen=0"), 0o666); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, tick, tick); err != nil {
		t.Fatal(err)
	}
	info, _ = os.Stat(path)
	second, err := cachedHashFile(path, info)
	if err != nil {
		t.Fatal(err)
	}

	if first.Hash == second.Hash {
		t.Errorf("a same-size rewrite with an identical stamp was served the old hash %s; "+
			"the sync would see no change in a save that changed", first.Hash[:12])
	}
}

// The window closes: once a file has been still long enough, its hash IS
// cached, or the cache would never hold anything a game has ever written.
func TestHashCache_ACoolFileIsCached(t *testing.T) {
	ClearHashCache()
	dir := t.TempDir()
	path := filepath.Join(dir, "player.sav")
	if err := os.WriteFile(path, []byte("settled"), 0o666); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Minute)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	if _, err := cachedHashFile(path, info); err != nil {
		t.Fatal(err)
	}
	if entries, _ := hashCacheStats(); entries != 1 {
		t.Errorf("a file modified a minute ago was not cached (%d entries); nothing would ever be", entries)
	}
}

// And a hot file is not — the other half of the same rule.
func TestHashCache_AHotFileIsNotCached(t *testing.T) {
	ClearHashCache()
	dir := t.TempDir()
	path := filepath.Join(dir, "player.sav")
	if err := os.WriteFile(path, []byte("just written"), 0o666); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	if _, err := cachedHashFile(path, info); err != nil {
		t.Fatal(err)
	}
	if entries, _ := hashCacheStats(); entries != 0 {
		t.Errorf("a file written this instant was cached (%d entries); a second write in the "+
			"same tick would now be served its hash", entries)
	}
}
