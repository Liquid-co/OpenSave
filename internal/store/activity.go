package store

import (
	"fmt"
	"strings"
)

// Activity kinds: what an ActivityEvent says happened.
const (
	ActivityReceived    = "received"     // files came from another device
	ActivitySent        = "sent"         // another device took files from this one
	ActivityDeleted     = "deleted"      // another device's deletion reached this one
	ActivityRestored    = "restored"     // a snapshot was put back
	ActivityCloudPulled = "cloud-pulled" // a newer save came from the cloud
	ActivityEmptied     = "emptied"      // every save file went at once here
	ActivityConflict    = "conflict"     // this device's save and another's diverged
)

// KeepActivity is how many events are kept; older ones go as new ones come.
const KeepActivity = 5000

// ActivityEvent is one thing that happened to a game's save.
type ActivityEvent struct {
	ID     int64  `db:"id" json:"id"`
	AtMs   int64  `db:"at_ms" json:"atMs"`
	GameID string `db:"game_id" json:"gameId"`
	Kind   string `db:"kind" json:"kind"`
	Device string `db:"device" json:"device,omitempty"`
	Files  int    `db:"files" json:"files,omitempty"`
	Bytes  int64  `db:"bytes" json:"bytes,omitempty"`
	Detail string `db:"detail" json:"detail,omitempty"`
}

// RecordActivity adds an event, and drops the oldest beyond KeepActivity.
// Returns the event with its id.
func (s *Store) RecordActivity(ev ActivityEvent) (ActivityEvent, error) {
	res, err := s.db.Exec(`INSERT INTO activity_events (at_ms, game_id, kind, device, files, bytes, detail)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, ev.AtMs, ev.GameID, ev.Kind, ev.Device, ev.Files, ev.Bytes, ev.Detail)
	if err != nil {
		return ev, fmt.Errorf("record activity: %w", err)
	}
	ev.ID, _ = res.LastInsertId()
	if ev.ID%100 == 0 {
		_, _ = s.db.Exec(`DELETE FROM activity_events WHERE id <= ?`, ev.ID-KeepActivity)
	}
	return ev, nil
}

// ListActivity returns events newest first: before 0 means from now,
// gameID "" means every game's.
func (s *Store) ListActivity(beforeMs int64, gameID string, limit int) ([]ActivityEvent, error) {
	var where []string
	var args []any
	if beforeMs > 0 {
		where = append(where, "at_ms < ?")
		args = append(args, beforeMs)
	}
	if gameID != "" {
		where = append(where, "game_id = ?")
		args = append(args, gameID)
	}
	q := `SELECT * FROM activity_events`
	if len(where) > 0 {
		q += ` WHERE ` + strings.Join(where, " AND ")
	}
	q += ` ORDER BY at_ms DESC, id DESC`
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	out := []ActivityEvent{}
	if err := s.db.Select(&out, q, args...); err != nil {
		return nil, fmt.Errorf("list activity: %w", err)
	}
	return out, nil
}

// LatestActivity is each game's newest event of any of the given kinds.
func (s *Store) LatestActivity(kinds ...string) (map[string]ActivityEvent, error) {
	if len(kinds) == 0 {
		return map[string]ActivityEvent{}, nil
	}
	marks := strings.TrimSuffix(strings.Repeat("?,", len(kinds)), ",")
	args := make([]any, len(kinds))
	for i, k := range kinds {
		args[i] = k
	}
	var rows []ActivityEvent
	if err := s.db.Select(&rows, `SELECT e.* FROM activity_events e
		JOIN (SELECT game_id, MAX(at_ms) AS at_ms FROM activity_events WHERE kind IN (`+marks+`) GROUP BY game_id) latest
		  ON latest.game_id = e.game_id AND latest.at_ms = e.at_ms
		WHERE e.kind IN (`+marks+`)`, append(args, args...)...); err != nil {
		return nil, fmt.Errorf("latest activity: %w", err)
	}
	out := map[string]ActivityEvent{}
	for _, r := range rows {
		out[r.GameID] = r
	}
	return out, nil
}

// RecentPlaySessions is every game's play sessions, newest first.
func (s *Store) RecentPlaySessions(beforeMs int64, limit int) ([]PlaySession, error) {
	q := `SELECT game_id, started_ms, ended_ms FROM play_sessions`
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
	out := []PlaySession{}
	if err := s.db.Select(&out, q, args...); err != nil {
		return nil, fmt.Errorf("recent play sessions: %w", err)
	}
	return out, nil
}

// RecentSnapshots is every game's snapshots, newest first. before is an
// ISO timestamp in the snapshots' own format, or "" for from now.
func (s *Store) RecentSnapshots(before string, limit int) ([]Snapshot, error) {
	q := `SELECT * FROM snapshots`
	var args []any
	if before != "" {
		q += ` WHERE timestamp < ?`
		args = append(args, before)
	}
	q += ` ORDER BY timestamp DESC`
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	out := []Snapshot{}
	if err := s.db.Select(&out, q, args...); err != nil {
		return nil, fmt.Errorf("recent snapshots: %w", err)
	}
	return out, nil
}
