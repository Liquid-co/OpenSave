package cliapp

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
)

const gamedbUsage = `usage: opensave gamedb <gameId> link       link a game tracked without an App ID to the game database's match
       opensave gamedb <gameId> dismiss    not that one — it is not suggested again for this folder`

// cmdGameDB answers the game database's match for a game tracked without an
// App ID: linked, the game gets cover art, launching, being seen played and
// the database's settings files kept on each device. Never linked without
// this answer, here or in the game's Config tab; `opensave status` lists it.
func cmdGameDB(args []string) int {
	asJSON, args := jsonFlag(args)
	if len(args) != 2 || (args[1] != "link" && args[1] != "dismiss") {
		fmt.Fprintln(os.Stderr, gamedbUsage)
		return 1
	}
	gameID := args[0]
	match, ok := runningDaemonGameDBSuggestions()[gameID]
	if !ok {
		return fail(asJSON, fmt.Errorf("no game database link is suggested for %s (OpenSave must be running)", gameID))
	}
	link := args[1] == "link"
	raw, err := daemonRequest("POST", "/api/games/"+url.PathEscape(gameID)+"/gamedb-link", map[string]bool{"accept": link})
	if err != nil {
		return fail(asJSON, err)
	}
	if asJSON {
		return emitRawJSON(raw)
	}
	if link {
		success("%s is linked to %s.", bold(gameID), match)
	} else {
		success("%s is not linked to %s.", bold(gameID), match)
	}
	return 0
}

// runningDaemonGameDBSuggestions is the game database's match, by name, for
// each game waiting for an answer.
func runningDaemonGameDBSuggestions() map[string]string {
	out := map[string]string{}
	raw, err := daemonRequest("GET", "/api/games", nil)
	if err != nil {
		return out
	}
	var games map[string]struct {
		GameDBSuggestion *struct {
			Name string `json:"name"`
		} `json:"gameDBSuggestion"`
	}
	if json.Unmarshal(raw, &games) != nil {
		return out
	}
	for id, g := range games {
		if g.GameDBSuggestion != nil {
			out[id] = g.GameDBSuggestion.Name
		}
	}
	return out
}
