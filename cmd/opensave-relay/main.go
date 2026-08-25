// Command opensave-relay runs the standalone WAN relay server.
package main

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/opensave/opensave/internal/version"
	"github.com/opensave/opensave/relay"
)

// The relay took any arguments you gave it, ignored them, and started a
// server. Someone self-hosting typed `opensave-relay config set relay-url …`
// — reasonable, since that is roughly what the client command looks like —
// and got "listen tcp 0.0.0.0:10000: bind: address already in use", because
// the arguments went nowhere and the only thing left to do was start a second
// relay next to the one already running. The error described neither what
// they asked for nor what was wrong with it.
//
// Two different binaries with similar names is the underlying trap:
// opensave-relay is the server, opensave is the client, and relay-url is a
// CLIENT setting. Nothing can stop the two being confused, but the relay can
// at least recognise a client command and say where it belongs.
const usage = `opensave-relay — the OpenSave WAN relay server

Commands:

  opensave-relay                     start it
  opensave-relay setup               ask for the secrets and store them
  opensave-relay config              show what is configured, and from where

Secrets come from "setup" or from the environment, environment first:

  GOOGLE_DRIVE_CLIENT_SECRET=…       optional, enables the OAuth token proxy
  STEAMGRIDDB_KEY=…                  optional, enables cover-art lookup

"setup" exists so neither is ever typed on a command line, where a shell
records it, or pasted into a unit file that gets committed. Nothing is echoed
while typing, and the file it writes is created 0600 before anything goes in.

Settings that are not secrets stay in the environment:

  PORT=8386                          port to listen on
  MAX_PER_ROOM=20                    most devices allowed in one room
  OPENSAVE_RELAY_SECRETS=…           where the secrets file lives

  PORT=9000 opensave-relay           on another port

Health check: GET /health — reports room and client counts, nothing else.

Looking for relay-url? That is a setting on the machines whose saves you are
syncing, not on the relay. Run this on each of THOSE, using the opensave
client:

  opensave config set relay-url wss://relay.example.com
  opensave relay join <room-code>

The relay never joins a room and has nothing to sign in to — rooms exist
because clients ask for them. See docs/RELAY.md.`

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-h", "--help", "help":
			fmt.Println(usage)
			return
		case "setup":
			os.Exit(runSetup())
		case "config", "show-config":
			os.Exit(runShowConfig())
		case "-v", "--version", "version":
			fmt.Printf("opensave-relay %s\n", version.Version)
			return
		default:
			// Non-zero, and on stderr: a typo in a systemd unit or a compose
			// file should fail loudly rather than start a server that ignores
			// half its command line.
			fmt.Fprintf(os.Stderr, "opensave-relay: unexpected argument %q\n\n%s\n", os.Args[1], usage)
			os.Exit(2)
		}
	}

	// Secrets come from the environment when it sets them, and from the file
	// `opensave-relay setup` writes otherwise. Environment first so a container
	// or a systemd unit that already injects them keeps working exactly as it
	// did, and so an operator who has automated this is never silently
	// overridden by a file someone left behind.
	secrets, sources := relay.ResolveSecrets(relay.SecretsPath())
	cfg := relay.Config{
		Port:               envInt("PORT", 8386),
		MaxPerRoom:         envInt("MAX_PER_ROOM", 20),
		GoogleClientSecret: secrets.GoogleClientSecret,
		// Optional. Without it the artwork lookup answers "not configured"
		// and clients simply show no cover for games Steam has none for.
		SteamGridDBKey: secrets.SteamGridDBKey,
	}

	srv := relay.New(cfg)
	addr, err := srv.Start()
	if err != nil {
		fmt.Fprintf(os.Stderr, "relay failed to start: %v\n", err)
		// "address already in use" is nearly always a relay already running,
		// and nearly always someone who does not realise it. Say so, rather
		// than leaving them to infer it from an errno.
		if isAddrInUse(err) {
			fmt.Fprintf(os.Stderr,
				"\nSomething is already listening on that port — most likely a relay you started earlier.\n"+
					"Check with: curl http://localhost:%d/health\n"+
					"Run on another port with: PORT=9000 opensave-relay\n", cfg.Port)
		}
		os.Exit(1)
	}
	fmt.Printf("OpenSave WAN Relay listening on %s (health: http://%s/health)\n", addr, addr)
	// Which optional features are actually on, and where each secret came
	// from. Masked, never the value: a relay log is the last place a secret
	// should end up, and "configured or not" is all a banner needs to say.
	fmt.Printf("  cloud sync:  %-14s (%s)\n",
		relay.MaskSecret(cfg.GoogleClientSecret), sources["googleClientSecret"])
	fmt.Printf("  cover art:   %-14s (%s)\n",
		relay.MaskSecret(cfg.SteamGridDBKey), sources["steamGridDBKey"])
	if cfg.GoogleClientSecret == "" && cfg.SteamGridDBKey == "" {
		fmt.Println("  Run `opensave-relay setup` to configure these without typing them on a command line.")
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	fmt.Println("\nshutting down...")
	srv.Stop()
}

// isAddrInUse reports whether a listen failure was a port clash. Matched on
// the message rather than syscall.EADDRINUSE so it reads the same on Windows,
// where the equivalent errno has a different name and value.
func isAddrInUse(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return containsFold(msg, "address already in use") ||
		containsFold(msg, "only one usage of each socket address")
}

func containsFold(haystack, needle string) bool {
	h, n := []byte(haystack), []byte(needle)
	if len(n) > len(h) {
		return false
	}
	lower := func(b byte) byte {
		if b >= 'A' && b <= 'Z' {
			return b + 32
		}
		return b
	}
	for i := 0; i+len(n) <= len(h); i++ {
		ok := true
		for j := range n {
			if lower(h[i+j]) != lower(n[j]) {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
