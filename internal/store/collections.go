package store

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// FavouritesID is the built-in collection a star puts a game in.
const FavouritesID = "favourites"

// MaxCollectionName bounds a collection's name, in characters: it is a chip
// in the filter bar, and a chip has to fit.
const MaxCollectionName = 40

// ErrInvalidCollection is a collection change refused for what it asked:
// an empty or over-long name, one already taken, or a change to Favourites
// that Favourites does not allow.
var ErrInvalidCollection = errors.New("invalid collection")

// Collection is a named group of games.
type Collection struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Position int      `json:"position"`
	Builtin  bool     `json:"builtin"`
	GameIDs  []string `json:"gameIds"`
}

// ListCollections returns every collection with its games, Favourites first
// and the rest in the order they were made.
func (s *Store) ListCollections() ([]Collection, error) {
	var rows []struct {
		ID       string `db:"id"`
		Name     string `db:"name"`
		Position int    `db:"position"`
	}
	if err := s.db.Select(&rows, `SELECT id, name, position FROM collections ORDER BY position, created_ms, id`); err != nil {
		return nil, fmt.Errorf("list collections: %w", err)
	}
	var members []struct {
		CollectionID string `db:"collection_id"`
		GameID       string `db:"game_id"`
	}
	if err := s.db.Select(&members, `SELECT collection_id, game_id FROM collection_games ORDER BY added_ms, game_id`); err != nil {
		return nil, fmt.Errorf("list collection games: %w", err)
	}
	byID := map[string][]string{}
	for _, m := range members {
		byID[m.CollectionID] = append(byID[m.CollectionID], m.GameID)
	}
	out := make([]Collection, 0, len(rows))
	for _, r := range rows {
		ids := byID[r.ID]
		if ids == nil {
			ids = []string{}
		}
		out = append(out, Collection{ID: r.ID, Name: r.Name, Position: r.Position, Builtin: r.ID == FavouritesID, GameIDs: ids})
	}
	return out, nil
}

// cleanCollectionName trims a name and checks it is usable and not taken by
// another collection (ignoring case: "roguelikes" and "Roguelikes" beside each
// other in the filter bar would be two chips for one idea).
func (s *Store) cleanCollectionName(name, exceptID string) (string, error) {
	name = strings.Join(strings.Fields(name), " ")
	if name == "" {
		return "", fmt.Errorf("%w: a collection needs a name", ErrInvalidCollection)
	}
	if utf8.RuneCountInString(name) > MaxCollectionName {
		return "", fmt.Errorf("%w: a collection's name can be at most %d characters", ErrInvalidCollection, MaxCollectionName)
	}
	var n int
	if err := s.db.Get(&n, `SELECT COUNT(*) FROM collections WHERE lower(name) = lower(?) AND id != ?`, name, exceptID); err != nil {
		return "", err
	}
	if n > 0 {
		return "", fmt.Errorf("%w: there is already a collection called %q", ErrInvalidCollection, name)
	}
	return name, nil
}

// CreateCollection makes an empty collection, last in the order.
func (s *Store) CreateCollection(name string) (Collection, error) {
	clean, err := s.cleanCollectionName(name, "")
	if err != nil {
		return Collection{}, err
	}
	now := time.Now().UnixMilli()
	id := fmt.Sprintf("col_%d", now)
	for i := 0; ; i++ {
		var n int
		if err := s.db.Get(&n, `SELECT COUNT(*) FROM collections WHERE id = ?`, id); err != nil {
			return Collection{}, err
		}
		if n == 0 {
			break
		}
		id = fmt.Sprintf("col_%d_%d", now, i+1)
	}
	var next int
	if err := s.db.Get(&next, `SELECT COALESCE(MAX(position), 0) + 1 FROM collections`); err != nil {
		return Collection{}, err
	}
	if _, err := s.db.Exec(`INSERT INTO collections (id, name, position, created_ms) VALUES (?, ?, ?, ?)`, id, clean, next, now); err != nil {
		return Collection{}, fmt.Errorf("create collection: %w", err)
	}
	return Collection{ID: id, Name: clean, Position: next, GameIDs: []string{}}, nil
}

// RenameCollection renames a collection; Favourites keeps its name.
func (s *Store) RenameCollection(id, name string) error {
	if id == FavouritesID {
		return fmt.Errorf("%w: Favourites keeps its name", ErrInvalidCollection)
	}
	clean, err := s.cleanCollectionName(name, id)
	if err != nil {
		return err
	}
	res, err := s.db.Exec(`UPDATE collections SET name = ? WHERE id = ?`, clean, id)
	if err != nil {
		return fmt.Errorf("rename collection %s: %w", id, err)
	}
	return checkRowAffected(res)
}

// DeleteCollection removes a collection; its games are not touched.
// Favourites stays.
func (s *Store) DeleteCollection(id string) error {
	if id == FavouritesID {
		return fmt.Errorf("%w: Favourites can't be deleted", ErrInvalidCollection)
	}
	res, err := s.db.Exec(`DELETE FROM collections WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete collection %s: %w", id, err)
	}
	return checkRowAffected(res)
}

// SetInCollection puts a game in a collection or takes it out. Doing what is
// already so is not an error.
func (s *Store) SetInCollection(collectionID, gameID string, in bool) error {
	var n int
	if err := s.db.Get(&n, `SELECT COUNT(*) FROM collections WHERE id = ?`, collectionID); err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("collection %q: %w", collectionID, ErrNotFound)
	}
	if !in {
		_, err := s.db.Exec(`DELETE FROM collection_games WHERE collection_id = ? AND game_id = ?`, collectionID, gameID)
		return err
	}
	if _, err := s.GetGame(gameID); err != nil {
		return err
	}
	_, err := s.db.Exec(`INSERT OR IGNORE INTO collection_games (collection_id, game_id, added_ms) VALUES (?, ?, ?)`,
		collectionID, gameID, time.Now().UnixMilli())
	return err
}

// FindCollection resolves a collection by id or, ignoring case, by name —
// what someone types at the terminal is more often the name.
func (s *Store) FindCollection(idOrName string) (Collection, error) {
	all, err := s.ListCollections()
	if err != nil {
		return Collection{}, err
	}
	for _, c := range all {
		if c.ID == idOrName {
			return c, nil
		}
	}
	for _, c := range all {
		if strings.EqualFold(c.Name, strings.TrimSpace(idOrName)) {
			return c, nil
		}
	}
	return Collection{}, fmt.Errorf("no collection %q: %w", idOrName, ErrNotFound)
}
