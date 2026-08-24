// Package winreg captures and restores the registry keys some games keep
// their saves in.
//
// 430 games in the Ludusavi manifest declare a save-tagged registry key, and
// for 303 of them it is the only place a save exists — those games had nothing
// for a file-based backup to capture, so they were dropped from the scan index
// entirely rather than backed up badly.
//
// A capture is a snapshot of one key and everything below it: value names,
// their types, and their data. Restoring writes those values back. Nothing
// else about the key is preserved — not ACLs, not timestamps — because a save
// is the data, and reapplying a security descriptor from another machine is a
// good way to make a key unreadable.
//
// The format is JSON rather than a .reg file. A .reg is a Windows-only text
// format with escaping rules that differ per value type, and it is executable
// by double-click, which is a poor thing to put inside an archive a user might
// extract by hand. JSON survives being read on a Steam Deck, which is where
// half of a sync pair often is.
package winreg

import (
	"errors"
	"fmt"
	"strings"
)

// ErrUnsupported is returned by Capture and Restore on platforms with no
// registry. It is not a failure: a Linux device syncing with a Windows one
// holds the captured values in its snapshots and hands them back on restore,
// it just cannot read or write any of its own.
var ErrUnsupported = errors.New("the registry is only available on Windows")

// Key is one captured registry key and the values beneath it.
type Key struct {
	// Path is the full key, hive included, exactly as the manifest writes it:
	// forward slashes, full hive name. Kept verbatim so a captured snapshot
	// can be matched back to the manifest entry that asked for it.
	Path   string  `json:"path"`
	Values []Value `json:"values,omitempty"`
	// Subkeys are captured recursively, each with its own full path.
	Subkeys []Key `json:"subkeys,omitempty"`
}

// Value is one registry value: its name, its type, and its data.
//
// Data is base64 for binary types and a plain string otherwise, which keeps a
// captured save readable when someone opens the JSON to see what a game stored.
type Value struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Data string `json:"data,omitempty"`
	// Multi holds REG_MULTI_SZ, which is a list rather than one string.
	Multi []string `json:"multi,omitempty"`
}

// Capture reads a key and everything under it.
//
// A key that does not exist is not an error and returns ok false: a game that
// has never been run has never written its key, and a snapshot of "nothing
// here yet" is the correct capture of that state.
func Capture(path string) (Key, bool, error) {
	return capture(path)
}

// Restore writes a captured key back, creating it if needed.
//
// Values present in the capture are written; values that exist now but are not
// in the capture are left alone. A restore puts back what was saved rather
// than making the key byte-identical to the moment of capture — the difference
// matters for keys a game shares with its own settings, where deleting an
// unrelated value would change behaviour nobody asked to roll back.
func Restore(k Key) error {
	return restore(k)
}

// Available reports whether this platform can read and write the registry.
func Available() bool { return available() }

// NormalizePath accepts the spellings a registry key arrives in and returns
// the canonical one: full hive name, backslashes.
//
// The manifest writes "HKEY_CURRENT_USER/Software/Landfall Games/Rounds";
// Windows tooling writes "HKCU\Software\...". Both name one key.
func NormalizePath(p string) string {
	p = strings.ReplaceAll(strings.TrimSpace(p), "/", `\`)
	p = strings.TrimRight(p, `\`)
	hive, rest, found := strings.Cut(p, `\`)
	full, ok := hiveAliases[strings.ToUpper(hive)]
	if !ok {
		return p
	}
	if !found {
		return full
	}
	return full + `\` + rest
}

var hiveAliases = map[string]string{
	"HKCU": "HKEY_CURRENT_USER", "HKEY_CURRENT_USER": "HKEY_CURRENT_USER",
	"HKLM": "HKEY_LOCAL_MACHINE", "HKEY_LOCAL_MACHINE": "HKEY_LOCAL_MACHINE",
	"HKCR": "HKEY_CLASSES_ROOT", "HKEY_CLASSES_ROOT": "HKEY_CLASSES_ROOT",
	"HKU": "HKEY_USERS", "HKEY_USERS": "HKEY_USERS",
	"HKCC": "HKEY_CURRENT_CONFIG", "HKEY_CURRENT_CONFIG": "HKEY_CURRENT_CONFIG",
}

// splitHive separates the canonical hive name from the subkey path.
func splitHive(p string) (hive, sub string, err error) {
	p = NormalizePath(p)
	hive, sub, _ = strings.Cut(p, `\`)
	if _, ok := hiveAliases[hive]; !ok {
		return "", "", fmt.Errorf("unrecognised registry hive in %q", p)
	}
	return hive, sub, nil
}
