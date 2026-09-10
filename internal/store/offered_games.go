package store

import (
	"fmt"
	"strings"
	"time"
)

// A game a peer syncs that this device has not been given a folder for.
//
// See migrations/0021_offered_games.sql for why an offer is a row of its own
// rather than a game with no save path. In short: an empty save path would
// collide in the "one folder, one game" check (filepath.Clean("") is "."), and
// every consumer of a game — watcher, snapshots, manifest builder — would need
// to learn to skip a game that is not really there.
type OfferedGame struct {
	GameID   string `db:"game_id" json:"gameId"`
	PeerID   string `db:"peer_id" json:"peerId"`
	Name     string `db:"name" json:"name"`
	AppID    string `db:"app_id" json:"appId"`
	CoverURL string `db:"cover_url" json:"coverUrl"`
	// PeerPath is where the game lives on the OFFERING device. It is shown to
	// help a person pick the matching folder here, and is never resolved or
	// written to on this machine.
	PeerPath  string `db:"peer_path" json:"peerPath"`
	FirstSeen string `db:"first_seen" json:"firstSeen"`
}

// RecordOfferedGame notes that a peer syncs a game this device has no folder
// for. Repeated offers of the same game by the same peer refresh the details
// without moving first_seen, so the list keeps its order and a peer that asks
// every few minutes does not keep jumping to the top.
//
// Refused, silently, for a game this device already tracks — by its own id or
// through an alias. An offer is by definition for a game with no folder here,
// and the caller's own check for that is not enough: it runs on the goroutine
// answering the peer, while the user placing that very offer runs on another.
// The peer's request could find no game, the user's placement could then
// create the game and clear the offer, and only afterwards would this write
// land — putting back an offer for a game that now exists. The user placed a
// game and watched the offer for it reappear. One statement, so the check and
// the insert cannot be separated by anything.
func (s *Store) RecordOfferedGame(o OfferedGame) error {
	if strings.TrimSpace(o.GameID) == "" || strings.TrimSpace(o.PeerID) == "" {
		return fmt.Errorf("an offered game needs both a game id and a peer id")
	}
	if o.FirstSeen == "" {
		o.FirstSeen = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := s.db.NamedExec(`
		INSERT INTO offered_games (game_id, peer_id, name, app_id, cover_url, peer_path, first_seen)
		SELECT :game_id, :peer_id, :name, :app_id, :cover_url, :peer_path, :first_seen
		WHERE NOT EXISTS (SELECT 1 FROM games        WHERE id       = :game_id)
		  AND NOT EXISTS (SELECT 1 FROM game_aliases WHERE alias_id = :game_id)
		ON CONFLICT(game_id, peer_id) DO UPDATE SET
			name      = excluded.name,
			app_id    = excluded.app_id,
			cover_url = excluded.cover_url,
			peer_path = excluded.peer_path`,
		o)
	if err != nil {
		return fmt.Errorf("record offered game %s: %w", o.GameID, err)
	}
	return nil
}

// ListOfferedGames returns every outstanding offer, oldest first.
func (s *Store) ListOfferedGames() ([]OfferedGame, error) {
	var out []OfferedGame
	if err := s.db.Select(&out, `
		SELECT game_id, peer_id, name, app_id, cover_url, peer_path, first_seen
		FROM offered_games ORDER BY first_seen, name`); err != nil {
		return nil, fmt.Errorf("list offered games: %w", err)
	}
	return out, nil
}

// OfferedGame returns the offers for one game id, across every peer that made
// them. Empty (not an error) when nothing is offered under that id.
func (s *Store) OfferedGame(gameID string) ([]OfferedGame, error) {
	var out []OfferedGame
	if err := s.db.Select(&out, `
		SELECT game_id, peer_id, name, app_id, cover_url, peer_path, first_seen
		FROM offered_games WHERE game_id = ? ORDER BY first_seen`, gameID); err != nil {
		return nil, fmt.Errorf("look up offered game %s: %w", gameID, err)
	}
	return out, nil
}

// ClearOfferedGame removes every offer for a game, whoever made it.
//
// Called both when the game is placed and when it is declined: in either case
// the question has been answered, and the answer is not per-peer. A second
// device offering the same game was waiting for the same thing — a folder on
// this machine — and once there is one, there is nothing left to ask.
func (s *Store) ClearOfferedGame(gameID string) error {
	if _, err := s.db.Exec(`DELETE FROM offered_games WHERE game_id = ?`, gameID); err != nil {
		return fmt.Errorf("clear offered game %s: %w", gameID, err)
	}
	return nil
}

// ClearOfferedGamesForPeer removes a peer's offers, for use when that peer is
// unpaired. An offer from a device you are no longer paired with cannot be
// acted on — placing it would create a game with nobody to sync it to.
func (s *Store) ClearOfferedGamesForPeer(peerID string) error {
	if _, err := s.db.Exec(`DELETE FROM offered_games WHERE peer_id = ?`, peerID); err != nil {
		return fmt.Errorf("clear offered games for peer %s: %w", peerID, err)
	}
	return nil
}

// How this device answers a peer asking about a game it does not track.
const (
	// UnknownGameTrack guesses a folder from the peer's save path and starts
	// syncing. The original behaviour, and the default.
	UnknownGameTrack = "track"
	// UnknownGameAsk records an offer and syncs nothing until a person
	// chooses where the game lives here.
	UnknownGameAsk = "ask"
)

// ShouldAskBeforeTracking reports whether an unknown game from a peer should
// become an offer rather than being tracked at a guessed folder.
//
// Anything other than an exact "ask" means track. Read that way round on
// purpose: an unrecognised value — a database written by a newer build, a
// hand-edited row — must fall back to the behaviour that syncs, not to the one
// that quietly stops syncing. A user who never chose this setting should never
// discover it by finding their saves stopped moving.
func (s Settings) ShouldAskBeforeTracking() bool {
	return strings.TrimSpace(strings.ToLower(s.UnknownGameFromPeer)) == UnknownGameAsk
}
