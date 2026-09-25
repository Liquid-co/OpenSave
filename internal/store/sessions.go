package store

import "fmt"

// PlaySession is one stretch of a game running on this device.
type PlaySession struct {
	GameID    string `db:"game_id" json:"gameId"`
	StartedMs int64  `db:"started_ms" json:"startedMs"`
	EndedMs   int64  `db:"ended_ms" json:"endedMs"`
}

// PlayStats sums up a game's sessions.
type PlayStats struct {
	Sessions     int   `db:"sessions" json:"sessions"`
	PlaytimeMs   int64 `db:"playtime_ms" json:"playtimeMs"`
	LastPlayedMs int64 `db:"last_played_ms" json:"lastPlayedMs"`
}

// AddPlaySession records a finished session. One that ends before it starts
// is refused rather than stored as negative time.
func (s *Store) AddPlaySession(gameID string, startedMs, endedMs int64) error {
	if endedMs < startedMs {
		return fmt.Errorf("session for %s ends before it starts", gameID)
	}
	if _, err := s.db.Exec(`INSERT INTO play_sessions (game_id, started_ms, ended_ms) VALUES (?, ?, ?)`,
		gameID, startedMs, endedMs); err != nil {
		return fmt.Errorf("record session for %s: %w", gameID, err)
	}
	return nil
}

// PlayStatsByGame returns every game's totals, keyed by game id; a game never
// played is absent.
func (s *Store) PlayStatsByGame() (map[string]PlayStats, error) {
	var rows []struct {
		GameID string `db:"game_id"`
		PlayStats
	}
	if err := s.db.Select(&rows, `SELECT game_id, COUNT(*) AS sessions,
		SUM(ended_ms - started_ms) AS playtime_ms, MAX(ended_ms) AS last_played_ms
		FROM play_sessions GROUP BY game_id`); err != nil {
		return nil, fmt.Errorf("play stats: %w", err)
	}
	out := make(map[string]PlayStats, len(rows))
	for _, r := range rows {
		out[r.GameID] = r.PlayStats
	}
	return out, nil
}

// ListPlaySessions returns a game's sessions, newest first, at most limit
// (0 for all).
func (s *Store) ListPlaySessions(gameID string, limit int) ([]PlaySession, error) {
	q := `SELECT game_id, started_ms, ended_ms FROM play_sessions WHERE game_id = ? ORDER BY ended_ms DESC`
	args := []any{gameID}
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	var out []PlaySession
	if err := s.db.Select(&out, q, args...); err != nil {
		return nil, fmt.Errorf("list sessions for %s: %w", gameID, err)
	}
	return out, nil
}

// PlayStatsFor returns one game's totals; the zero value when never played.
func (s *Store) PlayStatsFor(gameID string) (PlayStats, error) {
	var st PlayStats
	err := s.db.Get(&st, `SELECT COUNT(*) AS sessions, COALESCE(SUM(ended_ms - started_ms), 0) AS playtime_ms,
		COALESCE(MAX(ended_ms), 0) AS last_played_ms FROM play_sessions WHERE game_id = ?`, gameID)
	if err != nil {
		return PlayStats{}, fmt.Errorf("play stats for %s: %w", gameID, err)
	}
	return st, nil
}
