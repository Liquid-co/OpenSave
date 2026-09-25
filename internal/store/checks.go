package store

import "fmt"

// SetSnapshotCheck records the outcome of reading a snapshot's archive back:
// problem is empty when it was whole.
func (s *Store) SetSnapshotCheck(id string, checkedMs int64, problem string) error {
	res, err := s.db.Exec(`UPDATE snapshots SET checked_ms = ?, problem = ? WHERE id = ?`, checkedMs, problem, id)
	if err != nil {
		return fmt.Errorf("record check of snapshot %s: %w", id, err)
	}
	return checkRowAffected(res)
}

// CheckSummary is what the last checks found.
type CheckSummary struct {
	// LastCheckedMs is the most recent check of any snapshot; 0 for never.
	LastCheckedMs int64 `json:"lastCheckedMs"`
	// Checked is how many snapshots have been checked at least once, of Total.
	Checked int        `json:"checked"`
	Total   int        `json:"total"`
	Damaged []Snapshot `json:"damaged"`
}

// SnapshotChecks sums up every snapshot's last check.
func (s *Store) SnapshotChecks() (CheckSummary, error) {
	var sum CheckSummary
	if err := s.db.Get(&sum.LastCheckedMs, `SELECT COALESCE(MAX(checked_ms), 0) FROM snapshots`); err != nil {
		return sum, fmt.Errorf("snapshot checks: %w", err)
	}
	if err := s.db.Get(&sum.Checked, `SELECT COUNT(*) FROM snapshots WHERE checked_ms > 0`); err != nil {
		return sum, err
	}
	if err := s.db.Get(&sum.Total, `SELECT COUNT(*) FROM snapshots`); err != nil {
		return sum, err
	}
	sum.Damaged = []Snapshot{}
	if err := s.db.Select(&sum.Damaged, `SELECT * FROM snapshots WHERE problem != '' ORDER BY timestamp DESC`); err != nil {
		return sum, err
	}
	return sum, nil
}
