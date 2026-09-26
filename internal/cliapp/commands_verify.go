package cliapp

import (
	"context"
	"fmt"

	"github.com/opensave/opensave/internal/daemon"
)

// cmdVerify reads every snapshot back — of one game, or of all of them — and
// says whether each can still be restored. It exits 1 when one cannot, so a
// script can act on it.
//
//	--repair          put back, from the cloud, the damaged ones that have a whole copy there
//	--remove-damaged  remove from the history the ones that cannot be restored
func cmdVerify(d *daemon.Daemon, args []string) int {
	asJSON, args := jsonFlag(args)
	var rest []string
	repair, remove := false, false
	for _, a := range args {
		switch a {
		case "--repair":
			repair = true
		case "--remove-damaged":
			remove = true
		default:
			rest = append(rest, a)
		}
	}
	args = rest
	if repair {
		return cmdVerifyRepair(d, asJSON)
	}
	if remove {
		return cmdVerifyRemove(d, asJSON)
	}
	gameID := ""
	if len(args) > 0 {
		game, err := d.Store.GetGame(args[0])
		if err != nil {
			return fail(asJSON, unknownGameError(d, args[0], err))
		}
		gameID = game.ID
	}
	report, err := d.VerifySnapshots(context.Background(), gameID, 0)
	if err != nil {
		return fail(asJSON, err)
	}
	if asJSON {
		if code := emitJSON(report); code != 0 {
			return code
		}
		if len(report.Damaged) > 0 {
			return 1
		}
		return 0
	}
	if len(report.Damaged) == 0 {
		fmt.Printf("%s %d snapshot(s) checked — every one can be restored\n", accent("ok"), report.Checked)
		return 0
	}
	section(fmt.Sprintf("%d of %d snapshot(s) cannot be restored", len(report.Damaged), report.Checked))
	for _, s := range report.Damaged {
		fmt.Printf("  %s %s  %s\n      %s\n", symBullet(), bold(s.GameName), faint(s.SnapshotID), dangerText(s.Problem))
	}
	hint("opensave verify --repair          (put back from the cloud those that have a copy there)",
		"opensave verify --remove-damaged  (remove from the history those that cannot be restored)")
	return 1
}

func cmdVerifyRepair(d *daemon.Daemon, asJSON bool) int {
	report, err := d.RepairSnapshots(context.Background())
	if err != nil {
		return fail(asJSON, err)
	}
	if asJSON {
		return emitJSON(report)
	}
	for _, r := range report.Repaired {
		fmt.Printf("%s %s  %s  put back from its cloud copy\n", accent("ok"), bold(r.GameName), faint(r.SnapshotID))
	}
	switch {
	case len(report.Repaired) == 0 && len(report.Remaining) == 0:
		fmt.Printf("%s nothing to repair: every snapshot checked can be restored\n", accent("ok"))
		return 0
	case !report.CloudChecked:
		note("the cloud could not be looked at: " + report.CloudError)
	}
	if len(report.Remaining) > 0 {
		section(fmt.Sprintf("%d snapshot(s) have no whole copy anywhere", len(report.Remaining)))
		for _, r := range report.Remaining {
			fmt.Printf("  %s %s  %s\n", symBullet(), bold(r.GameName), faint(r.SnapshotID))
		}
		hint("opensave verify --remove-damaged  (remove them from the history)")
		return 1
	}
	return 0
}

func cmdVerifyRemove(d *daemon.Daemon, asJSON bool) int {
	removed, err := d.ForgetDamagedSnapshots()
	if err != nil {
		return fail(asJSON, err)
	}
	if asJSON {
		return emitJSON(map[string]any{"removed": removed})
	}
	if len(removed) == 0 {
		fmt.Printf("%s nothing to remove: no snapshot is known to be damaged\n", accent("ok"))
		return 0
	}
	fmt.Printf("%s removed %d snapshot(s) that could not be restored; the rest of each game's history is unchanged\n", accent("ok"), len(removed))
	return 0
}
