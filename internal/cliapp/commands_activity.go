package cliapp

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/opensave/opensave/internal/daemon"
)

// cmdActivity is the app's Activity timeline in a terminal: what happened to
// each game's save — what came from another device and what went to one,
// snapshots, play, deletions, restores — newest first, by day. The same thing
// happening again and again (a save changing every few minutes while played)
// is one line with a count, as the app shows it.
func cmdActivity(d *daemon.Daemon, args []string) int {
	asJSON, args := jsonFlag(args)
	limit := 40
	gameID := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--limit" && i+1 < len(args):
			n, err := strconv.Atoi(args[i+1])
			if err != nil || n < 1 {
				return fail(asJSON, fmt.Errorf("--limit takes a number of entries"))
			}
			limit = n
			i++
		case strings.HasPrefix(a, "-"):
			return fail(asJSON, fmt.Errorf("unknown option %q\n\nusage: opensave activity [<gameId>] [--limit N]", a))
		default:
			gameID = a
		}
	}
	names := map[string]string{}
	if games, err := d.Store.ListGames(); err == nil {
		for _, g := range games {
			names[g.ID] = g.Name
		}
	}
	if gameID != "" {
		game, err := d.Store.GetGame(gameID)
		if err != nil {
			return fail(asJSON, unknownGameError(d, gameID, err))
		}
		gameID = game.ID
	}

	report, err := d.Activity(0, gameID, limit)
	if err != nil {
		return fail(asJSON, err)
	}
	if asJSON {
		return emitJSON(report)
	}

	title := "Activity"
	if gameID != "" {
		title = "Activity " + symDot() + " " + names[gameID]
	}
	section(title)
	if len(report.Items) == 0 {
		note("Nothing yet. Syncs, snapshots and play show here as they happen.")
		return 0
	}

	now := time.Now()
	nameWidth := 0
	for _, it := range report.Items {
		if n := len(activityGameName(names, it.GameID)); n > nameWidth {
			nameWidth = n
		}
	}
	nameWidth = min(nameWidth, 28)

	day := ""
	for _, run := range activityRuns(report.Items) {
		at := time.UnixMilli(run.item.AtMs).Local()
		if d := activityDay(at, now); d != day {
			day = d
			fmt.Printf("\n  %s\n", faint(strings.ToUpper(day)))
		}
		line := activityLine(run.item)
		if run.count > 1 {
			line += faint(fmt.Sprintf("  ×%d", run.count))
		}
		game := ""
		if gameID == "" {
			game = padRight(bold(truncateRunes(activityGameName(names, run.item.GameID), 28)), nameWidth) + "  "
		}
		fmt.Printf("  %s  %s%s\n", faint(at.Format("15:04")), game, line)
	}
	if report.More {
		more := "opensave activity"
		if gameID != "" {
			more += " " + gameID
		}
		hint(fmt.Sprintf("%s --limit %d", more, limit*2))
	}
	fmt.Println()
	return 0
}

func activityGameName(names map[string]string, id string) string {
	if n := names[id]; n != "" {
		return n
	}
	return id
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

type activityRun struct {
	item  daemon.ActivityItem
	count int
}

// activityRuns folds the same thing happening to the same game in a row
// into one line, newest first. Play sessions are never folded.
func activityRuns(items []daemon.ActivityItem) []activityRun {
	var out []activityRun
	for _, it := range items {
		if n := len(out); n > 0 {
			last := &out[n-1]
			if last.item.GameID == it.GameID && last.item.Kind == it.Kind && it.Kind != "played" &&
				activityLine(last.item) == activityLine(it) {
				last.count++
				continue
			}
		}
		out = append(out, activityRun{item: it, count: 1})
	}
	return out
}

// activityDay is "Today", "Yesterday", or the date.
func activityDay(at, now time.Time) string {
	y1, m1, d1 := at.Date()
	y2, m2, d2 := now.Date()
	switch {
	case y1 == y2 && m1 == m2 && d1 == d2:
		return "Today"
	case at.Year() == now.AddDate(0, 0, -1).Year() && at.YearDay() == now.AddDate(0, 0, -1).YearDay():
		return "Yesterday"
	case y1 == y2:
		return at.Format("Mon 2 Jan")
	default:
		return at.Format("2 Jan 2006")
	}
}

// activityLine says what happened, in the words the app's timeline uses
// (lib/timeline.js).
func activityLine(it daemon.ActivityItem) string {
	files := "files"
	if it.Files > 0 {
		files = plural(it.Files, "file", "files")
	}
	where := ""
	if it.Detail != "" && it.Kind != "restored" {
		where = " " + it.Detail
	}
	switch it.Kind {
	case "received":
		size := ""
		if it.Bytes > 0 {
			size = faint("  " + humanBytes(it.Bytes))
		}
		return fmt.Sprintf("Got %s from %s%s%s", files, it.Device, where, size)
	case "sent":
		return fmt.Sprintf("%s took %s from here", it.Device, files)
	case "deleted":
		return fmt.Sprintf("Deleted %s as %s did%s", files, it.Device, where)
	case "restored":
		// The detail is "<snapshot id>|<when it was taken>".
		if _, when, ok := strings.Cut(it.Detail, "|"); ok {
			if t, err := time.Parse(time.RFC3339Nano, when); err == nil {
				return "Put back the snapshot of " + t.Local().Format("Mon 2 Jan, 15:04")
			}
		}
		return "Put back a snapshot"
	case "cloud-pulled":
		return fmt.Sprintf("Brought %s's newer save from the cloud", it.Device)
	case "emptied":
		return warnText("Every save file was deleted here")
	case "conflict":
		return warnText(fmt.Sprintf("Changed here and on %s at once", it.Device))
	case "played":
		if it.DurationMs > 0 {
			return "Played for " + playLength(it.DurationMs)
		}
		return "Played"
	case "snapshot":
		switch {
		case it.Device != "":
			return fmt.Sprintf("Kept a copy of %s's save", it.Device)
		case !it.Auto && it.Comment != "":
			return "Snapshot: " + it.Comment
		case it.Comment != "":
			return it.Comment
		default:
			return "Snapshot"
		}
	default:
		return it.Kind
	}
}
