package cliapp

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// What a sync did, said plainly.
//
// The daemon answers a sync with what happened with each device, or — for a
// game it did not sync — why: "paused" (syncing is paused here), "held" (the
// save was emptied here and waits for an answer), "offline" (no other device
// answered). This used to print "Sync started" whatever came back, including
// when nothing could have synced at all.

// syncOutcome is how one game's sync went.
type syncOutcome struct {
	Game string
	// Kind is changed | in-sync | conflict | waiting | queued | skipped |
	// paused | held | offline | error.
	Kind string
	// Detail is the error, the device a conflict is with, or what a device
	// is waiting on.
	Detail string
}

type peerSyncResult struct {
	Status   string `json:"status"`
	PeerName string `json:"peerName"`
}

// syncOutcomeOf reads one game's entry in a sync-all answer: either what
// happened with each device, or a status saying it did not sync and why.
func syncOutcomeOf(gameID string, raw json.RawMessage) syncOutcome {
	var flat struct {
		Status string `json:"status"`
		Error  string `json:"error"`
		Reason string `json:"reason"`
	}
	if json.Unmarshal(raw, &flat) == nil {
		switch flat.Status {
		case "error":
			kind := flat.Reason
			if kind == "" || kind == "error" {
				kind = reasonFromMessage(flat.Error)
			}
			return syncOutcome{Game: gameID, Kind: kind, Detail: flat.Error}
		case "queued":
			return syncOutcome{Game: gameID, Kind: "queued"}
		case "skipped":
			return syncOutcome{Game: gameID, Kind: "skipped", Detail: flat.Reason}
		}
	}
	var peers map[string]peerSyncResult
	_ = json.Unmarshal(raw, &peers)
	return outcomeFromPeers(gameID, peers)
}

// reasonFromMessage is the reason for a daemon from before sync answers had
// one, from the words it used.
func reasonFromMessage(msg string) string {
	m := strings.ToLower(msg)
	switch {
	case strings.Contains(m, "paused"):
		return "paused"
	case strings.Contains(m, "no online peers"):
		return "offline"
	case strings.Contains(m, "deleted here"):
		return "held"
	default:
		return "error"
	}
}

// outcomeFromPeers is one game's outcome from what happened with each device.
func outcomeFromPeers(gameID string, peers map[string]peerSyncResult) syncOutcome {
	ids := make([]string, 0, len(peers))
	for id := range peers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	changed, waiting := false, ""
	for _, id := range ids {
		p := peers[id]
		who := p.PeerName
		if who == "" {
			who = "the other device"
		}
		switch p.Status {
		case "conflict":
			return syncOutcome{Game: gameID, Kind: "conflict", Detail: who}
		case "updated", "updated_bidirectional", "deletions_synced", "triggered_peer_pull":
			changed = true
		case "peer_awaiting_folder":
			waiting = who + " is waiting for a folder to be chosen for it"
		case "peer_holding":
			waiting = who + " is holding it back: its save was emptied there"
		case "peer_missing":
			if waiting == "" {
				waiting = who + " does not track it"
			}
		}
	}
	switch {
	case changed:
		return syncOutcome{Game: gameID, Kind: "changed"}
	case waiting != "":
		return syncOutcome{Game: gameID, Kind: "waiting", Detail: waiting}
	default:
		return syncOutcome{Game: gameID, Kind: "in-sync"}
	}
}

// syncSummary is a sync-all's outcomes, counted.
type syncSummary struct {
	Total, Changed, InSync, Queued, Skipped, Paused, Offline int
	Conflicts, Held, Waiting, Failed                         []syncOutcome
}

func summarizeSync(outcomes []syncOutcome) syncSummary {
	s := syncSummary{Total: len(outcomes)}
	for _, o := range outcomes {
		switch o.Kind {
		case "changed":
			s.Changed++
		case "in-sync":
			s.InSync++
		case "queued":
			s.Queued++
		case "skipped":
			s.Skipped++
		case "paused":
			s.Paused++
		case "offline":
			s.Offline++
		case "conflict":
			s.Conflicts = append(s.Conflicts, o)
		case "held":
			s.Held = append(s.Held, o)
		case "waiting":
			s.Waiting = append(s.Waiting, o)
		default:
			s.Failed = append(s.Failed, o)
		}
	}
	return s
}

// nothingSynced says why none of it synced — "paused" or "offline" — or ""
// when something did, or could have.
func (s syncSummary) nothingSynced() string {
	switch {
	case s.Total == 0:
		return ""
	case s.Paused == s.Total-s.Skipped && s.Paused > 0:
		return "paused"
	case s.Offline+s.Paused == s.Total-s.Skipped && s.Offline > 0:
		return "offline"
	}
	return ""
}

// headline is the one line a sync-all is summed up in.
func (s syncSummary) headline() string {
	synced := s.Changed + s.InSync
	parts := []string{}
	if s.Changed > 0 {
		parts = append(parts, plural(s.Changed, "updated", "updated"))
	}
	if s.InSync > 0 {
		parts = append(parts, plural(s.InSync, "already in sync", "already in sync"))
	}
	if s.Queued > 0 {
		parts = append(parts, plural(s.Queued, "queued behind a sync already running", "queued behind syncs already running"))
	}
	head := "Synced " + plural(synced, "game", "games")
	if synced == 0 && s.Queued > 0 {
		head = "Nothing new synced"
	}
	if len(parts) == 0 {
		return head + "."
	}
	return head + ": " + strings.Join(parts, ", ") + "."
}

// plural is a count with its noun: "1 game", "3 games".
func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}
