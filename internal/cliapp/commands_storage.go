package cliapp

import (
	"fmt"

	"github.com/opensave/opensave/internal/daemon"
)

// cmdStorage shows where snapshot space goes: each game's share, the biggest
// snapshots, free space, and what `opensave prune` would give back.
func cmdStorage(d *daemon.Daemon, args []string) int {
	asJSON, _ := jsonFlag(args)
	r, err := d.Storage()
	if err != nil {
		return fail(asJSON, err)
	}
	if asJSON {
		return emitJSON(r)
	}

	section(fmt.Sprintf("Snapshots %s %s in %d", symDot(), humanBytes(r.TotalBytes), r.Snapshots))
	fmt.Printf("  %s\n", faint(r.BackupsDir))
	if r.FreeKnown {
		fmt.Printf("  %s\n", faint(humanBytes(int64(r.FreeBytes))+" free on that drive"))
	}
	if len(r.Games) == 0 {
		note("nothing tracked yet")
		return 0
	}
	fmt.Println()
	for _, g := range r.Games {
		extra := fmt.Sprintf("%d snapshot(s)", g.Snapshots)
		if g.Pinned > 0 {
			extra += fmt.Sprintf(", %d pinned", g.Pinned)
		}
		if g.Reclaimable > 0 {
			extra += ", " + humanBytes(g.Reclaimable) + " to clean up"
		}
		fmt.Printf("  %s %s  %s\n", padRight(bold(g.Name), 32), padRight(humanBytes(g.Bytes), 10), faint(extra))
	}

	if len(r.Biggest) > 0 {
		section("Biggest snapshots")
		for _, s := range r.Biggest {
			pin := ""
			if s.Pinned {
				pin = "  " + accent("pinned")
			}
			fmt.Printf("  %s  %s  %s  %s%s\n", padRight(humanBytes(s.Bytes), 10), padRight(s.GameName, 24), s.SnapshotID, faint(s.Timestamp), pin)
		}
	}

	fmt.Println()
	if r.Reclaimable > 0 {
		note(fmt.Sprintf("%s in %d snapshot(s) is past the limits and would be removed by clean-up", humanBytes(r.Reclaimable), r.ReclaimableSnapshots))
		hint("opensave prune")
	} else {
		note("nothing to clean up: every game is within its limits")
	}
	return 0
}
