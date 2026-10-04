package store

import "fmt"

// Mark returns the value a one-time step was taken for, empty if never.
// See migrations/0040_marks.sql.
func (s *Store) Mark(name string) string {
	var v string
	if err := s.db.Get(&v, `SELECT value FROM marks WHERE name = ?`, name); err != nil {
		return ""
	}
	return v
}

// SetMark records that a one-time step was taken for value.
func (s *Store) SetMark(name, value string) error {
	if _, err := s.db.Exec(`INSERT INTO marks (name, value) VALUES (?, ?)
		ON CONFLICT(name) DO UPDATE SET value = excluded.value`, name, value); err != nil {
		return fmt.Errorf("set mark %s: %w", name, err)
	}
	return nil
}
