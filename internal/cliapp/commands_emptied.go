package cliapp

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/opensave/opensave/internal/daemon"
)

// cmdEmptied lists the games whose save files were all deleted on this
// device, held back from the others until someone says whether that was
// meant (p2p/syncengine/hold.go), and answers for one:
//
//	opensave emptied                     what is held back
//	opensave emptied <game> delete       delete them on the other devices too
//	opensave emptied <game> restore      put them back here
func cmdEmptied(d *daemon.Daemon, args []string) int {
	asJSON, args := jsonFlag(args)
	if len(args) == 0 {
		list, err := d.EmptiedSaves()
		if err != nil {
			return fail(asJSON, err)
		}
		if asJSON {
			return emitJSON(list)
		}
		if len(list) == 0 {
			fmt.Printf("%s no save is held back: nothing was emptied on this device\n", accent("ok"))
			return 0
		}
		section(fmt.Sprintf("%d emptied save(s) held back from your other devices", len(list)))
		for _, e := range list {
			when := time.UnixMilli(e.SinceMs).UTC().Format(time.RFC3339)
			state := fmt.Sprintf("every file deleted here %s; %d on your other devices would go too", timeAgo(when, time.Now()), e.Files)
			if e.State == "fetching" {
				state = "putting the files back — still fetching some from your other devices"
			}
			fmt.Printf("  %s %s  %s\n      %s\n", symBullet(), bold(e.Name), faint(e.GameID), state)
			if len(e.Locations) > 0 && (len(e.Locations) > 1 || e.Locations[0] != "") {
				fmt.Printf("      %s\n", faint("in "+describeEmptied(e.Locations)))
			}
		}
		hint("opensave emptied <gameId> delete    (delete them on your other devices too)",
			"opensave emptied <gameId> restore   (put them back here)")
		return 0
	}
	if len(args) != 2 {
		return fail(asJSON, errors.New("usage: opensave emptied [<gameId> delete|restore]"))
	}
	game, err := d.Store.GetGame(args[0])
	if err != nil {
		return fail(asJSON, unknownGameError(d, args[0], err))
	}
	res, err := d.AnswerEmptied(game.ID, strings.ToLower(args[1]))
	if err != nil {
		return fail(asJSON, err)
	}
	if asJSON {
		return emitJSON(res)
	}
	switch {
	case args[1] == daemon.EmptiedDelete:
		fmt.Printf("%s the deletion of %s's save files goes to your other devices at the next sync; each keeps a snapshot first\n", accent("ok"), bold(game.Name))
	case res.Restored != "" && res.Fetching > 0:
		fmt.Printf("%s %s's files are back from a snapshot; %d more come from your other devices at the next sync\n", accent("ok"), bold(game.Name), res.Fetching)
	case res.Restored != "":
		fmt.Printf("%s %s's files are back from a snapshot\n", accent("ok"), bold(game.Name))
	default:
		fmt.Printf("%s %s's files come back from your other devices at the next sync\n", accent("ok"), bold(game.Name))
	}
	return 0
}

// describeEmptied names the emptied locations: "" is the main save folder.
func describeEmptied(names []string) string {
	parts := make([]string, 0, len(names))
	for _, n := range names {
		if n == "" {
			parts = append(parts, "the save folder")
		} else {
			parts = append(parts, fmt.Sprintf("the %q location", n))
		}
	}
	return strings.Join(parts, " and ")
}
