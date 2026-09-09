package delta

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The Windows rules are asked about explicitly rather than through
// UnrepresentableName, so they are checked on every machine. A test that only
// ran on Windows would be useless here: the names in question can only be
// CREATED on Linux or macOS, so the interesting direction is a Linux peer
// sending to a Windows one, and no single machine sees both ends.

func TestWindowsRejectsNamesLinuxAllows(t *testing.T) {
	cases := []struct {
		rel  string
		want string // a distinctive fragment of the reason
	}{
		{`what?.sav`, `"?"`},
		{`star*.sav`, `"*"`},
		{`pipe|.sav`, `"|"`},
		{`less<than.sav`, `"<"`},
		{`more>than.sav`, `">"`},
		{`quote".sav`, `"\""`},
		{`foo:bar.sav`, "alternate data stream"},
		{"tab\t.sav", "control character"},
		{"slot1/what?.sav", `"?"`},     // in a nested component
		{"bad?dir/save.sav", `"?"`},    // in a directory component
		{"trailing. ", "dot or space"}, // Windows silently strips these
		{"trailingdot.", "dot or space"},
		{"CON", "reserved device name"},
		{"con.sav", "reserved device name"},
		{"COM1.dat", "reserved device name"},
		{"lpt9", "reserved device name"},
		{"saves/NUL.sav", "reserved device name"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.rel, func(t *testing.T) {
			got := unrepresentableOn("windows", tc.rel)
			if got == "" {
				t.Fatalf("unrepresentableOn(windows, %q) = \"\"; this name cannot be "+
					"created on Windows and must be reported", tc.rel)
			}
			if !strings.Contains(got, tc.want) {
				t.Errorf("reason for %q was %q, want it to mention %s", tc.rel, got, tc.want)
			}
		})
	}
}

// The colon is called out separately because it is the dangerous one: Windows
// accepts the write and routes it into an alternate data stream, so without
// this check the file "syncs" and is invisible in the folder.
func TestTheColonCaseIsExplainedNotJustRejected(t *testing.T) {
	got := unrepresentableOn("windows", "foo:bar.sav")
	if !strings.Contains(got, "alternate data stream") {
		t.Errorf("reason = %q; a colon does not fail on Windows, it silently "+
			"redirects into a stream, and the message has to say so", got)
	}
}

// Ordinary save names must not be swept up. A false positive here means a
// file that silently never syncs, which is the failure this change exists to
// stop.
func TestOrdinaryNamesAreRepresentableEverywhere(t *testing.T) {
	ok := []string{
		"save.sav",
		"slot1/save.sav",
		"a/b/c/deep.sav",
		"with space/a b c.sav",
		"日本語.sav",
		"naïve/café.sav",
		"dots.in.the.name.sav",
		"UPPERCASE.SAV",
		"consortium.sav",  // starts with "con" but is not the device name
		"lpt10.sav",       // only lpt1-9 are reserved
		"com0.sav",        // com0 is not reserved
		"prnt.sav",        // not "prn"
		"a-b_c(1)[2].sav", // punctuation Windows allows
	}
	for _, goos := range []string{"windows", "linux", "darwin"} {
		for _, rel := range ok {
			if reason := unrepresentableOn(goos, rel); reason != "" {
				t.Errorf("unrepresentableOn(%s, %q) = %q; this is an ordinary save "+
					"name and refusing it means the file never syncs", goos, rel, reason)
			}
		}
	}
}

// Linux and macOS allow everything Windows forbids, so nothing may be skipped
// when syncing in that direction — otherwise the fix would trade one silent
// data gap for another.
func TestLinuxAndMacAcceptWhatWindowsRefuses(t *testing.T) {
	for _, rel := range []string{
		"what?.sav", "star*.sav", "foo:bar.sav", "pipe|.sav", "CON", "trailing. ",
	} {
		for _, goos := range []string{"linux", "darwin"} {
			if reason := unrepresentableOn(goos, rel); reason != "" {
				t.Errorf("unrepresentableOn(%s, %q) = %q; %s allows this name",
					goos, rel, reason, goos)
			}
		}
	}
}

// A NUL byte terminates a path in every syscall interface there is.
func TestANulByteIsRejectedOnEveryPlatform(t *testing.T) {
	for _, goos := range []string{"windows", "linux", "darwin"} {
		if reason := unrepresentableOn(goos, "save\x00.sav"); reason == "" {
			t.Errorf("unrepresentableOn(%s, ...) accepted a name containing NUL", goos)
		}
	}
}

// Structural segments are containment's problem, not legality's — reporting
// them here would mean IsSafePath's rejections arrived with a misleading
// explanation.
func TestStructuralSegmentsAreNotTreatedAsIllegalNames(t *testing.T) {
	for _, rel := range []string{"a/./b.sav", "a/../b.sav"} {
		if reason := unrepresentableOn("windows", rel); reason != "" {
			t.Errorf("unrepresentableOn(windows, %q) = %q; \".\" and \"..\" are "+
				"structure, and containment is IsSafePath's job", rel, reason)
		}
	}
}

// The rules have to describe the machine they actually run on. Asserted by
// trying the write: if this platform really can create the name, refusing it
// would be a bug, and if it cannot, accepting it would be.
func TestTheRulesMatchWhatThisFilesystemDoes(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{
		"ordinary.sav", "what?.sav", "star*.sav", "pipe|.sav", "less<than.sav",
	} {
		predicted := UnrepresentableName(name) != ""

		err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644)
		// A colon is excluded from this loop deliberately: the write succeeds
		// on Windows and the file simply is not there afterwards, so "did the
		// write error" is the wrong question for it.
		created := err == nil
		if created {
			// It must also be visible under that name, not hidden in a stream.
			if _, statErr := os.Stat(filepath.Join(dir, name)); statErr != nil {
				created = false
			}
		}

		if predicted && created {
			t.Errorf("%s: reported unrepresentable, but this filesystem created it fine",
				name)
		}
		if !predicted && !created {
			t.Errorf("%s: reported fine, but the write failed here (%v) — the sync "+
				"would abort on it", name, err)
		}
	}
	t.Logf("checked against a real %s filesystem", runtime.GOOS)
}
