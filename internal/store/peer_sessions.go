package store

import (
	"fmt"
	"time"
)

// Play sessions shared between devices. See migrations/0046_peer_play_sessions.sql
// for what is kept and p2p/playsessions.go for how it travels.

// PeerPlaySession is a session another device told this one about.
type PeerPlaySession struct {
	PeerID    string `db:"peer_id" json:"peerId"`
	GameID    string `db:"game_id" json:"gameId"`
	StartedMs int64  `db:"started_ms" json:"startedMs"`
	EndedMs   int64  `db:"ended_ms" json:"endedMs"`
}

// MaxPeerSession is the longest session believed: a game left running over
// a long weekend is real; a week is a clock or a bug.
const MaxPeerSession = 7 * 24 * time.Hour

// PlaySessionsEndedSince is this device's own sessions that ended at or after
// sinceMs, oldest first, at most limit: what a paired device asks for.
func (s *Store) PlaySessionsEndedSince(sinceMs int64, limit int) ([]PlaySession, error) {
	if limit <= 0 {
		limit = 500
	}
	out := []PlaySession{}
	if err := s.db.Select(&out, `SELECT game_id, started_ms, ended_ms FROM play_sessions
		WHERE ended_ms >= ? ORDER BY ended_ms ASC, started_ms ASC LIMIT ?`, sinceMs, limit); err != nil {
		return nil, fmt.Errorf("play sessions since %d: %w", sinceMs, err)
	}
	return out, nil
}

// AddPeerPlaySessions keeps sessions a paired device sent, and returns how
// many were new. Ones it sent before are kept once; ones that cannot be
// right — ending before they start, longer than MaxPeerSession, ending more
// than a day in this device's future, or naming no game — are dropped
// rather than shown as hours nobody played.
func (s *Store) AddPeerPlaySessions(peerID string, sessions []PlaySession, now time.Time) (int, error) {
	if len(sessions) == 0 {
		return 0, nil
	}
	tx, err := s.db.Beginx()
	if err != nil {
		return 0, fmt.Errorf("begin peer sessions: %w", err)
	}
	defer tx.Rollback()
	latest := now.Add(24 * time.Hour).UnixMilli()
	added := 0
	for _, p := range sessions {
		if p.GameID == "" || len(p.GameID) > 200 || p.StartedMs <= 0 || p.EndedMs < p.StartedMs ||
			p.EndedMs-p.StartedMs > MaxPeerSession.Milliseconds() || p.EndedMs > latest {
			continue
		}
		res, err := tx.Exec(`INSERT OR IGNORE INTO peer_play_sessions (peer_id, game_id, started_ms, ended_ms)
			VALUES (?, ?, ?, ?)`, peerID, p.GameID, p.StartedMs, p.EndedMs)
		if err != nil {
			return 0, fmt.Errorf("keep a session from %s: %w", peerID, err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			added++
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit peer sessions: %w", err)
	}
	return added, nil
}

// LatestPeerSessionEnd is when the newest session kept from a device ended,
// or 0: where the next request to it starts.
func (s *Store) LatestPeerSessionEnd(peerID string) (int64, error) {
	var ms int64
	if err := s.db.Get(&ms, `SELECT COALESCE(MAX(ended_ms), 0) FROM peer_play_sessions WHERE peer_id = ?`, peerID); err != nil {
		return 0, fmt.Errorf("latest session from %s: %w", peerID, err)
	}
	return ms, nil
}

// PeerPlayStatsByGame is each game's totals on each paired device: game id,
// then peer id, then the totals. A game no device told of is absent.
func (s *Store) PeerPlayStatsByGame() (map[string]map[string]PlayStats, error) {
	var rows []struct {
		GameID string `db:"game_id"`
		PeerID string `db:"peer_id"`
		PlayStats
	}
	if err := s.db.Select(&rows, `SELECT game_id, peer_id, COUNT(*) AS sessions,
		SUM(ended_ms - started_ms) AS playtime_ms, MAX(ended_ms) AS last_played_ms
		FROM peer_play_sessions GROUP BY game_id, peer_id`); err != nil {
		return nil, fmt.Errorf("peer play stats: %w", err)
	}
	out := map[string]map[string]PlayStats{}
	for _, r := range rows {
		if out[r.GameID] == nil {
			out[r.GameID] = map[string]PlayStats{}
		}
		out[r.GameID][r.PeerID] = r.PlayStats
	}
	return out, nil
}

// RecentPeerPlaySessions is every paired device's sessions, newest first,
// that ended before beforeMs (0: from now), at most limit.
func (s *Store) RecentPeerPlaySessions(beforeMs int64, limit int) ([]PeerPlaySession, error) {
	q := `SELECT peer_id, game_id, started_ms, ended_ms FROM peer_play_sessions`
	var args []any
	if beforeMs > 0 {
		q += ` WHERE ended_ms < ?`
		args = append(args, beforeMs)
	}
	q += ` ORDER BY ended_ms DESC`
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	out := []PeerPlaySession{}
	if err := s.db.Select(&out, q, args...); err != nil {
		return nil, fmt.Errorf("recent peer sessions: %w", err)
	}
	return out, nil
}
