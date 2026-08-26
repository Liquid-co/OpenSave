package delta

import (
	"os"
	"strings"
	"testing"
)

// A Mac peer reports its save paths as /Users/<name>/…, which matched neither
// the Windows branch nor the /home/ one — so those paths were handed to the
// other machine unchanged, and a Windows device would try to use
// "/Users/alice/Library/…" literally.
func TestAMacPeersPathIsTranslated(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home directory on this machine")
	}

	for _, remote := range []string{
		"/Users/alice/Library/Application Support/Studio/Game",
		"/Users/alice/Documents/Game",
	} {
		got := TranslatePathToLocal(remote, nil)
		if got == remote {
			t.Errorf("%q came back unchanged — a Mac peer's path would be used as-is "+
				"on this machine", remote)
			continue
		}
		if !strings.HasPrefix(got, home) {
			t.Errorf("TranslatePathToLocal(%q) = %q, want it under %q", remote, got, home)
		}
		// The part after the peer's home has to survive intact, or the file
		// lands somewhere other than where the game keeps it.
		if !strings.Contains(strings.ToLower(got), "game") {
			t.Errorf("the remainder was lost: %q -> %q", remote, got)
		}
	}
}

// macOS filesystems are usually case-insensitive, so a peer may report either
// spelling of its own home.
func TestAMacPathIsMatchedCaseInsensitively(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home directory")
	}
	got := TranslatePathToLocal("/users/alice/Library/Thing", nil)
	if !strings.HasPrefix(got, home) {
		t.Errorf("a lowercase /users path was not translated: %q", got)
	}
}

// The existing branches must keep working — this is the regression that would
// hurt most, since Windows and Linux are what people actually sync today.
func TestWindowsAndLinuxTranslationStillWork(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home directory")
	}
	for _, remote := range []string{
		`C:\Users\alice\AppData\LocalLow\Studio\Game`,
		"/home/alice/.local/share/Studio/Game",
	} {
		got := TranslatePathToLocal(remote, nil)
		if got == remote {
			t.Errorf("%q was not translated", remote)
		}
		if !strings.HasPrefix(got, home) {
			t.Errorf("TranslatePathToLocal(%q) = %q, want it under %q", remote, got, home)
		}
	}
}

// A path that belongs to no home layout is left alone. Rewriting an arbitrary
// absolute path onto the local home would point a restore somewhere the peer
// never meant.
func TestAnUnrelatedPathIsLeftAlone(t *testing.T) {
	for _, remote := range []string{
		"/opt/games/save",
		"/var/lib/thing",
		"/Usersnothome/alice/x",
	} {
		if got := TranslatePathToLocal(remote, nil); got != remote {
			t.Errorf("TranslatePathToLocal(%q) = %q, want it unchanged", remote, got)
		}
	}
}

// A user's own rule still wins over every built-in layout, which is the whole
// point of having them.
func TestACustomRuleStillTakesPrecedence(t *testing.T) {
	rules := []TranslationRule{{
		FromPattern: "/Users/alice/Library",
		ToPattern:   "/mnt/shared",
	}}
	got := TranslatePathToLocal("/Users/alice/Library/Application Support/G", rules)
	if !strings.HasPrefix(got, "/mnt/shared") {
		t.Errorf("the custom rule was ignored: %q", got)
	}
}
