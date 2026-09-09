package delta

import (
	"fmt"
	"runtime"
	"strings"
)

// Filenames that are ordinary on one operating system and impossible on
// another.
//
// Linux and macOS allow almost any byte in a filename except NUL and the
// separator. Windows forbids < > : " | ? * and control characters, strips
// trailing dots and spaces, and reserves a set of device names. A save folder
// synced from a Linux or macOS peer can therefore contain names that cannot
// be created here at all.
//
// Two things made this worth catching explicitly rather than letting the
// write fail:
//
//   - A colon is not an error on Windows. "foo:bar.sav" writes successfully
//     into an NTFS alternate data stream attached to a file named "foo", so
//     the bytes land somewhere no directory listing shows and no game reads.
//     Verified on Windows 11: the write returns nil and the name never
//     appears in os.ReadDir.
//   - The other characters fail the write outright, and the pull loop
//     returns on the first error — so one such file aborted the whole sync
//     for that game, every time, and nothing else transferred either.

// reservedWindowsDeviceNames are refused by the OS as a file name with or
// without an extension: "CON", "con.sav" and "CON.tar.gz" all fail.
var reservedWindowsDeviceNames = map[string]bool{
	"con": true, "prn": true, "aux": true, "nul": true,
	"com1": true, "com2": true, "com3": true, "com4": true, "com5": true,
	"com6": true, "com7": true, "com8": true, "com9": true,
	"lpt1": true, "lpt2": true, "lpt3": true, "lpt4": true, "lpt5": true,
	"lpt6": true, "lpt7": true, "lpt8": true, "lpt9": true,
}

// UnrepresentableName reports why relPath cannot be created on this machine's
// filesystem, or "" when it can. relPath is slash-separated, as manifest keys
// are.
func UnrepresentableName(relPath string) string {
	return unrepresentableOn(runtime.GOOS, relPath)
}

// unrepresentableOn is the same question asked about a named operating
// system, so the Windows rules can be tested from any machine — the whole
// point being that the bug only shows up between two different systems.
func unrepresentableOn(goos, relPath string) string {
	if relPath == "" {
		return "the name is empty"
	}
	// A NUL byte terminates a path in every syscall interface there is, on
	// every platform, so it is never representable.
	if strings.ContainsRune(relPath, 0) {
		return "the name contains a NUL byte"
	}
	if goos != "windows" {
		return ""
	}

	for _, component := range strings.Split(relPath, "/") {
		if component == "" || component == "." || component == ".." {
			// Structural, not a name. Containment is IsSafePath's job.
			continue
		}
		if reason := unrepresentableWindowsComponent(component); reason != "" {
			return reason
		}
	}
	return ""
}

func unrepresentableWindowsComponent(component string) string {
	for _, r := range component {
		switch {
		case r < 0x20:
			return fmt.Sprintf("%q contains a control character (0x%02x), which Windows forbids in a file name", component, r)
		case strings.ContainsRune(`<>:"|?*`, r):
			extra := ""
			if r == ':' {
				// Worth naming, because this one does not fail — it silently
				// redirects the contents into an alternate data stream.
				extra = " — Windows would treat this as an alternate data stream and the file would not appear in the folder"
			}
			return fmt.Sprintf("%q contains %q, which Windows forbids in a file name%s", component, string(r), extra)
		}
	}

	// Windows silently drops these, so "save. " and "save" become the same
	// file. Letting it through would quietly merge two distinct saves.
	if trimmed := strings.TrimRight(component, ". "); trimmed != component {
		return fmt.Sprintf("%q ends with a dot or space, which Windows strips — it would collide with %q", component, trimmed)
	}

	// The device name is reserved whatever extension follows it.
	stem := component
	if i := strings.Index(stem, "."); i >= 0 {
		stem = stem[:i]
	}
	if reservedWindowsDeviceNames[strings.ToLower(strings.TrimSpace(stem))] {
		return fmt.Sprintf("%q uses the reserved device name %q, which Windows will not create", component, stem)
	}
	return ""
}
