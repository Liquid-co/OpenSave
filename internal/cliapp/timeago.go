package cliapp

import (
	"fmt"
	"time"
)

// timeAgo renders a stored timestamp relative to now, the way a person would
// say it: "just now", "4 min ago", "yesterday". A moment in the past is what
// the reader wants to place, and "2026-09-21T10:14:03.000Z" makes them do the
// arithmetic. Beyond a week the date itself is the clearer answer.
//
// A timestamp that cannot be parsed is shown as it is, rather than hidden: a
// wrong-looking value is a bug someone can report, and a blank is not.
func timeAgo(iso string, now time.Time) string {
	t, ok := parseSyncTime(iso)
	if !ok {
		return iso
	}
	d := now.Sub(t)
	switch {
	case d < 45*time.Second:
		// Covers a small clock skew between devices too: a stamp a few
		// seconds in the future is "just now", not a negative age.
		return "just now"
	case d < 90*time.Second:
		return "a minute ago"
	case d < time.Hour:
		return fmt.Sprintf("%d min ago", int(d.Minutes()+0.5))
	case d < 90*time.Minute:
		return "an hour ago"
	case d < 24*time.Hour:
		return fmt.Sprintf("%d hours ago", int(d.Hours()+0.5))
	case d < 36*time.Hour:
		return "yesterday"
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%d days ago", int(d.Hours()/24+0.5))
	default:
		return "on " + t.Local().Format("2 Jan 2006")
	}
}

// parseSyncTime accepts both spellings a last-synced stamp has had: the
// millisecond form the daemon writes, and plain RFC 3339 from imports.
func parseSyncTime(iso string) (time.Time, bool) {
	if t, err := time.Parse("2006-01-02T15:04:05.000Z", iso); err == nil {
		return t, true
	}
	if t, err := time.Parse(time.RFC3339, iso); err == nil {
		return t, true
	}
	return time.Time{}, false
}
