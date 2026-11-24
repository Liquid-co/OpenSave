package cliapp

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const settingsUsage = `usage: opensave settings <gameId> keep <file>   keep a suggested settings file on this device, out of syncing
       opensave settings <gameId> sync <file>   keep syncing it — it is not suggested again`

// cmdSettings answers a file the daemon found that looks like this device's
// settings (graphics, resolution) rather than part of the save. It keeps
// syncing until answered, here or in the game's Config tab; `opensave status`
// lists the files waiting.
func cmdSettings(args []string) int {
	asJSON, args := jsonFlag(args)
	if len(args) != 3 || (args[1] != "keep" && args[1] != "sync") {
		fmt.Fprintln(os.Stderr, settingsUsage)
		return 1
	}
	gameID := args[0]
	// As status prints it, or as typed on Windows: the daemon keeps it
	// relative to the save folder, with forward slashes.
	file := strings.TrimPrefix(filepath.ToSlash(args[2]), "/")
	keep := args[1] == "keep"
	raw, err := daemonRequest("POST", "/api/games/"+url.PathEscape(gameID)+"/suggested-settings", map[string]any{"path": file, "keepHere": keep})
	if err != nil {
		return fail(asJSON, err)
	}
	if asJSON {
		return emitRawJSON(raw)
	}
	if keep {
		success("%s is kept on this device now; %s no longer syncs it.", file, bold(gameID))
	} else {
		success("%s keeps syncing for %s.", file, bold(gameID))
	}
	return 0
}
