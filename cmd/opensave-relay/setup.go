package main

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strings"

	"golang.org/x/term"

	"github.com/opensave/opensave/relay"
)

// `opensave-relay setup` asks for the secrets a relay needs and stores them,
// so an operator never types one on a command line.
//
// The two ways a secret usually leaks are a shell that records the command and
// a unit file that ends up committed. Prompting avoids both: nothing is echoed
// while typing, nothing is written anywhere but the secrets file, and that file
// is created 0600 before a byte goes into it.
func runSetup() int {
	path := relay.SecretsPath()
	existing, _ := relay.LoadSecrets(path)
	_, sources := relay.ResolveSecrets(path)

	fmt.Println()
	fmt.Println("  OpenSave relay setup")
	fmt.Println("  --------------------")
	fmt.Printf("  Secrets file: %s\n\n", path)

	// An operator whose values come from the environment needs to know that
	// before they type anything: what they enter here would be stored and then
	// ignored, which is a genuinely confusing hour to spend.
	overridden := false
	for _, name := range []string{"googleClientSecret", "steamGridDBKey"} {
		if strings.HasPrefix(sources[name], "environment") {
			fmt.Printf("  Note: %s is set in the %s and will take precedence over\n"+
				"        anything stored here.\n", name, sources[name])
			overridden = true
		}
	}
	if overridden {
		fmt.Println()
	}

	next := existing

	if v, ok := prompt(
		"  Google Drive client secret",
		"  Enables cloud sync through this relay. Leave blank to skip.",
		existing.GoogleClientSecret); ok {
		next.GoogleClientSecret = v
	}

	if v, ok := prompt(
		"  SteamGridDB API key",
		"  Enables cover art for games Steam has none for. Leave blank to skip.\n"+
			"  Get one free at https://www.steamgriddb.com/profile/preferences/api",
		existing.SteamGridDBKey); ok {
		next.SteamGridDBKey = v
	}

	if next == existing {
		fmt.Println("\n  Nothing changed.")
		return 0
	}
	if err := relay.SaveSecrets(path, next); err != nil {
		fmt.Fprintf(os.Stderr, "\n  Could not write %s: %v\n", path, err)
		return 1
	}

	fmt.Printf("\n  Saved to %s\n", path)

	// Hand the file to the account the service runs as.
	//
	// Without this, running setup under sudo on a machine installed by
	// install-relay.sh writes a root-owned 0600 file the service account
	// cannot read: the key is saved, reported as configured, and never
	// reaches the relay. Nothing surfaces that — the relay answers "no key
	// configured" while `config` says the opposite.
	if owner, err := relay.HandOverToServiceUser(path); err != nil {
		fmt.Fprintf(os.Stderr,
			"\n  WARNING: saved, but this file could not be handed to the %q account the\n"+
				"  relay service runs as: %v\n"+
				"  The relay will not be able to read it. Fix the ownership, or put these\n"+
				"  values in /etc/opensave-relay/env instead, which systemd reads as root.\n",
			relay.ServiceUser(), err)
	} else if owner != "" {
		fmt.Printf("  Owner: %s (the account the relay service runs as).\n", owner)
	}
	// Only claimed where it is true. Go's Chmod on Windows toggles the
	// read-only bit and nothing else, so the 0600 this is written with has no
	// effect there — saying "readable only by you" on Windows would be a plain
	// untruth about a file holding a secret.
	if runtime.GOOS == "windows" {
		fmt.Println("  Note: on Windows this file inherits the folder's permissions.")
		fmt.Println("  Keep it where only your account can read, or point")
		fmt.Println("  OPENSAVE_RELAY_SECRETS at a path you have locked down.")
	} else {
		fmt.Println("  Permissions: 0600 (readable only by you).")
	}
	fmt.Printf("  Google Drive client secret: %s\n", relay.MaskSecret(next.GoogleClientSecret))
	fmt.Printf("  SteamGridDB API key:        %s\n", relay.MaskSecret(next.SteamGridDBKey))
	fmt.Println("\n  Restart the relay for these to take effect.")
	return 0
}

// prompt asks for one secret. Returns ok false when the operator pressed enter
// on a value that already exists, which means "leave it alone" — re-running
// setup to change one secret must not blank the other.
func prompt(label, help, current string) (string, bool) {
	fmt.Println(label)
	if help != "" {
		fmt.Println(help)
	}
	if current != "" {
		fmt.Printf("  Currently: %s — press enter to keep it, or type a new value.\n", relay.MaskSecret(current))
	}
	fmt.Print("  > ")

	value, err := readHidden()
	fmt.Println()
	if err != nil {
		fmt.Fprintf(os.Stderr, "  (could not read input: %v)\n", err)
		return "", false
	}
	value = strings.TrimSpace(value)

	if value == "" {
		if current != "" {
			fmt.Println("  Kept.")
			return "", false
		}
		fmt.Println("  Skipped.")
		return "", false
	}
	fmt.Printf("  Set: %s\n\n", relay.MaskSecret(value))
	return value, true
}

// stdinReader is shared across prompts. A fresh bufio.Reader per prompt reads
// ahead and swallows everything after the first line, so the second prompt saw
// EOF and was silently skipped — which for a setup command means a secret the
// operator typed was never stored.
var sharedStdin *bufio.Reader

func stdinReader() *bufio.Reader {
	if sharedStdin == nil {
		sharedStdin = bufio.NewReader(os.Stdin)
	}
	return sharedStdin
}

// readHidden reads a line without echoing it.
//
// Falls back to a plain read when stdin is not a terminal — a setup piped from
// a provisioning script has nothing to hide the typing from, and refusing
// would break exactly the automation that should be encouraged.
func readHidden() (string, error) {
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		b, err := term.ReadPassword(fd)
		return string(b), err
	}
	line, err := stdinReader().ReadString('\n')
	return strings.TrimRight(line, "\r\n"), err
}

// runShowConfig prints what the relay would use, and where each value comes
// from, without revealing any of it.
func runShowConfig() int {
	path := relay.SecretsPath()
	secrets, sources := relay.ResolveSecrets(path)

	fmt.Println()
	fmt.Printf("  Secrets file: %s\n\n", path)
	fmt.Printf("  Google Drive client secret: %-14s from %s\n",
		relay.MaskSecret(secrets.GoogleClientSecret), sources["googleClientSecret"])
	fmt.Printf("  SteamGridDB API key:        %-14s from %s\n",
		relay.MaskSecret(secrets.SteamGridDBKey), sources["steamGridDBKey"])

	// Say who owns the file when a service account is involved. This command
	// is usually run as root, and root can read a file the service cannot —
	// so reporting "configured" while the relay sees nothing is exactly the
	// dead end this line exists to prevent.
	if svc := relay.ServiceUser(); svc != "" {
		if owner := relay.FileOwner(path); owner != "" && owner != svc {
			fmt.Printf("\n  WARNING: %s is owned by %q, but the relay service runs as %q.\n"+
				"  A 0600 file owned by another account cannot be read by the service, so\n"+
				"  these values will not reach it however correct they look here.\n"+
				"  Run `opensave-relay setup` again to fix the ownership.\n", path, owner, svc)
		}
	}
	fmt.Println()
	return 0
}
