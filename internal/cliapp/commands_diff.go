package cliapp

import (
	"fmt"
	"os"

	"github.com/opensave/opensave/internal/daemon"
	"github.com/opensave/opensave/internal/snapshot"
)

// cmdSnapshotDiff says what changed from one snapshot of a game to another.
func cmdSnapshotDiff(d *daemon.Daemon, args []string) int {
	asJSON, args := jsonFlag(args)
	if len(args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: opensave snapshot-diff <gameId> <fromSnapId> <toSnapId> [--json]")
		return 1
	}
	game, err := d.Store.GetGame(args[0])
	if err != nil {
		return fail(asJSON, unknownGameError(d, args[0], err))
	}
	c, err := d.Snapshots.Compare(game.ID, args[1], args[2])
	if err != nil {
		return fail(asJSON, err)
	}
	if asJSON {
		return emitJSON(c)
	}
	if len(c.Changes) == 0 {
		note(fmt.Sprintf("the same: %d file(s), none different", c.Unchanged))
		return 0
	}
	section(fmt.Sprintf("%d file(s) differ %s %d the same", len(c.Changes), symDot(), c.Unchanged))
	for _, ch := range c.Changes {
		path := ch.Path
		if ch.Location != "" {
			path = ch.Location + "/" + path
		}
		sizes := ""
		switch ch.Change {
		case snapshot.DiffChanged:
			sizes = humanBytes(ch.FromSize) + " → " + humanBytes(ch.ToSize)
		case snapshot.DiffAdded:
			sizes = humanBytes(ch.ToSize)
		case snapshot.DiffRemoved:
			sizes = humanBytes(ch.FromSize)
		}
		fmt.Printf("  %s %s  %s  %s\n", symBullet(), padRight(ch.Change, 8), path, faint(sizes))
	}
	return 0
}
