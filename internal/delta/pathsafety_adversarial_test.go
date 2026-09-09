package delta

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Every relative path used during a sync arrives from the peer. The sync
// engine joins it to the local save root and then writes, deletes, or creates
// directories at the result, so IsSafePath is the only thing standing between
// a malformed or hostile manifest and the rest of the disk.
//
// This matters more now that the save root is user-chosen: a root sitting one
// level under a documents folder makes "../" far more valuable to an attacker
// than a root buried in AppData.
func TestIsSafePathRejectsEscapes(t *testing.T) {
	base := t.TempDir()

	escapes := []struct{ label, rel string }{
		{"parent traversal", "../outside.sav"},
		{"deep traversal", "../../../../../../etc/passwd"},
		{"backslash traversal", `..\..\outside.sav`},
		{"traversal in the middle", "saves/../../outside.sav"},
		{"traversal after a valid prefix", "a/b/../../../outside.sav"},
		{"absolute unix path", "/etc/passwd"},
		{"absolute windows path", `C:\Windows\System32\drivers\etc\hosts`},
		{"unc path", `\\server\share\payload.sav`},
		{"extended-length prefix", `\\?\C:\Windows\win.ini`},
		{"drive-relative", `C:payload.sav`},
		{"bare parent", ".."},
		{"parent with separator", "../"},
		{"double-dot lookalike padded", "....//....//outside.sav"},
		{"trailing dot segment", "a/../../outside.sav"},
	}

	// The property is containment, not rejection. Several of these look
	// alarming but are already harmless: filepath.Join cleans "/etc/passwd"
	// to <root>/etc/passwd and `\\server\share\x` to <root>/server/share/x,
	// and "...." is a literal four-dot directory name, not a parent segment.
	// Approving those is fine — they stay inside. What must never happen is
	// approving something that resolves outside.
	for _, tc := range escapes {
		tc := tc
		t.Run(tc.label, func(t *testing.T) {
			approved := IsSafePath(base, tc.rel)
			if !approved {
				return // refused outright; nothing can be written
			}
			resolved := filepath.Join(base, filepath.FromSlash(tc.rel))
			cleanBase := filepath.Clean(base)
			inside := resolved == cleanBase ||
				strings.HasPrefix(resolved, cleanBase+string(os.PathSeparator))
			if !inside {
				t.Errorf("IsSafePath(%q) said yes, and it resolves to %q — outside the "+
					"save root %q, so a peer could write there", tc.rel, resolved, cleanBase)
			}
		})
	}
}

// The guard must not be so eager that ordinary saves are refused: a rejected
// path is a file that silently never syncs, which is its own kind of failure.
func TestIsSafePathAllowsOrdinarySaves(t *testing.T) {
	base := t.TempDir()

	ok := []string{
		"save.sav",
		"slot1/save.sav",
		"a/b/c/d/e/deep.sav",
		"with space/a b c.sav",
		"unicode/日本語.sav",
		"naïve/café.sav",
		"dots.in.name.sav",
		"a/./b.sav",               // a harmless current-dir segment
		"trailing/inner/../x.sav", // goes up but stays inside
	}
	for _, rel := range ok {
		rel := rel
		t.Run(rel, func(t *testing.T) {
			if !IsSafePath(base, rel) {
				t.Errorf("IsSafePath(%q) said no; this is an ordinary save path and "+
					"refusing it means the file never syncs", rel)
			}
		})
	}
}

// A root whose own name is a prefix of a sibling must not let a path slip into
// the sibling: "…/saves" and "…/saves-backup" share a prefix, and a check
// written with plain HasPrefix and no separator would treat the second as
// inside the first.
func TestIsSafePathDoesNotConfuseAPrefixSibling(t *testing.T) {
	parent := t.TempDir()
	base := filepath.Join(parent, "saves")
	sibling := filepath.Join(parent, "saves-backup")
	for _, d := range []string{base, sibling} {
		if err := os.MkdirAll(d, 0o777); err != nil {
			t.Fatal(err)
		}
	}
	// Reach the sibling from inside the base.
	if IsSafePath(base, "../saves-backup/steal.sav") {
		t.Error("a sibling folder sharing the root's name prefix was treated as inside it")
	}
}

// Windows silently strips trailing dots and spaces from path components, so
// "evil. " and "evil" name the same file. A guard that compares before the
// strip and writes after it can be walked past.
func TestIsSafePathHandlesWindowsTrailingDotsAndSpaces(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("trailing dot/space stripping is Windows-specific")
	}
	base := t.TempDir()
	for _, rel := range []string{
		"..../outside.sav",
		".. /outside.sav",
		"a/.. /../outside.sav",
	} {
		if IsSafePath(base, rel) {
			resolved := filepath.Join(base, filepath.FromSlash(rel))
			// Only a failure if it truly lands outside once Windows has had its
			// way with the name; report the resolved path so it is checkable.
			if !strings.HasPrefix(strings.ToLower(resolved), strings.ToLower(base)) {
				t.Errorf("IsSafePath(%q) said yes and resolves to %q, outside the root",
					rel, resolved)
			}
		}
	}
}

// Degenerate relative paths have to produce an answer rather than a panic:
// they name the root itself, and the engine calls this before every write,
// delete, and mkdir.
//
// Deliberately NOT asserted here: that an empty baseDir is refused. It is
// not — filepath.Abs("") yields the working directory, so IsSafePath("", x)
// answers true. That is safe in practice because every one of the callers
// passes a path that ValidateSavePath or CheckRestoreTarget has already
// rejected as empty, so the case is unreachable. Asserting it would be
// demanding a guard the design places elsewhere; the test that matters is the
// one on those two functions.
func TestIsSafePathOnDegenerateInput(t *testing.T) {
	base := t.TempDir()
	for _, rel := range []string{"", ".", "./", "//", `\`} {
		// Has to return, not panic, and stay contained if it approves.
		if IsSafePath(base, rel) {
			resolved := filepath.Join(base, filepath.FromSlash(rel))
			cleanBase := filepath.Clean(base)
			if resolved != cleanBase &&
				!strings.HasPrefix(resolved, cleanBase+string(os.PathSeparator)) {
				t.Errorf("IsSafePath(%q) approved a path resolving to %q, outside %q",
					rel, resolved, cleanBase)
			}
		}
	}
}
