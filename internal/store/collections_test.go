package store

import (
	"errors"
	"strings"
	"testing"
)

func collectionNames(t *testing.T, s *Store) []string {
	t.Helper()
	all, err := s.ListCollections()
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, c := range all {
		out = append(out, c.Name+"="+strings.Join(c.GameIDs, "+"))
	}
	return out
}

func TestFavouritesIsThereFromTheStart(t *testing.T) {
	s := openTestStore(t)
	all, err := s.ListCollections()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].ID != FavouritesID || !all[0].Builtin || all[0].GameIDs == nil {
		t.Errorf("collections on a new database = %+v", all)
	}
	if err := s.RenameCollection(FavouritesID, "Faves"); !errors.Is(err, ErrInvalidCollection) {
		t.Errorf("renaming Favourites: %v", err)
	}
	if err := s.DeleteCollection(FavouritesID); !errors.Is(err, ErrInvalidCollection) {
		t.Errorf("deleting Favourites: %v", err)
	}
}

func TestCollectionsHoldGamesAndLetGo(t *testing.T) {
	s := openTestStore(t)
	for _, id := range []string{"hades", "celeste"} {
		if err := s.CreateGame(Game{ID: id, Name: id, SavePath: t.TempDir(), ActiveBranch: "main"}); err != nil {
			t.Fatal(err)
		}
	}
	rogue, err := s.CreateCollection("  Roguelikes  ")
	if err != nil || rogue.Name != "Roguelikes" {
		t.Fatalf("create: %+v %v", rogue, err)
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(s.SetInCollection(rogue.ID, "hades", true))
	must(s.SetInCollection(rogue.ID, "hades", true)) // twice is fine
	must(s.SetInCollection(FavouritesID, "celeste", true))
	must(s.SetInCollection(FavouritesID, "hades", true))
	if got := strings.Join(collectionNames(t, s), " "); got != "Favourites=celeste+hades Roguelikes=hades" {
		t.Errorf("collections = %s", got)
	}

	// Untracking a game takes it out of every collection.
	must(s.DeleteGame("hades"))
	if got := strings.Join(collectionNames(t, s), " "); got != "Favourites=celeste Roguelikes=" {
		t.Errorf("after untracking hades: %s", got)
	}

	must(s.SetInCollection(FavouritesID, "celeste", false))
	must(s.SetInCollection(FavouritesID, "celeste", false)) // already out
	must(s.RenameCollection(rogue.ID, "Roguelites"))
	must(s.DeleteCollection(rogue.ID))
	if got := strings.Join(collectionNames(t, s), " "); got != "Favourites=" {
		t.Errorf("after rename and delete: %s", got)
	}
	// Deleting a collection leaves its games tracked.
	if _, err := s.GetGame("celeste"); err != nil {
		t.Errorf("a game went with the collection: %v", err)
	}
}

func TestCollectionNamesAreCheckedAndFound(t *testing.T) {
	s := openTestStore(t)
	a, err := s.CreateCollection("Playing now")
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "   ", "playing NOW", "favourites", strings.Repeat("x", MaxCollectionName+1)} {
		if _, err := s.CreateCollection(bad); !errors.Is(err, ErrInvalidCollection) {
			t.Errorf("CreateCollection(%q) = %v, want refused", bad, err)
		}
	}
	if err := s.RenameCollection(a.ID, "Playing Now"); err != nil {
		t.Errorf("renaming to a different case of its own name was refused: %v", err)
	}
	for _, q := range []string{a.ID, "playing now", "  PLAYING NOW "} {
		if c, err := s.FindCollection(q); err != nil || c.ID != a.ID {
			t.Errorf("FindCollection(%q) = %+v, %v", q, c, err)
		}
	}
	if _, err := s.FindCollection("nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("FindCollection(nope) = %v", err)
	}
	if err := s.SetInCollection("nope", "x", true); !errors.Is(err, ErrNotFound) {
		t.Errorf("adding to a missing collection: %v", err)
	}
	if err := s.SetInCollection(a.ID, "no-such-game", true); !errors.Is(err, ErrNotFound) {
		t.Errorf("adding a missing game: %v", err)
	}
	// Two made in the same millisecond still get their own ids.
	b, _ := s.CreateCollection("One")
	c, _ := s.CreateCollection("Two")
	if b.ID == c.ID {
		t.Errorf("two collections share id %s", b.ID)
	}
}
