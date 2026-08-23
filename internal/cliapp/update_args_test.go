package cliapp

import "testing"

// Running bare installs. That makes a silently ignored argument resolve the
// wrong way round: `opensave update check` — the spelling without dashes, and
// the one a user reaches for first — dropped the word and installed, which is
// the opposite of what it asked for. Unknown arguments have to be refused.
func TestUpdateRefusesArgumentsItDoesNotUnderstand(t *testing.T) {
	for _, arg := range []string{"check", "--dry-run", "-c", "install", "--yes"} {
		if rc := cmdUpdate([]string{arg}); rc == 0 {
			t.Errorf("cmdUpdate(%q) returned 0 — an unrecognised argument must not "+
				"fall through to the install path", arg)
		}
	}
}

// The two spellings that do mean something must keep working, and --help is
// not an error.
func TestUpdateStillAcceptsItsRealFlags(t *testing.T) {
	for _, arg := range []string{"--help", "-h"} {
		if rc := cmdUpdate([]string{arg}); rc != 0 {
			t.Errorf("cmdUpdate(%q) returned %d, want 0", arg, rc)
		}
	}
}
