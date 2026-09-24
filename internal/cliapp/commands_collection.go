package cliapp

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/opensave/opensave/internal/store"
)

const collectionUsage = `usage: opensave collection list
       opensave collection create <name>
       opensave collection rename <collection> <new name>
       opensave collection delete <collection>
       opensave collection add <collection> <gameId>...
       opensave collection remove <collection> <gameId>...

  A collection is named by its id or its name (quote a name with spaces).
  Favourites is always there:  opensave collection add favourites hades`

// cmdCollection manages collections through the running daemon, so the app
// shows a change the moment it is made.
func cmdCollection(args []string) int {
	asJSON, args := jsonFlag(args)
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, collectionUsage)
		return 1
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "list":
		all, err := listCollections()
		if err != nil {
			return fail(asJSON, err)
		}
		if asJSON {
			return emitJSON(all)
		}
		for _, c := range all {
			games := faint("empty")
			if len(c.GameIDs) > 0 {
				games = strings.Join(c.GameIDs, ", ")
			}
			fmt.Printf("  %s %s  %s\n", symBullet(), bold(c.Name), faint(fmt.Sprintf("(%s)", c.ID)))
			fmt.Printf("      %s\n", games)
		}
		return 0

	case "create":
		name := strings.Join(rest, " ")
		if name == "" {
			fmt.Fprintln(os.Stderr, collectionUsage)
			return 1
		}
		raw, err := daemonRequest("POST", "/api/collections", map[string]any{"name": name})
		if err != nil {
			return fail(asJSON, err)
		}
		if asJSON {
			return emitRawJSON(raw)
		}
		var c store.Collection
		_ = json.Unmarshal(raw, &c)
		success("Created %s", accent(c.Name))
		return 0

	case "rename", "delete", "add", "remove":
		if len(rest) < 1 || (sub != "delete" && len(rest) < 2) {
			fmt.Fprintln(os.Stderr, collectionUsage)
			return 1
		}
		c, err := resolveCollection(rest[0])
		if err != nil {
			return fail(asJSON, err)
		}
		path := "/api/collections/" + url.PathEscape(c.ID)
		switch sub {
		case "rename":
			name := strings.Join(rest[1:], " ")
			if _, err := daemonRequest("PATCH", path, map[string]any{"name": name}); err != nil {
				return fail(asJSON, err)
			}
			if !asJSON {
				success("Renamed %s to %s", c.Name, accent(strings.Join(strings.Fields(name), " ")))
			}
		case "delete":
			if _, err := daemonRequest("DELETE", path, nil); err != nil {
				return fail(asJSON, err)
			}
			if !asJSON {
				success("Deleted the collection %s", accent(c.Name))
				note("its games are still tracked")
			}
		default: // add, remove
			in := sub == "add"
			for _, gameID := range rest[1:] {
				if _, err := daemonRequest("POST", path+"/games", map[string]any{"gameId": gameID, "in": in}); err != nil {
					return fail(asJSON, fmt.Errorf("%s: %w", gameID, err))
				}
			}
			if !asJSON {
				verb := "Added to"
				if !in {
					verb = "Removed from"
				}
				success("%s %s: %s", verb, accent(c.Name), strings.Join(rest[1:], ", "))
			}
		}
		if asJSON {
			return emitJSON(map[string]any{"collection": c.ID, "done": sub})
		}
		return 0

	default:
		fmt.Fprintf(os.Stderr, "unknown collection command %q\n\n%s\n", sub, collectionUsage)
		return 1
	}
}

func listCollections() ([]store.Collection, error) {
	raw, err := daemonRequest("GET", "/api/collections", nil)
	if err != nil {
		return nil, err
	}
	var all []store.Collection
	if err := json.Unmarshal(raw, &all); err != nil {
		return nil, fmt.Errorf("unexpected answer from the daemon: %w", err)
	}
	return all, nil
}

// resolveCollection finds a collection by id, or by name ignoring case.
func resolveCollection(idOrName string) (store.Collection, error) {
	all, err := listCollections()
	if err != nil {
		return store.Collection{}, err
	}
	for _, c := range all {
		if c.ID == idOrName {
			return c, nil
		}
	}
	for _, c := range all {
		if strings.EqualFold(c.Name, strings.TrimSpace(idOrName)) {
			return c, nil
		}
	}
	return store.Collection{}, fmt.Errorf("no collection %q — see `opensave collection list`", idOrName)
}
