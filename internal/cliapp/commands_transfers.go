package cliapp

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/opensave/opensave/internal/transfers"
)

// cmdTransfers shows what is moving between this device and others, and what
// moved recently — the running daemon's record, since it is the one syncing.
func cmdTransfers(args []string) int {
	asJSON, _ := jsonFlag(args)
	raw, err := daemonRequest("GET", "/api/transfers", nil)
	if err != nil {
		return fail(asJSON, err)
	}
	if asJSON {
		return emitRawJSON(raw)
	}
	var s transfers.Snapshot
	if err := json.Unmarshal(raw, &s); err != nil {
		return fail(asJSON, fmt.Errorf("unexpected answer from the daemon: %w", err))
	}
	if len(s.Active) == 0 && len(s.Recent) == 0 {
		note("nothing has moved between your devices since the daemon started")
		return 0
	}
	if len(s.Active) > 0 {
		section("Now")
		for _, t := range s.Active {
			line := fmt.Sprintf("%d%%", t.Percentage)
			if t.SpeedBytesPerSec > 0 {
				line += "  " + humanBytes(int64(t.SpeedBytesPerSec)) + "/s"
			}
			fmt.Printf("  %s %s  %s  %s\n", symBullet(), bold(t.GameID), transferWay(t), faint(line))
		}
	}
	if len(s.Recent) > 0 {
		section("Recently")
		for _, t := range s.Recent {
			outcome := accent("done")
			if t.State == "error" {
				outcome = "failed: " + t.Error
			}
			when := ""
			if ended, err := time.Parse(time.RFC3339, t.EndedAt); err == nil {
				when = timeAgo(ended.Format("2006-01-02T15:04:05.000Z"), time.Now())
			}
			size := ""
			if t.TotalBytes > 0 {
				size = humanBytes(t.TotalBytes) + "  "
			}
			fmt.Printf("  %s %s  %s  %s%s  %s\n", symBullet(), bold(t.GameID), transferWay(t), faint(size), outcome, faint(when))
		}
	}
	return 0
}

// transferWay says which way a transfer went: "from Deck" or "to Deck".
func transferWay(t transfers.Transfer) string {
	peer := t.Peer
	if peer == "" {
		peer = "another device"
	}
	if t.Direction == "upload" {
		return "to " + peer
	}
	return "from " + peer
}
