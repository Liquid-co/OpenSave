//go:build !windows

package cliapp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const installedName = "opensave"

const pathActivationHint = "Open a new terminal, or run `hash -r`, and `opensave` will resolve."

// defaultInstallDir follows the XDG user-binary convention. ~/.local/bin is
// already on PATH on most desktop distributions (and on SteamOS), so the
// common case needs no shell-config edit at all.
func defaultInstallDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot find your home directory: %w", err)
	}
	return filepath.Join(home, ".local", "bin"), nil
}

func normalizePathEntry(p string) string {
	return strings.TrimRight(strings.TrimSpace(p), "/")
}

// writeAliases links `os` and `opensave-cli` to the installed binary.
// Symlinks are free here, unlike on Windows.
// aliasNames are the short spellings installed beside the binary. They are
// symlinks here, so they carry no suffix.
var aliasNames = []string{"os", "opensave-cli"}

const aliasSuffix = ""

// aliasPointsAtUs reports whether a symlink still points at our binary. A
// program of somebody else's called `os` is not ours to delete, and on a
// shared ~/.local/bin that is not a hypothetical.
func aliasPointsAtUs(alias, dir string) bool {
	target, err := os.Readlink(alias)
	if err != nil {
		return false
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(alias), target)
	}
	return filepath.Clean(target) == filepath.Join(dir, installedName)
}

// removeInstalledBinary deletes the installed copy. Unix unlinks a running
// binary happily, so there is nothing to work around.
func removeInstalledBinary(installed string) error {
	return os.Remove(installed)
}

func writeAliases(dir string) (written, skipped []string) {
	for _, alias := range aliasNames {
		link := filepath.Join(dir, alias+aliasSuffix)
		if _, err := os.Lstat(link); err == nil {
			// Somebody else's `os` is not ours to replace. This removed
			// whatever was there, while install.sh and the README promised
			// to leave it alone.
			if !aliasPointsAtUs(link, dir) {
				skipped = append(skipped, link)
				continue
			}
			_ = os.Remove(link)
		}
		if err := os.Symlink(filepath.Join(dir, installedName), link); err == nil {
			written = append(written, link)
		}
	}
	return written, skipped
}

// shellProfiles lists the startup files worth appending a PATH line to.
// Only files that already exist are touched: creating ~/.zshrc on a bash
// system, or a profile for a shell the user does not have, leaves clutter
// behind that nothing ever reads.
func shellProfiles() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	var out []string
	for _, name := range []string{".bashrc", ".zshrc", ".profile"} {
		p := filepath.Join(home, name)
		if _, err := os.Stat(p); err == nil {
			out = append(out, p)
		}
	}
	return out
}

// pathLineFor is the line ensureOnPath appends, and the one removeFromPath
// looks for. Defined once so the writer and the remover cannot drift.
func pathLineFor(dir string) string {
	return fmt.Sprintf("export PATH=\"%s:$PATH\"", dir)
}

const pathMarker = "# added by opensave install"

// removeFromPath strips the line ensureOnPath added from the shell profiles
// that carry it, reporting whether anything changed.
//
// Only the exact line this tool wrote, with its marker comment if that sits
// directly above it. A profile is a file the person edits by hand, and an
// uninstaller that rewrites more of it than it wrote is worse than one that
// leaves a line behind.
func removeFromPath(dir string) (bool, error) {
	line := pathLineFor(dir)
	changed := false
	for _, p := range shellProfiles() {
		raw, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		lines := strings.Split(string(raw), "\n")
		kept := make([]string, 0, len(lines))
		for i := 0; i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) == line {
				// Drop the marker comment above it too, and the blank line
				// that separated the pair from what came before.
				if n := len(kept); n > 0 && strings.TrimSpace(kept[n-1]) == pathMarker {
					kept = kept[:n-1]
					if n := len(kept); n > 0 && strings.TrimSpace(kept[n-1]) == "" {
						kept = kept[:n-1]
					}
				}
				changed = true
				continue
			}
			kept = append(kept, lines[i])
		}
		if !changed {
			continue
		}
		if err := os.WriteFile(p, []byte(strings.Join(kept, "\n")), 0o644); err != nil {
			return changed, fmt.Errorf("update %s: %w", p, err)
		}
	}
	return changed, nil
}

// ensureOnPath appends a PATH line to the user's shell profiles when the
// directory isn't already reachable.
func ensureOnPath(dir string) (bool, error) {
	if pathContains(os.Getenv("PATH"), dir) {
		return false, nil
	}

	profiles := shellProfiles()
	if len(profiles) == 0 {
		return false, fmt.Errorf("found no shell profile to update (looked for ~/.bashrc, ~/.zshrc, ~/.profile)")
	}

	line := pathLineFor(dir)
	marker := pathMarker
	var wrote bool
	for _, p := range profiles {
		existing, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		// Don't stack a duplicate export on every re-run.
		if strings.Contains(string(existing), line) {
			continue
		}
		f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			continue
		}
		_, err = fmt.Fprintf(f, "\n%s\n%s\n", marker, line)
		f.Close()
		if err == nil {
			wrote = true
		}
	}
	if !wrote {
		// Already present in every profile: PATH is configured, this shell
		// just hasn't re-read it.
		return false, nil
	}

	_ = os.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return true, nil
}
