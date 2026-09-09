package delta

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// A cache of file hashes, so building a manifest does not re-read bytes that
// have not changed.
//
// Why this exists: BuildManifest hashes every file it walks, and it is called
// on a timer whether or not anything changed — answering a peer's ping (at
// most once per 20s per game), the 60s reconcile backstop, and every incoming
// manifest request. On an idle machine with a paired peer that meant reading
// every byte of every save, forever. Reported as constant disk activity and
// several hundred megabytes of resident memory on a machine doing nothing:
// the reads were invisible because they happen BEFORE the comparison that
// finds nothing to do, so the app correctly reported "no syncs" while
// saturating a hard drive.
//
// The memory was not a leak either. Every pass allocated a fresh []Block for
// every file and dropped it moments later; that churn keeps Go's heap target
// high and the runtime returns pages to the OS lazily. Caching turns a
// repeated allocation into a resident one, which is both smaller and steady.
//
// What makes it safe is what it refuses to assume. An entry is only reused
// when size and modification time both match to the nanosecond, and callers
// that write into a save folder drop the whole folder from the cache rather
// than reasoning about which files they touched. See InvalidateRoot.

// cacheStamp is the observed identity of a file. Both fields must match for a
// cached hash to be reused.
//
// Nanoseconds, not seconds: a game writing a save twice inside one second is
// ordinary, and a second-resolution stamp would call the second write a cache
// hit and hand back the first write's hash.
type cacheStamp struct {
	size    int64
	mtimeNs int64
}

type cacheEntry struct {
	stamp cacheStamp
	entry FileEntry
	// cost is the approximate bytes this entry holds, used to bound the
	// cache. Dominated by the block list.
	cost int
	// storedAt drives the periodic forced re-read. See cacheMaxAge.
	storedAt time.Time
	// usedAt drives eviction when the cache is over budget.
	usedAt time.Time
}

// cacheMaxBytes bounds what the cache holds.
//
// Generous enough that an ordinary save library fits entirely — the whole
// point is to stop re-reading it — and far below the footprint this replaces.
// A library too big for the budget still benefits: the hottest files stay
// cached and the rest are re-read as before, which is strictly no worse than
// the old behaviour.
const (
	// cacheMinBytes is the floor, and the value a small library gets.
	cacheMinBytes = 64 << 20
	// cacheMaxCeiling is the most this will ever hold, however many games are
	// tracked. A cache is meant to replace repeated reading, not to become
	// the memory problem it was added to fix.
	cacheMaxCeiling = 384 << 20
	// cachePerGameBytes is how much the budget grows per tracked game.
	//
	// From measurement rather than taste: a game's manifest costs roughly
	// (files x blocks x ~96 bytes), which for ordinary saves lands in the
	// low hundreds of kilobytes. A fixed 64 MB was sized for a normal
	// library and is not enough for someone tracking 350 games — the cache
	// sits at its cap and evicts entries it is about to want again, which
	// quietly reinstates the repeated reading it exists to remove.
	cachePerGameBytes = 512 << 10
)

// cacheMaxBytes is the current budget. Not a constant: it scales with how many
// games are tracked, because the right size for ten games and for three
// hundred differ by more than an order of magnitude.
var cacheMaxBytes = cacheMinBytes

// SetHashCacheBudgetForGames sizes the cache for a library of n games.
//
// Called when the set of tracked games changes. Clamped at both ends: a floor
// so a small library still caches usefully, and a ceiling so a very large one
// cannot turn this into the problem it was written to solve.
func SetHashCacheBudgetForGames(n int) {
	budget := n * cachePerGameBytes
	if budget < cacheMinBytes {
		budget = cacheMinBytes
	}
	if budget > cacheMaxCeiling {
		budget = cacheMaxCeiling
	}

	hashCache.Lock()
	cacheMaxBytes = budget
	over := hashCache.bytes > cacheMaxBytes
	hashCache.Unlock()

	// Shrinking mid-run has to take effect now rather than at the next
	// insert, or the cache would sit above a budget that has just been
	// lowered until something happened to write to it.
	if over {
		hashCache.Lock()
		evictLocked()
		hashCache.Unlock()
	}
}

// HashCacheBudget reports the current budget, for tests and diagnostics.
func HashCacheBudget() int {
	hashCache.Lock()
	defer hashCache.Unlock()
	return cacheMaxBytes
}

// cacheMaxAge forces a re-read of an entry that has been trusted for this
// long, however unchanged it looks.
//
// Size and mtime identify content well but not perfectly: a program that
// restores a file's mtime after writing it, or writes the same number of
// bytes within the same nanosecond stamp, would be missed. Syncthing has the
// same exposure and answers it the same way, with a periodic full rescan.
// This is that rescan, staggered per file rather than run as one sweep.
const cacheMaxAge = time.Hour

// approxBlockCost is the per-block memory charge: the struct's two ints and a
// string header, plus the 64 hex characters of the hash itself.
const approxBlockCost = 8 + 16 + 8 + 64

var hashCache = struct {
	sync.Mutex
	entries map[string]cacheEntry
	bytes   int
}{entries: map[string]cacheEntry{}}

// cachedHashFile returns the hash of path, reusing a previous result when the
// file's size and modification time are unchanged.
//
// info is the FileInfo the caller's directory walk already produced, so a hit
// costs no syscall at all beyond that walk.
//
// The returned FileEntry SHARES its Blocks slice with the cached copy, so
// callers must treat it as read-only. Every caller does today — the block
// lists are compared, never assigned into — and copying on each hit would
// reintroduce the per-pass allocation this cache exists to remove. If you
// need to modify one, copy it first.
func cachedHashFile(path string, info os.FileInfo) (FileEntry, error) {
	key := cacheKeyFor(path)
	stamp := cacheStamp{size: info.Size(), mtimeNs: info.ModTime().UnixNano()}

	hashCache.Lock()
	hit, ok := hashCache.entries[key]
	if ok && hit.stamp == stamp && time.Since(hit.storedAt) < cacheMaxAge {
		hit.usedAt = time.Now()
		hashCache.entries[key] = hit
		hashCache.Unlock()
		return hit.entry, nil
	}
	hashCache.Unlock()

	entry, err := HashFile(path)
	if err != nil {
		// A file that could not be read must not leave a stale entry behind
		// claiming otherwise.
		invalidate(key)
		return FileEntry{}, err
	}

	// Re-stat rather than trusting the pre-read FileInfo. A file written
	// while it was being hashed would otherwise be stored under the stamp it
	// had before the write, and every later pass would return the torn read
	// as a hit. Storing the post-read stamp means the next pass sees a
	// mismatch and re-reads, which is the safe direction.
	after, statErr := os.Stat(path)
	if statErr != nil || after.Size() != stamp.size || after.ModTime().UnixNano() != stamp.mtimeNs {
		// Returned but not stored. This is the same value the uncached code
		// always returned for a file written mid-read, so nothing is made
		// worse; what must not happen is that value becoming a cache hit for
		// every later pass.
		return entry, nil
	}

	store(key, cacheEntry{
		stamp:    stamp,
		entry:    entry,
		cost:     entryCost(entry),
		storedAt: time.Now(),
		usedAt:   time.Now(),
	})
	return entry, nil
}

func entryCost(e FileEntry) int {
	return 128 + len(e.Blocks)*approxBlockCost
}

// isCaseInsensitiveFS is true only where the filesystem is reliably
// case-insensitive.
//
// Windows only, deliberately. Lowering a key on a case-SENSITIVE filesystem
// would merge two genuinely different files — "save.dat" and "Save.dat" — into
// one cache entry and hand back the wrong hash for one of them. macOS is
// usually case-insensitive but can be formatted either way, and there is no
// cheap way to know which; the cost of not lowering there is a duplicate
// entry, which is wasteful rather than wrong.
var isCaseInsensitiveFS = runtime.GOOS == "windows"

// cacheKeyFor normalises a path so the same file looked up two ways shares one
// entry, and so InvalidateRoot's prefix match cannot be defeated by a caller
// spelling the root with different capitalisation than the walk that filled
// the cache.
func cacheKeyFor(path string) string {
	clean := filepath.Clean(path)
	if isCaseInsensitiveFS {
		clean = strings.ToLower(clean)
	}
	return clean
}

func store(key string, e cacheEntry) {
	hashCache.Lock()
	defer hashCache.Unlock()
	if old, ok := hashCache.entries[key]; ok {
		hashCache.bytes -= old.cost
	}
	hashCache.entries[key] = e
	hashCache.bytes += e.cost
	if hashCache.bytes > cacheMaxBytes {
		evictLocked()
	}
}

// evictLocked drops the least recently used entries until the cache is
// comfortably under budget. Called only on overflow, so the scan it costs is
// rare.
//
// Trimming to a fraction of the cap rather than to the cap itself stops a
// cache sitting exactly at the limit from evicting on every single insert.
func evictLocked() {
	type aged struct {
		key string
		at  time.Time
	}
	all := make([]aged, 0, len(hashCache.entries))
	for k, v := range hashCache.entries {
		all = append(all, aged{k, v.usedAt})
	}
	// Partial selection would be faster, but this runs only on overflow and
	// clarity is worth more here than the microseconds.
	for i := 1; i < len(all); i++ {
		for j := i; j > 0 && all[j].at.Before(all[j-1].at); j-- {
			all[j], all[j-1] = all[j-1], all[j]
		}
	}
	target := cacheMaxBytes * 3 / 4
	for _, a := range all {
		if hashCache.bytes <= target {
			return
		}
		hashCache.bytes -= hashCache.entries[a.key].cost
		delete(hashCache.entries, a.key)
	}
}

func invalidate(key string) {
	hashCache.Lock()
	defer hashCache.Unlock()
	if old, ok := hashCache.entries[key]; ok {
		hashCache.bytes -= old.cost
		delete(hashCache.entries, key)
	}
}

// InvalidateRoot forgets every cached hash at or below root.
//
// Call it after writing anything into a save folder. It is deliberately
// coarse: the alternative is for each writer to name the files it touched,
// and a writer that names one file too few produces a manifest that describes
// content the folder no longer holds — a silent wrong answer, in the code
// that decides what to copy over someone's save. Dropping the folder cannot
// be wrong, only wasteful, and the waste is one re-read.
//
// This matters more than it looks: OpenSave stamps a pulled file with the
// PEER'S modification time, and conflict resolution rewrites mtimes across a
// whole folder. Neither reliably moves an mtime forward, so neither can be
// relied on to invalidate an entry by itself.
func InvalidateRoot(root string) {
	if strings.TrimSpace(root) == "" {
		return
	}
	prefix := cacheKeyFor(root)
	hashCache.Lock()
	defer hashCache.Unlock()
	for k, v := range hashCache.entries {
		if k == prefix || strings.HasPrefix(k, prefix+string(filepath.Separator)) {
			hashCache.bytes -= v.cost
			delete(hashCache.entries, k)
		}
	}
}

// ClearHashCache empties the cache. For tests, and for anything that needs a
// guaranteed cold read.
func ClearHashCache() {
	hashCache.Lock()
	defer hashCache.Unlock()
	hashCache.entries = map[string]cacheEntry{}
	hashCache.bytes = 0
}

// hashCacheStats reports what the cache holds, for tests.
func hashCacheStats() (entries, bytes int) {
	hashCache.Lock()
	defer hashCache.Unlock()
	return len(hashCache.entries), hashCache.bytes
}
