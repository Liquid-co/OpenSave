package cliapp

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/opensave/opensave/internal/daemon"
)

// cmdSessions shows when games were played on this device and for how long:
// every game's totals, or one game's recent sessions.
func cmdSessions(d *daemon.Daemon, args []string) int {
	asJSON, args := jsonFlag(args)
	if len(args) > 0 {
		game, err := d.Store.GetGame(args[0])
		if err != nil {
			return fail(asJSON, unknownGameError(d, args[0], err))
		}
		list, err := d.Store.ListPlaySessions(game.ID, 20)
		if err != nil {
			return fail(asJSON, err)
		}
		stats, _ := d.Store.PlayStatsFor(game.ID)
		if asJSON {
			return emitJSON(map[string]any{"sessions": list, "stats": stats})
		}
		section(fmt.Sprintf("%s %s %s played", game.Name, symDot(), playLength(stats.PlaytimeMs)))
		if len(list) == 0 {
			note("not played here since OpenSave began keeping track")
			return 0
		}
		for _, s := range list {
			started := time.UnixMilli(s.StartedMs)
			fmt.Printf("  %s %s  %s\n", symBullet(), started.Local().Format("Mon 2 Jan, 15:04"),
				faint(playLength(s.EndedMs-s.StartedMs)))
		}
		return 0
	}

	games, err := d.Store.ListGames()
	if err != nil {
		return fail(asJSON, err)
	}
	stats, err := d.Store.PlayStatsByGame()
	if err != nil {
		return fail(asJSON, err)
	}
	if asJSON {
		return emitJSON(stats)
	}
	section("Played on this device")
	shown := 0
	for _, g := range games {
		st, ok := stats[g.ID]
		if !ok {
			continue
		}
		shown++
		last := timeAgo(time.UnixMilli(st.LastPlayedMs).UTC().Format("2006-01-02T15:04:05.000Z"), time.Now())
		fmt.Printf("  %s %s  %s  %s\n", symBullet(), padRight(bold(g.Name), 30), padRight(playLength(st.PlaytimeMs), 14), faint("last "+last))
	}
	if shown == 0 {
		note("nothing played here since OpenSave began keeping track")
		hint("opensave wrap <gameId> -- <command>")
	}
	return 0
}

// playLength says how long: "45 min", "3 h 12 min".
func playLength(ms int64) string {
	dur := (time.Duration(ms) * time.Millisecond).Round(time.Minute)
	h, m := int(dur/time.Hour), int(dur%time.Hour/time.Minute)
	switch {
	case h == 0:
		return fmt.Sprintf("%d min", m)
	case m == 0:
		return fmt.Sprintf("%d h", h)
	default:
		return fmt.Sprintf("%d h %d min", h, m)
	}
}

const wrapUsage = `usage: opensave wrap <gameId> -- <command> [args…]
    Runs a game the way Ludusavi's wrap does, bracketed by OpenSave: brings
    the newest save from your other devices first, then runs it, then keeps
    the save as you left it — a snapshot named for the session — and syncs
    it on. For Steam, put this in the game's launch options:
        opensave wrap <gameId> -- %command%`

// cmdWrap runs a game between the two halves of a session. It needs the
// running daemon — the desktop app or `opensave daemon start` — which does
// the syncing and keeps the session; without one the game still runs.
func cmdWrap(args []string) int {
	sep := -1
	for i, a := range args {
		if a == "--" {
			sep = i
			break
		}
	}
	if sep != 1 || sep == len(args)-1 {
		_, _ = os.Stderr.WriteString(wrapUsage + "\n")
		return 1
	}
	gameID, command := args[0], args[sep+1:]

	tracked := true
	if _, err := daemonRequest("GET", "/api/games/"+gameID+"/sessions", nil); err != nil {
		warning("not keeping this session: %v", err)
		tracked = false
	}
	if tracked {
		// The newest save first, so the game starts from it. A sync that
		// finds nobody online is quick; one that is slow is not waited on
		// past the request's own limit.
		if raw, err := daemonRequestSlow("POST", "/api/games/"+gameID+"/sync", map[string]any{}); err == nil {
			var res struct {
				Results map[string]struct {
					Status string `json:"status"`
				} `json:"results"`
			}
			if json.Unmarshal(raw, &res) == nil && len(res.Results) > 0 {
				note(fmt.Sprintf("checked %d device(s) for a newer save", len(res.Results)))
			}
		}
		_, _ = daemonRequest("POST", "/api/games/"+gameID+"/session", map[string]string{"state": "start"})
	}

	cmd := exec.Command(command[0], command[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	runErr := cmd.Run()

	if tracked {
		if _, err := daemonRequestSlow("POST", "/api/games/"+gameID+"/session", map[string]string{"state": "end"}); err != nil {
			warning("the session could not be closed: %v", err)
		}
	}
	if runErr != nil {
		if exit, ok := runErr.(*exec.ExitError); ok {
			return exit.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", runErr)
		return 1
	}
	return 0
}
