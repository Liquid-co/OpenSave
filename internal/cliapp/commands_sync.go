package cliapp

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
)

// These are the commands that make the CLI a real headless client rather than
// a local snapshot utility: without them a server or a Steam Deck in Game Mode
// can track games but never pair with anything or sync.

// cmdSync triggers a sync for one game or all of them.
func cmdSync(args []string) int {
	asJSON, args := jsonFlag(args)

	if len(args) == 0 || args[0] == "--all" {
		raw, err := daemonRequest("POST", "/api/games/sync-all", map[string]any{})
		if err != nil {
			return fail(asJSON, err)
		}
		if asJSON {
			return emitRawJSON(raw)
		}
		return reportSyncAll(raw)
	}

	gameID := args[0]
	raw, err := daemonRequest("POST", "/api/games/"+gameID+"/sync", map[string]any{})
	if err != nil {
		var refused *daemonError
		if !asJSON && errors.As(err, &refused) {
			if said := sayWhyNotSynced(gameID, refused); said {
				return 1
			}
		}
		return fail(asJSON, err)
	}
	if asJSON {
		return emitRawJSON(raw)
	}
	// A sync that lands while another is running is queued, not an error.
	var res struct {
		Queued  bool                      `json:"queued"`
		Results map[string]peerSyncResult `json:"results"`
	}
	if json.Unmarshal(raw, &res) == nil && res.Queued {
		success("Queued %s behind the sync already running.", bold(gameID))
		return 0
	}
	o := outcomeFromPeers(gameID, res.Results)
	switch o.Kind {
	case "changed":
		success("Synced %s.", bold(gameID))
	case "conflict":
		warning("%s changed here and on %s at once — choose which to keep.", bold(gameID), o.Detail)
		hint("opensave conflicts")
		return 1
	case "waiting":
		success("Synced %s with the devices that could take it.", bold(gameID))
		note(o.Detail)
	default:
		success("%s is already in sync.", bold(gameID))
	}
	return 0
}

// sayWhyNotSynced explains a sync the daemon refused for a reason that is
// not a failure — paused, held, nobody online — and reports whether it did.
func sayWhyNotSynced(gameID string, refused *daemonError) bool {
	reason := refused.Reason
	if reason == "" {
		reason = reasonFromMessage(refused.Message)
	}
	switch reason {
	case "paused":
		warning("Syncing is paused, so %s was not synced.", bold(gameID))
		hint("opensave resume")
	case "offline":
		warning("No other device is online, so %s was not synced. It goes over when one is.", bold(gameID))
	case "held":
		warning("%s is held back: every save file of it was deleted here.", bold(gameID))
		note("Your other devices keep theirs until you say whether that was meant.")
		hint("opensave emptied")
	default:
		return false
	}
	return true
}

// reportSyncAll says what a sync of everything did, game by game where it
// matters, and exits non-zero when nothing could sync or something failed.
func reportSyncAll(raw []byte) int {
	var body struct {
		Results map[string]json.RawMessage `json:"results"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return emitRawJSON(raw) // shape changed; show what came back
	}
	ids := make([]string, 0, len(body.Results))
	for id := range body.Results {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	outcomes := make([]syncOutcome, 0, len(ids))
	for _, id := range ids {
		outcomes = append(outcomes, syncOutcomeOf(id, body.Results[id]))
	}
	s := summarizeSync(outcomes)

	switch s.nothingSynced() {
	case "paused":
		warning("Syncing is paused, so nothing was synced.")
		hint("opensave resume")
		return 1
	case "offline":
		warning("No other device is online, so nothing was synced. Saves go over when one is.")
		return 1
	}
	if s.Total == 0 {
		note("Nothing is tracked yet.")
		hint("opensave scan")
		return 0
	}

	success("%s", s.headline())
	for _, o := range s.Conflicts {
		fmt.Printf("  %s %s changed here and on %s at once — needs a decision\n", symBullet(), bold(o.Game), o.Detail)
	}
	for _, o := range s.Held {
		fmt.Printf("  %s %s is held back: every save file of it was deleted here\n", symBullet(), bold(o.Game))
	}
	for _, o := range s.Waiting {
		fmt.Printf("  %s %s: %s\n", symBullet(), bold(o.Game), o.Detail)
	}
	for _, o := range s.Failed {
		fmt.Printf("  %s %s: %s\n", symFail(), bold(o.Game), o.Detail)
	}
	if s.Offline > 0 {
		note(fmt.Sprintf("%s not synced: no other device answered.", plural(s.Offline, "game", "games")))
	}
	if s.Paused > 0 {
		note(fmt.Sprintf("%s not synced: syncing is paused.", plural(s.Paused, "game", "games")))
	}
	switch {
	case len(s.Conflicts) > 0 && len(s.Held) > 0:
		hint("opensave conflicts", "opensave emptied")
	case len(s.Conflicts) > 0:
		hint("opensave conflicts")
	case len(s.Held) > 0:
		hint("opensave emptied")
	}
	if len(s.Failed) > 0 {
		return 1
	}
	return 0
}

// conflictSideLabel says which side a difference is on, naming the peer
// rather than saying "remote" — the user knows their devices by name.
func conflictSideLabel(status, peerName string) string {
	switch status {
	case "only-local":
		return "only here"
	case "only-remote":
		return "only " + peerName
	default:
		return "differs"
	}
}

// conflictSizeNote renders the two sizes when both exist, and one when the
// file is on a single side. Directories carry -1 on both and get nothing:
// "0 B" beside a folder name reads as an empty file.
func conflictSizeNote(d conflictDiffFile) string {
	switch {
	case d.LocalSize >= 0 && d.RemoteSize >= 0:
		return fmt.Sprintf("(%s / %s)", humanBytes(d.LocalSize), humanBytes(d.RemoteSize))
	case d.LocalSize >= 0:
		return "(" + humanBytes(d.LocalSize) + ")"
	case d.RemoteSize >= 0:
		return "(" + humanBytes(d.RemoteSize) + ")"
	default:
		return ""
	}
}

// cmdConflicts lists saves that diverged on two devices and are waiting on a
// decision. Until one is made that game stops syncing, so a headless machine
// needs to see and settle them without a desktop.
//
// A game's extra save folders can diverge on their own, and are listed after
// the games: `--locations --json` gives those alone, since `--json` on its own
// has always been the games keyed by id.
func cmdConflicts(args []string) int {
	asJSON, args := jsonFlag(args)
	locationsOnly := false
	for _, a := range args {
		if a == "--locations" {
			locationsOnly = true
		}
	}

	conflicts, err := fetchConflicts()
	if err != nil {
		return fail(asJSON, err)
	}
	locations, err := fetchLocationConflicts()
	if err != nil {
		return fail(asJSON, err)
	}
	if asJSON {
		if locationsOnly {
			return emitJSON(locations)
		}
		return emitJSON(conflicts)
	}
	if len(conflicts) == 0 && len(locations) == 0 {
		section("Conflicts")
		fmt.Printf("  %s Everything is in sync.\n", symOK())
		fmt.Println()
		return 0
	}
	if len(conflicts) == 0 {
		printLocationConflicts(locations)
		fmt.Println()
		return 0
	}

	section(fmt.Sprintf("Conflicts %s %d waiting on a decision", symDot(), len(conflicts)))
	t := newTable("game", "diverged from", "newer side", "files")
	for gameID, c := range conflicts {
		var newer string
		switch {
		case c.LocalStats.LatestMtimeMs > c.RemoteStats.LatestMtimeMs:
			newer = accent("this device")
		case c.RemoteStats.LatestMtimeMs > c.LocalStats.LatestMtimeMs:
			newer = accent(c.Peer.Name)
		default:
			newer = faint("same age")
		}
		files := faint("—")
		if c.DiffTotal > 0 {
			files = fmt.Sprintf("%d", c.DiffTotal)
		}
		t.add(bold(gameID), c.Peer.Name, newer, files)
	}
	t.render()

	// Then what differs, per game. Capped: a conflict over a save with
	// hundreds of files should not bury the resolve hints below it, and the
	// count in the table above is already the honest total.
	const maxShown = 12
	for gameID, c := range conflicts {
		if len(c.DiffFiles) == 0 {
			continue
		}
		fmt.Printf("\n  %s %s\n", symBullet(), bold(gameID))

		// Width from the labels actually being printed: "only <device>" is as
		// long as the device is named, so a fixed column either wastes space
		// or fails to align the moment someone's PC is called something.
		labelWidth := 0
		for i, d := range c.DiffFiles {
			if i == maxShown {
				break
			}
			if n := len(conflictSideLabel(d.Status, c.Peer.Name)); n > labelWidth {
				labelWidth = n
			}
		}

		for i, d := range c.DiffFiles {
			if i == maxShown {
				fmt.Printf("      %s\n", faint(fmt.Sprintf("…and %d more", len(c.DiffFiles)-maxShown)))
				break
			}
			fmt.Printf("      %s  %s %s\n",
				faint(padRight(conflictSideLabel(d.Status, c.Peer.Name), labelWidth)),
				d.Path,
				faint(conflictSizeNote(d)))
		}
	}

	hint(
		"opensave resolve <game> keep-both      keeps both, theirs on a branch (safest)",
		"opensave resolve <game> keep-local     this device's save wins",
		"opensave resolve <game> keep-remote    the other device's save wins",
	)
	if len(locations) > 0 {
		printLocationConflicts(locations)
	}
	fmt.Println()
	return 0
}

// printLocationConflicts lists a game's extra save folders that diverged.
// There is no keeping both for one of those: that parks the other device's
// copy on a branch, and branches belong to the whole game.
func printLocationConflicts(list []locationConflictInfo) {
	section(fmt.Sprintf("Save locations %s %d waiting on a decision", symDot(), len(list)))
	t := newTable("game", "folder", "diverged from", "newer side", "files")
	for _, c := range list {
		newer := faint("same age")
		switch {
		case c.LocalStats.LatestMtimeMs > c.RemoteStats.LatestMtimeMs:
			newer = accent("this device")
		case c.RemoteStats.LatestMtimeMs > c.LocalStats.LatestMtimeMs:
			newer = accent(c.Peer.Name)
		}
		files := faint("—")
		if c.DiffTotal > 0 {
			files = fmt.Sprintf("%d", c.DiffTotal)
		}
		t.add(bold(c.GameID), c.Root, c.Peer.Name, newer, files)
	}
	t.render()
	hint(
		"opensave resolve <game> keep-local --location <folder>    this device's copy of that folder wins",
		"opensave resolve <game> keep-remote --location <folder>   the other device's copy wins",
	)
}

// cmdResolve settles one conflict. The peer id is looked up from the conflict
// itself, so the user never has to find and type a node id.
//
// With --location <folder>, it settles one of the game's extra save folders
// instead; and a game whose only conflict is in one such folder needs no
// --location at all.
func cmdResolve(args []string) int {
	asJSON, args := jsonFlag(args)
	location, args, err := locationFlag(args)
	if err != nil {
		return fail(asJSON, fmt.Errorf("%v\n\n%s", err, resolveUsage))
	}
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, resolveUsage)
		return 1
	}
	gameID, choice := args[0], args[1]
	if location != "" {
		return resolveLocation(asJSON, gameID, choice, location)
	}

	// "keep-both" is what the app calls this; merge-branch is the wire name.
	resolution := choice
	if choice == "keep-both" {
		resolution = "merge-branch"
	}
	switch resolution {
	case "keep-local", "keep-remote", "merge-branch":
	default:
		return fail(asJSON, fmt.Errorf("unknown resolution %q\n\n%s", choice, resolveUsage))
	}

	conflicts, err := fetchConflicts()
	if err != nil {
		return fail(asJSON, err)
	}
	c, ok := conflicts[gameID]
	if !ok {
		// Not the whole game: one of its extra save folders, when that is
		// the only thing it could mean.
		locations, lErr := fetchLocationConflicts()
		if lErr != nil {
			return fail(asJSON, lErr)
		}
		var roots []string
		for _, l := range locations {
			if l.GameID == gameID {
				roots = append(roots, l.Root)
			}
		}
		switch len(roots) {
		case 0:
			return fail(asJSON, fmt.Errorf("no active conflict for %q — run `opensave conflicts`", gameID))
		case 1:
			return resolveLocation(asJSON, gameID, choice, roots[0])
		default:
			return fail(asJSON, fmt.Errorf("%q has %d save locations in conflict (%s) — say which with --location <folder>",
				gameID, len(roots), strings.Join(roots, ", ")))
		}
	}

	if _, err := daemonRequest("POST", "/api/games/"+gameID+"/resolve-conflict", map[string]any{
		"peerId":     c.Peer.ID,
		"resolution": resolution,
	}); err != nil {
		return fail(asJSON, err)
	}

	if asJSON {
		return emitJSON(map[string]any{"game": gameID, "resolution": resolution, "applying": true})
	}
	// Applying can pull the peer's whole save, so the daemon does it in the
	// background; the request only confirms it was accepted.
	success("Resolving %s (%s).", bold(gameID), accent(choice))
	note("This runs in the background — a large save can take a while.")
	hint("opensave conflicts")
	return 0
}

const resolveUsage = `usage: opensave resolve <gameId> keep-both|keep-local|keep-remote
       opensave resolve <gameId> keep-local|keep-remote --location <folder>

  keep-both     Keep both saves; the peer's lands on a separate branch (safest)
  keep-local    This device's save wins
  keep-remote   The other device's save wins
  --location    Settle one of the game's extra save folders (see opensave conflicts)`

// locationFlag takes --location <folder> (or --location=<folder>) out of args.
func locationFlag(args []string) (string, []string, error) {
	out := args[:0:0]
	location := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--location":
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("--location needs the folder's name")
			}
			location = args[i+1]
			i++
		case strings.HasPrefix(a, "--location="):
			location = strings.TrimPrefix(a, "--location=")
		default:
			out = append(out, a)
		}
	}
	return location, out, nil
}

// resolveLocation settles a divergence in one of a game's extra save folders.
func resolveLocation(asJSON bool, gameID, choice, root string) int {
	if choice == "keep-both" || choice == "merge-branch" {
		return fail(asJSON, fmt.Errorf("a save location has no keep-both — that parks the other copy on a branch, and branches belong to the whole game; use keep-local or keep-remote"))
	}
	if choice != "keep-local" && choice != "keep-remote" {
		return fail(asJSON, fmt.Errorf("unknown resolution %q\n\n%s", choice, resolveUsage))
	}
	locations, err := fetchLocationConflicts()
	if err != nil {
		return fail(asJSON, err)
	}
	var match *locationConflictInfo
	for i, l := range locations {
		if l.GameID == gameID && strings.EqualFold(l.Root, root) {
			match = &locations[i]
			break
		}
	}
	if match == nil {
		return fail(asJSON, fmt.Errorf("no conflict in %q's %q location — run `opensave conflicts`", gameID, root))
	}
	if _, err := daemonRequest("POST", "/api/games/"+gameID+"/resolve-location-conflict", map[string]any{
		"peerId":     match.Peer.ID,
		"root":       match.Root,
		"resolution": choice,
	}); err != nil {
		return fail(asJSON, err)
	}
	if asJSON {
		return emitJSON(map[string]any{"game": gameID, "location": match.Root, "resolution": choice, "applying": true})
	}
	success("Resolving %s's %s folder (%s).", bold(gameID), bold(match.Root), accent(choice))
	note("This runs in the background — a large folder can take a while.")
	hint("opensave conflicts")
	return 0
}

// locationConflictInfo mirrors /api/status's locationConflicts.
type locationConflictInfo struct {
	GameID string `json:"gameId"`
	Root   string `json:"root"`
	Peer   struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"peer"`
	LocalStats struct {
		LatestMtimeMs int64 `json:"latestMtimeMs"`
	} `json:"localStats"`
	RemoteStats struct {
		LatestMtimeMs int64 `json:"latestMtimeMs"`
	} `json:"remoteStats"`
	DiffTotal int `json:"diffTotal"`
}

// fetchLocationConflicts lists diverged save locations; a daemon from before
// status carried them reports none.
func fetchLocationConflicts() ([]locationConflictInfo, error) {
	raw, err := daemonRequest("GET", "/api/status", nil)
	if err != nil {
		return nil, err
	}
	var st struct {
		LocationConflicts []locationConflictInfo `json:"locationConflicts"`
	}
	if err := json.Unmarshal(raw, &st); err != nil {
		return nil, err
	}
	if st.LocationConflicts == nil {
		st.LocationConflicts = []locationConflictInfo{}
	}
	return st.LocationConflicts, nil
}

// conflictInfo mirrors the conflict shape /api/status reports.
type conflictInfo struct {
	Peer struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"peer"`
	LocalStats struct {
		LatestMtimeMs int64 `json:"latestMtimeMs"`
	} `json:"localStats"`
	RemoteStats struct {
		LatestMtimeMs int64 `json:"latestMtimeMs"`
	} `json:"remoteStats"`
	DiffTotal int `json:"diffTotal"`
	// DiffFiles is what actually differs. The count alone tells you a
	// decision is needed but nothing about which way to decide, which is the
	// question in front of the user — the app has shown this list since the
	// conflict dialog was rebuilt; the CLI was decoding the count beside it
	// and dropping the rest of the payload on the floor.
	DiffFiles []conflictDiffFile `json:"diffFiles"`
}

type conflictDiffFile struct {
	Path string `json:"path"`
	// changed | only-local | only-remote
	Status     string `json:"status"`
	LocalSize  int64  `json:"localSize"`
	RemoteSize int64  `json:"remoteSize"`
}

func fetchConflicts() (map[string]conflictInfo, error) {
	raw, err := daemonRequest("GET", "/api/status", nil)
	if err != nil {
		return nil, err
	}
	var st struct {
		Conflicts map[string]conflictInfo `json:"conflicts"`
	}
	if err := json.Unmarshal(raw, &st); err != nil {
		return nil, err
	}
	if st.Conflicts == nil {
		st.Conflicts = map[string]conflictInfo{}
	}
	return st.Conflicts, nil
}

// cmdPeers lists paired devices and their status.
// cmdPeerGames lists what a paired device is tracking.
//
// `opensave link` already accepts a peer's game id — LinkGames records the
// alias and leaves the local library alone when the id isn't one of ours — so
// a cross-device link was possible from here, but only if you already knew an
// id the CLI had no way to show you. The desktop picker listed them; this is
// the same list.
func cmdPeerGames(args []string) int {
	asJSON, args := jsonFlag(args)
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr,
			"usage: opensave peers games <peerId>\n"+
				"  Lists what that device is tracking, so you can link one of its\n"+
				"  entries to a game here with `opensave link`.\n"+
				"  Device ids come from `opensave peers`.")
		return 1
	}
	peerID := args[0]

	raw, err := daemonRequest("GET", "/api/peers/"+url.PathEscape(peerID)+"/games", nil)
	if err != nil {
		return fail(asJSON, err)
	}
	if asJSON {
		return emitRawJSON(raw)
	}

	var games []struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		SavePath string `json:"savePath"`
		AppID    string `json:"appId"`
	}
	if json.Unmarshal(raw, &games) != nil {
		return emitRawJSON(raw)
	}
	if len(games) == 0 {
		section(peerID)
		note("That device isn't tracking anything.")
		fmt.Println()
		return 0
	}

	section(fmt.Sprintf("%s %s %d game(s)", peerID, symDot(), len(games)))
	t := newTable("id", "name", "app id", "path")
	for _, g := range games {
		appID := faint("—")
		if g.AppID != "" {
			appID = g.AppID
		}
		t.add(accent(g.ID), bold(g.Name), appID, faint(g.SavePath))
	}
	t.render()
	hint("opensave link <localGameId> <theirGameId>     treat them as the same game")
	fmt.Println()
	return 0
}

func cmdPeers(args []string) int {
	asJSON, rest := jsonFlag(args)

	// `peers games <id>` asks a device what it holds; bare `peers` lists the
	// devices themselves.
	if len(rest) > 0 && rest[0] == "games" {
		return cmdPeerGames(args[1:])
	}

	raw, err := daemonRequest("GET", "/api/peers", nil)
	if err != nil {
		return fail(asJSON, err)
	}
	if asJSON {
		return emitRawJSON(raw)
	}

	payload, err := decodePeersPayload(raw)
	if err != nil {
		return emitRawJSON(raw) // shape changed; show what we got
	}

	if len(payload.Peers) > 0 {
		section("Paired devices")
		t := newTable("device", "status", "address", "protection", "id")
		repair := []string{}
		for id, p := range payload.Peers {
			var status string
			switch p.Status {
			case "online":
				status = okText(sym("● online", "online"))
			case "offline", "":
				status = faint(sym("● offline", "offline"))
			default:
				status = warnText(p.Status)
			}
			addr := p.Address
			if addr == "relay" {
				addr = "internet relay"
			} else if addr != "" && p.Port != 0 {
				addr = fmt.Sprintf("%s:%d", p.Address, p.Port)
			}
			t.add(bold(p.Name), status, faint(addr), p.protection(), faint(id))
			if p.needsRepair() {
				repair = append(repair, p.Name)
			}
		}
		t.render()
		// The one state a person can do something about, said where they
		// can act on it. Pairing over a relay before 2.4.0 kept no key, so
		// the fix is the same for every such device: unpair and pair again.
		if len(repair) > 0 {
			note("Not encrypted: pairing over the internet on an earlier version kept no key.")
			note("Unpair and pair again to protect: " + strings.Join(repair, ", "))
			hint("opensave unpair <id>", "opensave pair <other-device>")
		}
		note("Local-network pairings are direct and never touch a relay. Fingerprints: opensave peers --json")
	}

	if len(payload.PairingRequests) > 0 {
		section("Waiting for your approval")
		t := newTable("device", "from", "id")
		for _, r := range payload.PairingRequests {
			name := r.DeviceName
			if name == "" {
				name = "(unnamed device)"
			}
			origin := fmt.Sprintf("%s:%d", r.Address, r.Port)
			if r.IsWan {
				origin = "over the relay"
			}
			t.add(bold(name), faint(origin), faint(r.PeerID))
		}
		t.render()
		hint("opensave pair approve <id>")
	}

	// The daemon lists LAN discoveries and relay room members together;
	// they are found differently and paired differently, so they are shown
	// apart. A room member used to appear under "on this network" with no
	// name and a meaningless "relay:<port>" address.
	// A device already paired is not a candidate to pair with; the daemon
	// lists every room member regardless, so the paired ones are dropped
	// here, as the app does.
	lan, room := 0, 0
	for _, d := range payload.DiscoveredPeers {
		if _, paired := payload.Peers[d.ID]; paired {
			continue
		}
		if d.IsWan {
			room++
		} else {
			lan++
		}
	}
	if lan > 0 {
		section("Found on this network")
		t := newTable("device", "address")
		for _, d := range payload.DiscoveredPeers {
			if _, paired := payload.Peers[d.ID]; paired || d.IsWan {
				continue
			}
			addr := d.Address
			if addr != "" && d.Port != 0 {
				addr = fmt.Sprintf("%s:%d", d.Address, d.Port)
			}
			t.add(bold(d.Name), faint(addr))
		}
		t.render()
		hint("opensave pair <address>")
	}
	if room > 0 {
		section("In your relay room")
		t := newTable("device", "id")
		for _, d := range payload.DiscoveredPeers {
			if _, paired := payload.Peers[d.ID]; paired || !d.IsWan {
				continue
			}
			name := d.DeviceName
			if name == "" {
				name = d.Name
			}
			t.add(bold(name), faint(d.ID))
		}
		t.render()
		hint("opensave pair <id>")
	}

	if len(payload.Peers) == 0 && len(payload.DiscoveredPeers) == 0 && len(payload.PairingRequests) == 0 {
		section("Devices")
		note("Nothing paired or discovered yet.")
		hint(
			"opensave pair <address>        same network",
			"opensave relay join <code>     different networks",
		)
	}
	fmt.Println()
	return 0
}

// peersPayload is what /api/peers returns: the whole dashboard peer state,
// not just paired devices.
type peersPayload struct {
	Peers           map[string]peerRow `json:"peers"`
	DiscoveredPeers []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		// A relay room member says deviceName where a LAN peer says name.
		DeviceName string `json:"deviceName"`
		Address    string `json:"address"`
		Port       int    `json:"port"`
		IsWan      bool   `json:"isWan"`
	} `json:"discoveredPeers"`
	PairingRequests []struct {
		PeerID string `json:"peerId"`
		// The daemon sends this as "deviceName"; decoding it as "name" left
		// the column blank on every request, so approving one meant trusting
		// a bare node id with no way to tell which machine was asking.
		DeviceName string `json:"deviceName"`
		Address    string `json:"address"`
		Port       int    `json:"port"`
		IsWan      bool   `json:"isWan"`
	} `json:"pairingRequests"`
}

// peerRow is one paired device as the daemon describes it.
//
// The protection fields are what the app's per-device badge is built from;
// see PeerProtection in internal/p2p/payloadseal.go. They are pointers so a
// daemon from before they existed decodes to nil rather than to "not
// encrypted" — an older daemon has not said the pairing is unprotected, it
// has said nothing, and the two must not print the same.
type peerRow struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Address string `json:"address"`
	Port    int    `json:"port"`

	Encrypted   *bool  `json:"encrypted"`
	HasKey      *bool  `json:"hasKey"`
	OverRelay   *bool  `json:"overRelay"`
	Fingerprint string `json:"fingerprint"`
}

// protection renders the same four states the app shows, by the same rule.
//
// The rule lives in the daemon and in the app's protection.js, and this is
// the third copy; all three must agree or a terminal user and an app user
// looking at the same pairing get different answers. The condition for
// "encrypted" is the one the send path applies — a key exists and the peer
// has proved it holds the matching half — so this can never claim more
// protection than is actually in force.
func (p peerRow) protection() string {
	overRelay := p.Address == "relay"
	if p.OverRelay != nil {
		overRelay = *p.OverRelay
	}
	if !overRelay {
		return faint(sym("🖧 direct", "direct"))
	}
	if p.OverRelay == nil && p.Encrypted == nil {
		// A relay pairing on a daemon that does not report protection at
		// all. Saying "not encrypted" would be a guess dressed as a fact.
		return faint(sym("? unknown", "unknown"))
	}
	if p.Encrypted != nil && *p.Encrypted {
		return okText(sym("🔒 encrypted", "encrypted"))
	}
	if p.HasKey != nil && *p.HasKey {
		return warnText(sym("🔐 encrypting shortly", "encrypting shortly"))
	}
	return warnText(sym("🔓 not encrypted", "not encrypted"))
}

// needsRepair reports whether the only fix for this pairing is to make it
// again — a relay pairing with no key at all.
func (p peerRow) needsRepair() bool {
	overRelay := p.Address == "relay"
	if p.OverRelay != nil {
		overRelay = *p.OverRelay
	}
	return overRelay && p.HasKey != nil && !*p.HasKey
}

func decodePeersPayload(raw []byte) (peersPayload, error) {
	var p peersPayload
	err := json.Unmarshal(raw, &p)
	return p, err
}

// cmdPair handles the pairing lifecycle: request, inspect and approve.
// Pairing is mutual by design, so a headless device needs both halves —
// asking to pair, and approving someone else's request.
func cmdPair(args []string) int {
	asJSON, args := jsonFlag(args)
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, pairUsage)
		return 1
	}

	switch args[0] {
	case "requests":
		raw, err := daemonRequest("GET", "/api/peers", nil)
		if err != nil {
			return fail(asJSON, err)
		}
		payload, err := decodePeersPayload(raw)
		if err != nil {
			return emitRawJSON(raw)
		}
		if asJSON {
			return emitJSON(payload.PairingRequests)
		}
		if len(payload.PairingRequests) == 0 {
			fmt.Println("No pending pairing requests.")
			return 0
		}
		section(fmt.Sprintf("Pairing requests %s %d waiting", symDot(), len(payload.PairingRequests)))
		t := newTable("DEVICE", "FROM", "ID")
		for _, r := range payload.PairingRequests {
			name := r.DeviceName
			if name == "" {
				name = "(unnamed device)"
			}
			origin := fmt.Sprintf("%s:%d", r.Address, r.Port)
			if r.IsWan {
				origin = "over the relay"
			}
			t.add(bold(name), faint(origin), faint(r.PeerID))
		}
		t.render()
		hint("opensave pair approve <id>", "opensave pair reject <id>")
		fmt.Println()
		return 0

	case "approve":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: opensave pair approve <peerId>")
			return 1
		}
		raw, err := daemonRequest("POST", "/api/peers/approve", map[string]any{"peerId": args[1]})
		if err != nil {
			return fail(asJSON, err)
		}
		if asJSON {
			return emitRawJSON(raw)
		}
		success("Paired with %s.", bold(args[1]))
		return 0

	case "reject":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: opensave pair reject <peerId>")
			return 1
		}
		raw, err := daemonRequest("POST", "/api/peers/reject", map[string]any{"peerId": args[1]})
		if err != nil {
			return fail(asJSON, err)
		}
		if asJSON {
			return emitRawJSON(raw)
		}
		success("Rejected %s.", bold(args[1]))
		return 0

	default:
		target := args[0]

		// `opensave pair <node id>` — a device in the relay room. The ids
		// are what `opensave peers` lists under "In your relay room", and
		// the daemon pairs through the relay when given one. Sending it as
		// a hostname, as this used to, asked the LAN for a machine called
		// "node_…" and reported that it could not be reached.
		if strings.HasPrefix(target, "node_") {
			raw, err := daemonRequest("POST", "/api/peers/pair", map[string]any{
				"peerId":  target,
				"address": "relay",
			})
			if err != nil {
				return fail(asJSON, err)
			}
			if asJSON {
				return emitRawJSON(raw)
			}
			success("Pairing request sent through the relay to %s.", bold(target))
			note("Pairing is mutual — approve it on that device to finish.")
			hint("opensave pair requests     (on the other device)")
			return 0
		}

		// `opensave pair <host[:port]>` — ask a device on the LAN to pair.
		host := target
		port := 8383
		if h, p, ok := strings.Cut(host, ":"); ok {
			host = h
			fmt.Sscanf(p, "%d", &port)
		}
		raw, err := daemonRequest("POST", "/api/peers/pair", map[string]any{
			"address": host,
			"port":    port,
		})
		if err != nil {
			return fail(asJSON, err)
		}
		if asJSON {
			return emitRawJSON(raw)
		}
		success("Pairing request sent to %s.", bold(fmt.Sprintf("%s:%d", host, port)))
		note("Pairing is mutual — approve it on that device to finish.")
		hint("opensave pair requests     (on the other device)")
		return 0
	}
}

const pairUsage = `usage:
  opensave pair <host[:port]>     Ask a device on your LAN to pair
  opensave pair <node id>         Ask a device in your relay room to pair (see: opensave peers)
  opensave pair requests          Show incoming pairing requests
  opensave pair approve <peerId>  Approve an incoming request
  opensave pair reject <peerId>   Reject an incoming request`

// cmdUnpair drops a paired device.
func cmdUnpair(args []string) int {
	asJSON, args := jsonFlag(args)
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: opensave unpair <peerId>")
		return 1
	}
	raw, err := daemonRequest("POST", "/api/peers/unpair", map[string]any{"peerId": args[0]})
	if err != nil {
		return fail(asJSON, err)
	}
	if asJSON {
		return emitRawJSON(raw)
	}
	success("Unpaired %s.", bold(args[0]))
	return 0
}

// cmdRelay manages internet sync, which on a headless box is the only way to
// reach devices that aren't on the same LAN.
func cmdRelay(args []string) int {
	asJSON, args := jsonFlag(args)
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, relayUsage)
		return 1
	}

	switch args[0] {
	case "status":
		raw, err := daemonRequest("GET", "/api/settings", nil)
		if err != nil {
			return fail(asJSON, err)
		}
		if asJSON {
			return emitRawJSON(raw)
		}
		var s struct {
			RelayURL string `json:"relayUrl"`
			SyncCode string `json:"syncCode"`
		}
		if json.Unmarshal(raw, &s) != nil {
			return emitRawJSON(raw)
		}
		section("Internet sync")
		if s.SyncCode == "" {
			field("room", faint("not joined"))
			field("relay", faint(s.RelayURL))
			hint("opensave relay join <code>     same code on every device")
			fmt.Println()
			return 0
		}
		field("room", accent(s.SyncCode))
		field("relay", faint(s.RelayURL))
		fmt.Println()
		return 0

	case "join":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: opensave relay join <code>")
			return 1
		}
		raw, err := daemonRequest("POST", "/api/settings", map[string]any{"syncCode": args[1]})
		if err != nil {
			return fail(asJSON, err)
		}
		if asJSON {
			return emitRawJSON(raw)
		}
		success("Joined relay room %s.", accent(args[1]))
		note("Use the same code on your other devices so they find each other.")
		return 0

	case "leave":
		raw, err := daemonRequest("POST", "/api/settings", map[string]any{"syncCode": ""})
		if err != nil {
			return fail(asJSON, err)
		}
		if asJSON {
			return emitRawJSON(raw)
		}
		success("Left the relay room.")
		return 0

	default:
		fmt.Fprintln(os.Stderr, relayUsage)
		return 1
	}
}

const relayUsage = `usage:
  opensave relay status        Show the current relay room
  opensave relay join <code>   Join a relay room (same code on every device)
  opensave relay leave         Leave the current room`
