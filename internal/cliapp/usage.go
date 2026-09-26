package cliapp

import (
	"fmt"
	"sort"
	"strings"
)

// The command reference as data rather than a printed blob. Two things fall
// out of that: `--help` can be styled like every other screen instead of
// dumping unthemed text, and shell completion is derived from the same list,
// so a new command can't appear in one and be missing from the other.

type commandEntry struct {
	usage string // "sync [<gameId>|--all]"
	desc  string
}

type commandGroup struct {
	title   string
	entries []commandEntry
}

var commandGroups = []commandGroup{
	{"Games", []commandEntry{
		{"scan [--all]", "Auto-detect game saves (--all includes empty folders)"},
		{"add <name> <path>", "Track a game save folder or file"},
		{"add <path>", "Track a folder or file, named from its path"},
		{"add <number>", "Track one of the last scan's results"},
		{"remove <gameId>", "Stop tracking a game"},
		{"untrack-all --yes", "Stop tracking everything (keeps snapshots)"},
		{"game <gameId> set <key> <value>", "Per-game settings (path, app-id, auto-sync…)"},
		{"launch <gameId>", "Start the game"},
		{"wrap <gameId> -- <command>", "Run a game: newest save first, a snapshot and sync after"},
		{"sessions [<gameId>]", "When games were played here, and for how long"},
		{"status", "Tracked games, branches and peers"},
		{"collection list|create|rename|delete|add|remove", "Group games (Favourites is built in)"},
	}},
	{"Sync", []commandEntry{
		{"sync [<gameId>|--all]", "Sync now (everything by default)"},
		{"pause [30m|1h|…]", "Stop syncing for a while, or until resumed"},
		{"resume", "Start syncing again and catch up"},
		{"transfers", "What is moving between devices, and what moved lately"},
		{"peers", "Paired, discovered and pending devices"},
		{"peers games <peerId>", "What that device is tracking"},
		{"pair <host[:port]>", "Ask a device on your LAN to pair"},
		{"pair <node id>", "Ask a device in your relay room to pair"},
		{"pair requests|approve|reject", "Handle incoming pairing requests"},
		{"unpair <peerId>", "Drop a paired device"},
		{"probe <host[:port]>", "Check whether a device answers"},
		{"forget <peerId>", "Remove a stale device record"},
		{"relay status|join|leave", "Internet sync between networks"},
		{"conflicts", "Saves waiting on a decision"},
		{"resolve <gameId> <choice>", "Settle a conflict"},
	}},
	{"History", []commandEntry{
		{"snapshot <gameId> [-m] [comment]", "Create a snapshot"},
		{"snapshot --all [comment]", "Snapshot every tracked game"},
		{"snapshots <gameId>", "List snapshots"},
		{"snapshot-diff <gameId> <from> <to>", "What changed from one snapshot to another"},
		{"rollback <gameId> <snapId> [--dry-run]", "Restore a snapshot (or list what it would change)"},
		{"branch <gameId> <name>", "Create a branch"},
		{"checkout <gameId> <name>", "Switch branch"},
		{"branch-delete <gameId> <name>", "Delete a branch and its snapshots"},
		{"snapshot-delete <id> <snapId>", "Delete one snapshot"},
		{"snapshot-pin <id> <snapId>", "Keep a snapshot through every limit and clean-up"},
		{"snapshot-unpin <id> <snapId>", "Let the limits apply to it again"},
		{"snapshot-note <id> <snapId> [note]", "Write a note on a snapshot (none removes it)"},
		{"prune [--apply-default]", "Apply retention limits now"},
		{"storage", "Space per game, the biggest snapshots, what prune frees; --compact shares unchanged files now"},
		{"verify [<gameId>] [--repair|--remove-damaged]", "Check every snapshot can still be restored; repair from the cloud, or remove what cannot be"},
		{"emptied [<gameId> delete|restore]", "Saves emptied here, held back from your other devices; answer for one"},
		{"files <gameId> <snapId> [path]", "List a snapshot's contents, or restore one file"},
		{"export <gameId> <dir>", "Copy the current save out to a folder"},
		{"backup export <file.sscb>", "Write a portable backup archive"},
		{"backup import <file.sscb>", "Read one back"},
	}},
	{"Cloud backup", []commandEntry{
		{"cloud connect <provider>", "Sign in to Google Drive, Dropbox or OneDrive"},
		{"cloud setup <provider>", "Configure WebDAV, a webhook, or a local folder"},
		{"cloud disconnect", "Forget the current provider"},
		{"cloud status", "Provider and connection state"},
		{"cloud browse", "Everything stored in the cloud"},
		{"cloud list <gameId>", "Cloud snapshots for one game"},
		{"cloud push <gameId>", "Upload local snapshots"},
		{"cloud restore <id> <file>", "Pull one back"},
		{"cloud delete <id> <file>", "Remove one cloud snapshot"},
		{"cloud delete <gameId> --yes", "Remove every cloud copy of a game"},
		{"cloud check", "Newer saves from your other devices"},
		{"cloud take|skip <gameId>", "Bring that save here, or keep this one"},
	}},
	{"Configuration", []commandEntry{
		{"config [set <key> <value>]", "Read or change settings"},
		{"scanpath list|add|remove", "Extra folders to auto-scan"},
		{"exclude list|add|remove", "Folders auto-scan should skip"},
		{"link <gameId> <otherId>", "Treat two tracked games as the same"},
		{"unlink <aliasId>", "Undo a link"},
		{"links <gameId>", "Show ids linked to a game"},
		{"locations <gameId>", "Extra save folders belonging to a game"},
		{"ignore <gameId>", "Files in a save folder that should not sync"},
	}},
	{"Service", []commandEntry{
		{"daemon start [--port N]", "Run the daemon (REST API + watcher)"},
		{"daemon status", "Is a daemon running, and where"},
		{"daemon stop", "Stop a daemon started by the CLI"},
		{"service install|uninstall", "Install a systemd --user service (Linux)"},
		{"install [--dir D]", "Put this binary on your PATH"},
		{"install --uninstall", "Take it off again (keeps your data)"},
		{"completion bash|zsh|fish", "Shell completion script"},
		{"upnp <port> [--delete]", "Forward or remove a router port via UPnP"},
		{"update [--check]", "Update this CLI to the latest release"},
		{"version", "Print the version"},
	}},
}

// printUsage renders the reference using the same styling as everything else.
func printUsage() {
	fmt.Println()
	fmt.Printf("  %s   %s\n", heading("OpenSave"), faint("peer-to-peer game save sync"))
	fmt.Printf("  %s\n", faint("No account, no server, no quota. Devices sync directly with each other."))

	// Width the command column to the longest entry so descriptions line up
	// across every group, not just within one.
	width := 0
	for _, g := range commandGroups {
		for _, e := range g.entries {
			if n := len(e.usage); n > width {
				width = n
			}
		}
	}

	for _, g := range commandGroups {
		fmt.Printf("\n  %s\n", faint(strings.ToUpper(g.title)))
		for _, e := range g.entries {
			fmt.Printf("    %s %s\n",
				padRight(accent(e.usage), width+2), faint(e.desc))
		}
	}

	fmt.Printf("\n  %s\n\n", faint("Add --json to any command for machine-readable output."))
}

// commandNames returns every top-level command word, derived from the groups
// above so completion can never drift from the documented list.
func commandNames() []string {
	seen := map[string]bool{"help": true}
	for _, g := range commandGroups {
		for _, e := range g.entries {
			name := strings.Fields(e.usage)[0]
			seen[name] = true
		}
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
