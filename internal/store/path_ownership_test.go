package store

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func ownedTestGame(id, path string) Game {
	return Game{ID: id, Name: id, SavePath: path, ActiveBranch: "main", AutoSync: false}
}

func TestNativeOwnershipRejectsDirectAndNestedPrimary(t *testing.T) {
	s := openTestStore(t)
	parent := t.TempDir()
	inner := filepath.Join(parent, "slot")
	if err := os.MkdirAll(inner, 0700); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateGame(ownedTestGame("first", inner)); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{inner, parent, filepath.Join(inner, "later")} {
		if err := s.CreateGame(ownedTestGame("second", path)); !errors.Is(err, ErrGamePathOwned) {
			t.Errorf("second native owner of %q: got %v, want ErrGamePathOwned", path, err)
		}
	}
	games, err := s.ListGames()
	if err != nil || len(games) != 1 || games[0].ID != "first" {
		t.Fatalf("failed collision inserted another native game: %+v %v", games, err)
	}
	if err := s.CreateGame(ownedTestGame("other", t.TempDir())); err != nil {
		t.Fatalf("disjoint ordinary game refused: %v", err)
	}
}

func TestNativeOwnershipProtectsRemoteAutoTrackAndRelocation(t *testing.T) {
	s := openTestStore(t)
	first := t.TempDir()
	if err := s.CreateGame(ownedTestGame("original", first)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateGameUnlessAliased(ownedTestGame("peer-other", first)); !errors.Is(err, ErrGamePathOwned) {
		t.Fatalf("peer auto-track claimed existing primary: %v", err)
	}
	second := t.TempDir()
	if err := s.CreateGame(ownedTestGame("second", second)); err != nil {
		t.Fatal(err)
	}
	g, err := s.GetGame("second")
	if err != nil {
		t.Fatal(err)
	}
	g.SavePath = first
	if err := s.UpdateGame(g); !errors.Is(err, ErrGamePathOwned) {
		t.Fatalf("relocation claimed another game's root: %v", err)
	}
	got, err := s.GetGame("second")
	if err != nil || got.SavePath != second {
		t.Fatalf("failed relocation changed native path: %+v %v", got, err)
	}
}

func TestNativeOwnershipIncludesMappedNamedRootsAndPhysicalAliases(t *testing.T) {
	s := openTestStore(t)
	first := t.TempDir()
	other := t.TempDir()
	if err := s.CreateGame(ownedTestGame("a", first)); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateGame(ownedTestGame("b", other)); err != nil {
		t.Fatal(err)
	}
	extra := t.TempDir()
	if err := s.AddGameRoot("a", "settings", extra); err != nil {
		t.Fatal(err)
	}
	if err := s.AddGameRoot("b", "settings", extra); !errors.Is(err, ErrGamePathOwned) {
		t.Fatalf("second game claimed named root: %v", err)
	}
	if err := s.CreateGame(ownedTestGame("c", filepath.Join(extra, "child"))); !errors.Is(err, ErrGamePathOwned) {
		t.Fatalf("new primary claimed another game's named root: %v", err)
	}
	parent := filepath.Dir(first)
	alias := filepath.Join(parent, "alias-"+filepath.Base(first))
	if err := os.Symlink(first, alias); err == nil {
		if err := s.CreateGame(ownedTestGame("alias", alias)); !errors.Is(err, ErrGamePathOwned) {
			t.Fatalf("physical alias incorrectly became a separate native game: %v", err)
		}
	}
	if err := s.AddGameRoot("b", "unrelated", t.TempDir()); err != nil {
		t.Fatalf("disjoint ordinary named root refused: %v", err)
	}
}

func TestNativeOwnershipConcurrentDistinctGameIDs(t *testing.T) {
	// Two SQLite connections model the daemon and an independent CLI.
	// The transactional check must produce at most one committed owner;
	// SQLITE_BUSY itself is a fail-closed, permitted loser result.
	for n := 0; n < 6; n++ {
		file := filepath.Join(t.TempDir(), "shared.db")
		first, err := Open(file)
		if err != nil {
			t.Fatal(err)
		}
		second, err := Open(file)
		if err != nil {
			first.Close()
			t.Fatal(err)
		}
		root := t.TempDir()
		start := make(chan struct{})
		errs := make([]error, 2)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); <-start; errs[0] = first.CreateGame(ownedTestGame("one", root)) }()
		go func() { defer wg.Done(); <-start; errs[1] = second.CreateGame(ownedTestGame("two", root)) }()
		close(start)
		wg.Wait()
		games, e := first.ListGames()
		if e != nil || len(games) > 1 {
			t.Fatalf("race trial %d: duplicate native ownership %+v, err=%v, results=%v", n, games, e, errs)
		}
		if len(games) == 0 {
			t.Fatalf("race trial %d: neither completed, results=%v", n, errs)
		}
		first.Close()
		second.Close()
	}
}

func TestNativeOwnershipFailsClosedOnUnreadableDatabase(t *testing.T) {
	s := openTestStore(t)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateGame(ownedTestGame("should-not-exist", t.TempDir())); err == nil {
		t.Fatal("closed native DB admitted ordinary game")
	}
	if _, err := s.CreateGameUnlessAliased(ownedTestGame("peer", t.TempDir())); err == nil {
		t.Fatal("closed native DB admitted peer-created game")
	}
}

func TestNativeOwnershipConcurrentNamedRootsAcrossGames(t *testing.T) {
	file := filepath.Join(t.TempDir(), "native.db")
	a, err := Open(file)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := Open(file)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if err := a.CreateGame(ownedTestGame("a", t.TempDir())); err != nil {
		t.Fatal(err)
	}
	if err := a.CreateGame(ownedTestGame("b", t.TempDir())); err != nil {
		t.Fatal(err)
	}
	shared := t.TempDir()
	start := make(chan struct{})
	results := make([]error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); <-start; results[0] = a.AddGameRoot("a", "settings", shared) }()
	go func() { defer wg.Done(); <-start; results[1] = b.AddGameRoot("b", "settings", shared) }()
	close(start)
	wg.Wait()
	n := 0
	for _, id := range []string{"a", "b"} {
		roots, e := a.GameRootPaths(id)
		if e != nil {
			t.Fatal(e)
		}
		if roots["settings"] == shared {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("concurrent named-root writers created %d owners, results: %v", n, results)
	}
}

func TestNativeOwnershipRelocationCannotConsumeOwnExtraRoot(t *testing.T) {
	s := openTestStore(t)
	main := t.TempDir()
	extra := t.TempDir()
	if err := s.CreateGame(ownedTestGame("a", main)); err != nil {
		t.Fatal(err)
	}
	if err := s.AddGameRoot("a", "config", extra); err != nil {
		t.Fatal(err)
	}
	g, err := s.GetGame("a")
	if err != nil {
		t.Fatal(err)
	}
	g.SavePath = extra
	if err := s.UpdateGame(g); !errors.Is(err, ErrGamePathOwned) {
		t.Fatalf("moved primary onto existing named root: %v", err)
	}
	got, err := s.GetGame("a")
	if err != nil || got.SavePath != main {
		t.Fatalf("refused relocation changed native primary: %+v %v", got, err)
	}
}

func TestNativeOwnershipMissingLeafUnderPhysicalAlias(t *testing.T) {
	s := openTestStore(t)
	base := t.TempDir()
	owned := filepath.Join(base, "real", "game")
	if err := os.MkdirAll(owned, 0700); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateGame(ownedTestGame("primary", owned)); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(base, "alias")
	if err := os.Symlink(filepath.Join(base, "real"), alias); err != nil {
		t.Skipf("symlink unavailable on this test host: %v", err)
	}
	// The leaf does not exist, so EvalSymlinks of the full requested
	// path and os.Stat both fail; the resolved parent still overlaps.
	future := filepath.Join(alias, "game", "future-slot")
	if _, err := os.Stat(future); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("test requires missing future leaf, got %v", err)
	}
	if err := s.CreateGame(ownedTestGame("second", future)); !errors.Is(err, ErrGamePathOwned) {
		t.Fatalf("symlinked future leaf admitted a second owner: %v", err)
	}
	if err := s.AddGameRoot("primary", "nested", future); !errors.Is(err, ErrGamePathOwned) {
		t.Fatalf("symlinked future leaf admitted overlapping same-game root: %v", err)
	}
	// Missing and disjoint leaves remain valid for peer/unmapped workflows.
	if err := s.CreateGame(ownedTestGame("disjoint", filepath.Join(alias, "other", "future"))); err != nil {
		t.Fatalf("disjoint future root refused: %v", err)
	}
}

func TestPreparedOwnershipDoesNotHoldSQLiteDuringFilesystemResolution(t *testing.T) {
	s := openTestStore(t)
	owned := t.TempDir()
	if err := s.CreateGame(ownedTestGame("existing", owned)); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	unblock := make(chan struct{})
	defer func() {
		select {
		case <-unblock:
		default:
			close(unblock)
		}
	}()
	result := make(chan error, 1)
	go func() {
		_, err := s.prepareNativePathsFor(t.TempDir(), func(path string) preparedNativePath {
			select {
			case <-started:
			default:
				close(started)
			}
			<-unblock // Simulates an unresponsive NAS or unplugged disk.
			return prepareNativePath(path)
		})
		result <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("resolver was never entered")
	}
	// The exact same Store has SetMaxOpenConns(1). The independent DB read
	// must proceed while the resolver is blocked, not wait for a transaction.
	queryDone := make(chan error, 1)
	go func() {
		_, err := s.GetGame("existing")
		queryDone <- err
	}()
	select {
	case err := <-queryDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("filesystem resolution monopolised the SQLite connection")
	}
	close(unblock)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}

func TestPreparedOwnershipRejectsNewClaimAfterPreflight(t *testing.T) {
	s := openTestStore(t)
	choice := t.TempDir()
	prepared, err := s.prepareNativePaths(choice)
	if err != nil {
		t.Fatal(err)
	}
	// A competing transaction commits after the file identities were read.
	newClaim := t.TempDir()
	if err := s.CreateGame(ownedTestGame("late-owner", newClaim)); err != nil {
		t.Fatal(err)
	}
	tx, err := s.db.Beginx()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := checkGamePathOwnershipTx(tx, prepared, "incoming"); !errors.Is(err, ErrOwnershipClaimsChanged) {
		t.Fatalf("unprepared new claim accepted after preflight: %v", err)
	}
}

func TestPreparedOwnershipRejectsChangedRootAfterPreflight(t *testing.T) {
	s := openTestStore(t)
	primary := t.TempDir()
	if err := s.CreateGame(ownedTestGame("main-game", primary)); err != nil {
		t.Fatal(err)
	}
	candidate := t.TempDir()
	prepared, err := s.prepareNativePaths(candidate)
	if err != nil {
		t.Fatal(err)
	}
	newRoot := t.TempDir()
	if err := s.AddGameRoot("main-game", "config", newRoot); err != nil {
		t.Fatal(err)
	}
	tx, err := s.db.Beginx()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := checkGameLocalRootsTx(tx, prepared, "main-game", ""); !errors.Is(err, ErrOwnershipClaimsChanged) {
		t.Fatalf("unprepared same-game root accepted: %v", err)
	}
}

func TestPreparedOwnershipRetainsCrossGameAndPhysicalAliasExclusion(t *testing.T) {
	s := openTestStore(t)
	owned := t.TempDir()
	if err := s.CreateGame(ownedTestGame("owner", owned)); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(owned, alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	prepared, err := s.prepareNativePaths(filepath.Join(alias, "new-leaf"))
	if err != nil {
		t.Fatal(err)
	}
	tx, err := s.db.Beginx()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := checkGamePathOwnershipTx(tx, prepared, "incoming"); !errors.Is(err, ErrGamePathOwned) {
		t.Fatalf("prepared physical alias escaped ownership: %v", err)
	}
}
