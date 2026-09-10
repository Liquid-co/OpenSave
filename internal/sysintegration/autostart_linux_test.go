//go:build linux

package sysintegration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The entry that starts OpenSave at login must ask for a hidden start.
//
// For a long time it launched the bare executable, which is exactly what a
// double-click does, so every login brought the full window up over the
// desktop. Someone who enabled start-at-login asked for the syncing, not the
// window; this is the line that makes the difference.
func TestAutostartEntryStartsHidden(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FLATPAK_ID", "")

	if err := SetAutostart(true); err != nil {
		t.Fatalf("SetAutostart: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(home, ".config", "autostart", "opensave.desktop"))
	if err != nil {
		t.Fatalf("no .desktop file was written: %v", err)
	}
	entry := string(raw)

	exec := execLine(t, entry)
	if !strings.HasSuffix(exec, " "+StartHiddenFlag) {
		t.Errorf("Exec line does not end with %s, so login would show the window:\n  %s",
			StartHiddenFlag, exec)
	}
	// The path is quoted: a home directory with a space in it is ordinary,
	// and an Exec line splits on whitespace.
	exe, _ := os.Executable()
	if !strings.HasPrefix(exec, `Exec="`+exe+`"`) {
		t.Errorf("executable path is not quoted as the first Exec token:\n  %s", exec)
	}

	if !AutostartEnabled() {
		t.Error("AutostartEnabled reports false right after SetAutostart(true)")
	}
	if err := SetAutostart(false); err != nil {
		t.Fatal(err)
	}
	if AutostartEnabled() {
		t.Error("AutostartEnabled reports true after SetAutostart(false)")
	}
}

// Under Flatpak the launch is `flatpak run <id>`, which is three words and
// must stay three words — quoting it as one would ask the session to run a
// program literally named "flatpak run org.example.App".
func TestAutostartEntryUnderFlatpakIsNotOverQuoted(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FLATPAK_ID", "org.example.OpenSave")

	if err := SetAutostart(true); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(home, ".config", "autostart", "opensave.desktop"))
	if err != nil {
		t.Fatal(err)
	}
	exec := execLine(t, string(raw))
	want := "Exec=flatpak run org.example.OpenSave " + StartHiddenFlag
	if exec != want {
		t.Errorf("Exec line:\n  got  %s\n  want %s", exec, want)
	}
}

func execLine(t *testing.T, entry string) string {
	t.Helper()
	for _, line := range strings.Split(entry, "\n") {
		if strings.HasPrefix(line, "Exec=") {
			return line
		}
	}
	t.Fatalf("no Exec= line in:\n%s", entry)
	return ""
}
