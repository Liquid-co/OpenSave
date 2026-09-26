package cliapp

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Games a paired device syncs that this one was set to ask about rather than
// track at a guessed folder (`config set unknown-game-from-peer ask`). The
// app lists them with a folder picker; without this, a device driven from a
// terminal could switch the setting on and then never place one — every game
// offered to it stayed unsynced for good.

type offeredGame struct {
	GameID   string `json:"gameId"`
	PeerID   string `json:"peerId"`
	Name     string `json:"name"`
	PeerPath string `json:"peerPath"`
}

const offersUsage = `usage: opensave offers                          games another device syncs, waiting for a folder here
       opensave offers place <gameId> <folder>  track it here, in that folder
       opensave offers decline <gameId>         don't — it is not offered again`

func cmdOffers(args []string) int {
	asJSON, args := jsonFlag(args)
	if len(args) == 0 || args[0] == "list" {
		return listOffers(asJSON)
	}
	switch args[0] {
	case "place":
		if len(args) < 3 {
			fmt.Fprintln(os.Stderr, offersUsage)
			return 1
		}
		folder, err := filepath.Abs(args[2])
		if err != nil {
			return fail(asJSON, err)
		}
		raw, err := daemonRequest("POST", "/api/offered-games/"+url.PathEscape(args[1])+"/place", map[string]string{"path": folder})
		if err != nil {
			return fail(asJSON, err)
		}
		if asJSON {
			return emitRawJSON(raw)
		}
		var game struct {
			Name string `json:"name"`
		}
		_ = json.Unmarshal(raw, &game)
		success("Now tracking %s at %s — it syncs from here on.", bold(orNone(game.Name)), folder)
		return 0
	case "decline":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, offersUsage)
			return 1
		}
		raw, err := daemonRequest("POST", "/api/offered-games/"+url.PathEscape(args[1])+"/decline", map[string]any{})
		if err != nil {
			return fail(asJSON, err)
		}
		if asJSON {
			return emitRawJSON(raw)
		}
		success("Declined %s. It is not offered again unless you track it yourself.", bold(args[1]))
		return 0
	default:
		return fail(asJSON, fmt.Errorf("unknown offers command %q\n\n%s", args[0], offersUsage))
	}
}

func listOffers(asJSON bool) int {
	raw, err := daemonRequest("GET", "/api/offered-games", nil)
	if err != nil {
		return fail(asJSON, err)
	}
	if asJSON {
		return emitRawJSON(raw)
	}
	var offers []offeredGame
	if err := json.Unmarshal(raw, &offers); err != nil {
		return emitRawJSON(raw)
	}
	if len(offers) == 0 {
		section("Offered games")
		fmt.Printf("  %s Nothing waiting for a folder.\n", symOK())
		if !askingBeforeTracking() {
			note("Games from your other devices are tracked here by themselves; to be asked instead:")
			hint("opensave config set unknown-game-from-peer ask")
		}
		fmt.Println()
		return 0
	}

	names := map[string]string{}
	if raw, err := daemonRequest("GET", "/api/peers", nil); err == nil {
		if p, err := decodePeersPayload(raw); err == nil {
			for id, row := range p.Peers {
				names[id] = row.Name
			}
		}
	}
	section(fmt.Sprintf("Offered games %s %d waiting for a folder", symDot(), len(offers)))
	t := newTable("game", "name", "from", "its folder there")
	for _, o := range offers {
		from := names[o.PeerID]
		if from == "" {
			from = faint("a paired device")
		}
		t.add(bold(o.GameID), o.Name, from, faint(o.PeerPath))
	}
	t.render()
	hint(
		"opensave offers place <game> <folder>   track it here, in that folder",
		"opensave offers decline <game>          don't",
	)
	fmt.Println()
	return 0
}

// askingBeforeTracking reports whether this device asks where to keep a game
// another device syncs; not knowing reads as the default, which does not.
func askingBeforeTracking() bool {
	raw, err := daemonRequest("GET", "/api/settings", nil)
	if err != nil {
		return false
	}
	var s struct {
		UnknownGameFromPeer string `json:"unknownGameFromPeer"`
	}
	_ = json.Unmarshal(raw, &s)
	return strings.EqualFold(strings.TrimSpace(s.UnknownGameFromPeer), "ask")
}
