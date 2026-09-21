package cliapp

import (
	"testing"
	"time"
)

func TestTimeAgo(t *testing.T) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	stamp := func(d time.Duration) string {
		return now.Add(-d).UTC().Format("2006-01-02T15:04:05.000Z")
	}
	cases := []struct {
		in   string
		want string
	}{
		{stamp(0), "just now"},
		{stamp(30 * time.Second), "just now"},
		// A peer's clock a few seconds ahead of ours must not print a
		// negative age.
		{stamp(-5 * time.Second), "just now"},
		{stamp(60 * time.Second), "a minute ago"},
		{stamp(4 * time.Minute), "4 min ago"},
		{stamp(59 * time.Minute), "59 min ago"},
		{stamp(70 * time.Minute), "an hour ago"},
		{stamp(5 * time.Hour), "5 hours ago"},
		{stamp(26 * time.Hour), "yesterday"},
		{stamp(3 * 24 * time.Hour), "3 days ago"},
		// The legacy import wrote plain RFC 3339; it must still read.
		{now.Add(-10 * time.Minute).Format(time.RFC3339), "10 min ago"},
		// Garbage is shown, not swallowed.
		{"not a time", "not a time"},
	}
	for _, c := range cases {
		if got := timeAgo(c.in, now); got != c.want {
			t.Errorf("timeAgo(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	// Beyond a week the date wins; its exact text depends on the local zone,
	// so only the shape is pinned.
	if got := timeAgo(stamp(30*24*time.Hour), now); len(got) < 8 || got[:3] != "on " {
		t.Errorf("a month-old stamp printed as %q, want a date", got)
	}
}
