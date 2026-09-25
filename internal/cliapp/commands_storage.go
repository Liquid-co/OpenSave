package cliapp

import (
	"context"
	"fmt"

	"github.com/opensave/opensave/internal/daemon"
	"github.com/opensave/opensave/internal/snapshot"
)

// cmdStorage shows where snapshot space goes: each game's share, the biggest
// snapshots, free space, and what `opensave prune` would give back.
//
// --compact first has older snapshots share the files they have in common
// (snapshot/shared.go), which the app otherwise does every few hours.
func cmdStorage(d *daemon.Daemon, args []string) int {
	asJSON, args := jsonFlag(args)
	compact := false
	for _, a := range args {
		if a == "--compact" {
			compact = true
		}
	}
	var compacted *snapshot.CompactResult
	if compact {
		res, err := d.CompactSnapshots(context.Background(), 0)
		if err != nil {
			return fail(asJSON, err)
		}
		compacted = &res
	}
	r, err := d.Storage()
	if err != nil {
		return fail(asJSON, err)
	}
	if asJSON {
		if compacted != nil {
			return emitJSON(struct {
				daemon.StorageReport
				Compacted snapshot.CompactResult `json:"compacted"`
			}{r, *compacted})
		}
		return emitJSON(r)
	}

	if compacted != nil {
		switch {
		case compacted.Compacted > 0:
			fmt.Printf("%s %d older snapshot(s) now share the files they have in common %s %s freed\n",
				accent("ok"), compacted.Compacted, symDot(), humanBytes(compacted.Freed))
		default:
			fmt.Printf("%s nothing more to share: the newest snapshot of each branch, pinned ones and any under an hour old stay whole\n", accent("ok"))
		}
		if compacted.Failed > 0 {
			fmt.Printf("  %s\n", dangerText(fmt.Sprintf("%d could not be shared and were left as they were; the log says why", compacted.Failed)))
		}
		fmt.Println()
	}

	section(fmt.Sprintf("Snapshots %s %s in %d", symDot(), humanBytes(r.TotalBytes), r.Snapshots))
	if r.DiskBytes > 0 && r.DiskBytes < r.TotalBytes {
		fmt.Printf("  %s\n", faint(fmt.Sprintf("%s on disk %s %s saved by sharing unchanged files between snapshots",
			humanBytes(r.DiskBytes), symDot(), humanBytes(r.TotalBytes-r.DiskBytes))))
	}
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
		if g.DiskBytes > 0 && g.DiskBytes < g.Bytes {
			extra += ", " + humanBytes(g.DiskBytes) + " on disk"
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
