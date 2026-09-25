package cliapp

import (
	"context"
	"fmt"

	"github.com/opensave/opensave/internal/daemon"
)

// cmdVerify reads every snapshot back — of one game, or of all of them — and
// says whether each can still be restored. It exits 1 when one cannot, so a
// script can act on it.
func cmdVerify(d *daemon.Daemon, args []string) int {
	asJSON, args := jsonFlag(args)
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
	hint("opensave snapshot-delete <gameId> <snapId>   (removes one that cannot be used)")
	return 1
}
