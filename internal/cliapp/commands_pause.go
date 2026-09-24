package cliapp

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/opensave/opensave/internal/syncpause"
)

// Pausing syncing from the terminal. The pause lives in the running daemon —
// it is that process that syncs — so these ask it rather than opening the
// database, and say so when there is no daemon to ask.

// cmdPause pauses syncing for a while, or with no duration until resumed.
func cmdPause(args []string) int {
	asJSON, args := jsonFlag(args)
	body := map[string]any{"untilRestart": true}
	if len(args) > 1 || (len(args) == 1 && (args[0] == "-h" || args[0] == "--help")) {
		fmt.Fprintln(os.Stderr, pauseUsage)
		return 1
	}
	if len(args) == 1 {
		d, err := parsePauseDuration(args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n\n%s\n", err, pauseUsage)
			return 1
		}
		body = map[string]any{"minutes": int(d / time.Minute)}
	}
	raw, err := daemonRequest("POST", "/api/sync/pause", body)
	if err != nil {
		return fail(asJSON, err)
	}
	var st syncpause.Status
	_ = json.Unmarshal(raw, &st)
	if asJSON {
		return emitJSON(st)
	}
	success("Syncing paused %s", describePause(st))
	note("snapshots are still taken; everything catches up on `opensave resume`")
	return 0
}

const pauseUsage = `usage: opensave pause [duration]

  Stops this device syncing with your other devices and the cloud backup.
  Snapshots are still taken, and everything catches up when syncing resumes.

    opensave pause        until 'opensave resume', or until OpenSave restarts
    opensave pause 30m    for half an hour (also: 1h, 1h30m, or minutes: 45)`

// parsePauseDuration reads "30m", "1h", "1h30m" or a bare number of minutes,
// rounded to whole minutes and kept between one minute and a day.
func parsePauseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	var d time.Duration
	if n, err := strconv.Atoi(s); err == nil {
		d = time.Duration(n) * time.Minute
	} else if parsed, err := time.ParseDuration(s); err == nil {
		// Checked before rounding: rounded first, "30s" became a minute.
		if parsed < time.Minute {
			return 0, fmt.Errorf("a pause has to last at least a minute")
		}
		d = parsed.Round(time.Minute)
	} else {
		return 0, fmt.Errorf("%q is not a duration — try 30m, 1h or 90", s)
	}
	if d < time.Minute {
		return 0, fmt.Errorf("a pause has to last at least a minute")
	}
	if d > 24*time.Hour {
		return 0, fmt.Errorf("a pause can last at most 24h; with no duration it lasts until you resume")
	}
	return d, nil
}

// cmdResume ends a pause.
func cmdResume(args []string) int {
	asJSON, _ := jsonFlag(args)
	raw, err := daemonRequest("POST", "/api/sync/resume", map[string]any{})
	if err != nil {
		return fail(asJSON, err)
	}
	var res struct {
		Resumed bool `json:"resumed"`
	}
	_ = json.Unmarshal(raw, &res)
	if asJSON {
		return emitJSON(res)
	}
	if res.Resumed {
		success("Syncing resumed")
		note("catching up with your other devices and the cloud now")
	} else {
		note("syncing was not paused")
	}
	return 0
}

// describePause says how long a pause lasts: "for 45 more minutes", "until
// you resume".
func describePause(st syncpause.Status) string {
	if st.UntilRestart {
		return "until you resume (or OpenSave restarts)"
	}
	left := time.Duration(st.RemainingSeconds) * time.Second
	switch {
	case left >= time.Hour:
		h := int(left / time.Hour)
		m := int((left % time.Hour) / time.Minute)
		if m == 0 {
			return fmt.Sprintf("for %dh", h)
		}
		return fmt.Sprintf("for %dh%02dm", h, m)
	case left >= time.Minute:
		return fmt.Sprintf("for %d more minute(s)", int(left.Round(time.Minute)/time.Minute))
	default:
		return "for less than a minute"
	}
}

// runningDaemonPause asks the running daemon whether syncing is paused; the
// zero status when there is no daemon to ask.
func runningDaemonPause() syncpause.Status {
	raw, err := daemonRequest("GET", "/api/sync/pause", nil)
	if err != nil {
		return syncpause.Status{}
	}
	var st syncpause.Status
	_ = json.Unmarshal(raw, &st)
	return st
}
