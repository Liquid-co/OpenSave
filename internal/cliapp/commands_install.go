package cliapp

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"
)

// cmdInstall copies this binary somewhere permanent and puts that directory
// on the user's PATH, so `opensave` works from any new terminal.
//
// install.ps1 and install.sh already do this for people who pipe the
// installer to a shell. The release also publishes the bare binary, though,
// and someone who downloads that gets a loose executable with nothing to
// wire it up — they have to know to move it somewhere and edit PATH by hand.
// This closes that gap from the binary itself, so however it was obtained,
// one command finishes the job.
//
// Deliberately a command rather than something the binary does on first run.
// Editing PATH is a change to the user's environment that outlives the
// process; a tool that does that because you merely ran it once is a tool
// that cannot be tried out.
func cmdInstall(args []string) int {
	dir := ""
	uninstall, assumeYes := false, false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--uninstall":
			uninstall = true
		case "--yes", "-y":
			assumeYes = true
		case "--dir":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --dir needs a directory")
				return 1
			}
			dir = args[i+1]
			i++
		case "--help", "-h":
			fmt.Println("Usage: opensave install [--dir <directory>]")
			fmt.Println("       opensave install --uninstall [--dir <directory>] [--yes]")
			fmt.Println()
			fmt.Println("Copies this binary to a permanent location and adds it to your PATH,")
			fmt.Println("so `opensave` works from any terminal.")
			fmt.Println()
			fmt.Println("  --dir <directory>   install here instead of the default")
			fmt.Println("  --uninstall         remove it again, and take it back off PATH")
			fmt.Println("  --yes               do not ask (for scripts and the app's uninstaller)")
			fmt.Println()
			fmt.Println("Uninstalling removes only this tool. Your games, snapshots and")
			fmt.Println("settings live elsewhere and are never touched.")
			return 0
		default:
			fmt.Fprintf(os.Stderr, "error: unknown option %q\n", args[i])
			return 1
		}
	}

	if uninstall {
		return uninstallCLI(dir, assumeYes)
	}

	if dir == "" {
		d, err := defaultInstallDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		dir = d
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	dir = abs

	self, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot locate this binary: %v\n", err)
		return 1
	}
	if resolved, err := filepath.EvalSymlinks(self); err == nil {
		self = resolved
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	dest := filepath.Join(dir, installedName)

	// Running from the destination already (a re-run of `opensave install`,
	// or an installed copy): copying a file onto itself truncates it. Skip
	// the copy and go straight to repairing PATH, which is the part that is
	// actually worth re-running.
	if sameFile(self, dest) {
		fmt.Printf("Already installed at %s\n", dest)
	} else {
		if err := copyExecutable(self, dest); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		fmt.Printf("Installed %s\n", dest)
	}

	written, skipped := writeAliases(dir)
	for _, line := range written {
		fmt.Printf("  %s\n", line)
	}
	for _, p := range skipped {
		fmt.Printf("  %s already exists and is not OpenSave's - left alone\n", p)
	}

	added, err := ensureOnPath(dir)
	if err != nil {
		// The binary is in place; only PATH failed. Say what to do by hand
		// rather than pretending the whole install failed.
		fmt.Fprintf(os.Stderr, "\nwarning: could not update PATH: %v\n", err)
		fmt.Fprintf(os.Stderr, "Add this directory to your PATH manually:\n  %s\n", dir)
		return 1
	}
	fmt.Println()
	if added {
		fmt.Printf("Added %s to your PATH.\n", dir)
		fmt.Println(pathActivationHint)
	} else {
		fmt.Printf("%s was already on your PATH.\n", dir)
	}
	fmt.Println()
	fmt.Println("Then: opensave scan")
	return 0
}

// uninstallCLI removes what cmdInstall put down: the binary, the aliases
// beside it, and the PATH entry.
//
// It is what the Windows app's uninstaller calls, and the counterpart of
// `install.sh --uninstall` on Linux. Three things it will not do. It does not
// touch ~/.opensave — the tool is not the data, and someone removing a
// command-line front end has not asked to lose their snapshots. It does not
// delete a directory it did not create: the default is a folder of our own,
// but --dir can point anywhere and ~/.local/bin holds other people's
// programs. And it removes an alias only when that alias is still ours.
func uninstallCLI(dir string, assumeYes bool) int {
	ownDir := dir == ""
	if dir == "" {
		d, err := defaultInstallDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		dir = d
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	dir = abs

	installed := filepath.Join(dir, installedName)
	if _, statErr := os.Stat(installed); statErr != nil {
		fmt.Printf("Nothing to remove: no %s in %s\n", installedName, dir)
		// A stale PATH entry outlives a binary deleted by hand, so it is
		// still worth taking off.
		if changed, pathErr := removeFromPath(dir); pathErr == nil && changed {
			fmt.Printf("Removed %s from your PATH.\n", dir)
		}
		return 0
	}

	if !assumeYes {
		if !stdinIsTerminal() {
			// Nobody to ask and no --yes. Refused, and as a failure: exiting
			// 0 here told a script its uninstall had worked when nothing had
			// been removed at all.
			fmt.Fprintln(os.Stderr, "error: refusing to uninstall without --yes when there is no terminal to ask")
			return 1
		}
		if !confirmRemoval(installed, dir) {
			// A person said no. That is an answer, not a failure.
			fmt.Println("Left alone.")
			return 0
		}
	}

	if err := removeInstalledBinary(installed); err != nil {
		fmt.Fprintf(os.Stderr, "error: could not remove %s: %v\n", installed, err)
		return 1
	}
	fmt.Printf("Removed %s\n", installed)

	for _, alias := range aliasPaths(dir) {
		if !aliasPointsAtUs(alias, dir) {
			continue
		}
		if err := os.Remove(alias); err == nil {
			fmt.Printf("Removed %s\n", alias)
		}
	}

	if changed, err := removeFromPath(dir); err != nil {
		fmt.Fprintf(os.Stderr, "\nwarning: could not update PATH: %v\n", err)
		fmt.Fprintf(os.Stderr, "Remove this directory from your PATH by hand:\n  %s\n", dir)
		return 1
	} else if changed {
		fmt.Printf("Removed %s from your PATH.\n", dir)
	}

	// Only a directory we chose, and only once it is empty.
	if ownDir {
		if err := os.Remove(dir); err == nil {
			fmt.Printf("Removed %s\n", dir)
		}
	}

	fmt.Println()
	fmt.Println("Your games, snapshots and settings were not touched.")
	return 0
}

// confirmRemoval asks the person at the terminal. The caller has already
// made sure there is one.
func confirmRemoval(installed, dir string) bool {
	fmt.Printf("Remove %s and take %s off your PATH? [y/N] ", installed, dir)
	var answer string
	_, _ = fmt.Scanln(&answer)
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes"
}

// stdinIsTerminal reports whether there is a person to ask.
//
// A real terminal check, not "is it a character device": the null device is
// one too, on Windows and Unix alike, so stdin redirected from it passed as a
// console, the prompt read nothing, and a script without --yes was told it
// had been "Left alone" with exit 0.
func stdinIsTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// aliasPaths lists the short names writeAliases drops beside the binary.
// One list, so the writer and the remover cannot drift apart.
func aliasPaths(dir string) []string {
	var out []string
	for _, alias := range aliasNames {
		out = append(out, filepath.Join(dir, alias+aliasSuffix))
	}
	return out
}

// sameFile reports whether two paths are the same file on disk, so a re-run
// doesn't copy a binary over itself.
func sameFile(a, b string) bool {
	ai, err := os.Stat(a)
	if err != nil {
		return false
	}
	bi, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(ai, bi)
}

// copyExecutable writes src to dst with the executable bit set, moving any
// existing copy aside first: Windows refuses to overwrite a running binary,
// and on Unix writing into a file another process is executing gives it a
// corrupted image mid-run.
func copyExecutable(src, dst string) error {
	if _, err := os.Stat(dst); err == nil {
		old := dst + ".old"
		_ = os.Remove(old)
		if err := os.Rename(dst, old); err != nil {
			return fmt.Errorf("%s is in use — close it and try again", filepath.Base(dst))
		}
		// Best effort: on Windows this fails while the old copy is still
		// running, and the next install cleans it up.
		defer func() { _ = os.Remove(old) }()
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chmod(dst, 0o755)
}

// pathEntries splits a PATH value into its entries, dropping empties.
func pathEntries(path string) []string {
	var out []string
	for _, p := range strings.Split(path, string(os.PathListSeparator)) {
		if strings.TrimSpace(p) != "" {
			out = append(out, p)
		}
	}
	return out
}

// pathContains reports whether dir is already among a PATH value's entries.
// Compares whole entries rather than substrings: a plain `strings.Contains`
// matches "C:\tools\opensave" inside "C:\tools\opensave-old" and then skips
// an update the user needed.
func pathContains(path, dir string) bool {
	target := normalizePathEntry(dir)
	for _, p := range pathEntries(path) {
		if normalizePathEntry(p) == target {
			return true
		}
	}
	return false
}
