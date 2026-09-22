//go:build windows

package cliapp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const installedName = "opensave.exe"

const pathActivationHint = "Open a new PowerShell window and run `opensave`."

func defaultInstallDir() (string, error) {
	local := os.Getenv("LOCALAPPDATA")
	if local == "" {
		return "", fmt.Errorf("LOCALAPPDATA is not set — pass --dir to choose a location")
	}
	return filepath.Join(local, "OpenSave", "bin"), nil
}

// normalizePathEntry makes two spellings of the same directory compare equal:
// Windows paths are case-insensitive and a trailing separator is meaningless.
func normalizePathEntry(p string) string {
	return strings.ToLower(strings.TrimRight(strings.TrimSpace(p), `\/`))
}

// aliasNames are the short spellings installed beside the binary, and
// aliasSuffix is what they are called on this platform.
var aliasNames = []string{"os", "opensave-cli"}

const aliasSuffix = ".cmd"

// aliasPointsAtUs reports whether a shim is still one of ours — it names our
// binary and nothing else. An `os.cmd` somebody else wrote, or edited since,
// is theirs, and an uninstaller that deletes a file because it recognises
// the NAME has not checked anything at all.
func aliasPointsAtUs(alias, dir string) bool {
	body, err := os.ReadFile(alias)
	if err != nil {
		return false
	}
	return strings.Contains(string(body), installedName)
}

// removeInstalledBinary deletes the installed copy.
//
// On Windows the file being removed may be the very process doing the
// removing — `opensave install --uninstall` run from the installed copy —
// and Windows will not delete a running image. Renaming it is allowed, so
// the copy is moved aside and marked for deletion at the next reboot; the
// name it occupied is free immediately, which is what a reinstall needs.
func removeInstalledBinary(installed string) error {
	if err := os.Remove(installed); err == nil {
		return nil
	}
	aside := installed + ".old"
	_ = os.Remove(aside)
	if err := os.Rename(installed, aside); err != nil {
		return err
	}
	if p, err := windows.UTF16PtrFromString(aside); err == nil {
		// MOVEFILE_DELAY_UNTIL_REBOOT, with no destination: delete on boot.
		_ = windows.MoveFileEx(p, nil, windows.MOVEFILE_DELAY_UNTIL_REBOOT)
	}
	return nil
}

// writeAliases drops `os` and `opensave-cli` next to the binary as .cmd
// shims. Shims rather than copies of a 15 MB binary, and rather than
// symlinks, which need admin rights or Developer Mode.
func writeAliases(dir string) []string {
	var out []string
	for _, alias := range aliasNames {
		shim := filepath.Join(dir, alias+aliasSuffix)
		body := "@echo off\r\n\"%~dp0" + installedName + "\" %*\r\n"
		if err := os.WriteFile(shim, []byte(body), 0o755); err == nil {
			out = append(out, shim)
		}
	}
	return out
}

// nextPathValue returns the PATH value that should replace current once dir
// is on it, or "" when dir is already there and nothing needs writing.
//
// Split out from the registry I/O so the part that can damage a user's PATH
// is testable without touching HKCU.
func nextPathValue(current, dir string) string {
	if pathContains(current, dir) {
		return ""
	}
	if strings.TrimSpace(current) == "" {
		return dir
	}
	return strings.TrimRight(current, ";") + ";" + dir
}

// pathWithout returns the PATH value with dir removed, or "" when dir is not
// on it and nothing needs writing.
//
// Split out for the same reason nextPathValue is: this is the half that can
// damage a PATH, and it must be testable without touching HKCU. Entries are
// compared the way pathContains compares them — case-insensitively, ignoring
// a trailing separator — so the spelling the user has is the one that is
// matched, and every other entry is written back exactly as it was found,
// %VAR% references included.
func pathWithout(current, dir string) string {
	if !pathContains(current, dir) {
		return ""
	}
	want := normalizePathEntry(dir)
	kept := make([]string, 0, 8)
	for _, entry := range strings.Split(current, ";") {
		if strings.TrimSpace(entry) == "" {
			continue
		}
		if normalizePathEntry(entry) == want {
			continue
		}
		kept = append(kept, entry)
	}
	return strings.Join(kept, ";")
}

// removeFromPath takes dir back off the user's PATH, reporting whether it
// changed anything. The mirror of ensureOnPath, and it preserves the value
// type for the same reason.
func removeFromPath(dir string) (bool, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return false, fmt.Errorf("open HKCU\\Environment: %w", err)
	}
	defer key.Close()

	current, valType, err := key.GetStringValue("Path")
	if err == registry.ErrNotExist {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read PATH: %w", err)
	}
	updated := pathWithout(current, dir)
	if updated == "" && !pathContains(current, dir) {
		return false, nil
	}

	switch valType {
	case registry.EXPAND_SZ:
		err = key.SetExpandStringValue("Path", updated)
	default:
		err = key.SetStringValue("Path", updated)
	}
	if err != nil {
		return false, fmt.Errorf("write PATH: %w", err)
	}
	broadcastEnvironmentChange()
	return true, nil
}

// ensureOnPath adds dir to the user's PATH, reporting whether it changed
// anything.
//
// Writes HKCU\Environment directly instead of going through
// SetEnvironmentVariable. That API returns the *expanded* value on read and
// writes back a plain REG_SZ, so a PATH containing %USERPROFILE% or
// %JAVA_HOME% gets those references silently baked into whatever they
// happened to resolve to at that moment — and then stops tracking the
// variable. Preserving the original value type is the difference between
// adding one entry and quietly rewriting the user's entire PATH.
func ensureOnPath(dir string) (bool, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return false, fmt.Errorf("open HKCU\\Environment: %w", err)
	}
	defer key.Close()

	current, valType, err := key.GetStringValue("Path")
	if err != nil && err != registry.ErrNotExist {
		return false, fmt.Errorf("read PATH: %w", err)
	}
	if err == registry.ErrNotExist {
		// No user PATH at all yet; REG_EXPAND_SZ is what Windows uses.
		valType = registry.EXPAND_SZ
		current = ""
	}
	updated := nextPathValue(current, dir)
	if updated == "" {
		return false, nil
	}

	// Write back with the type it already had, so unexpanded references keep
	// working. GetStringValue does not expand for us, so `current` still
	// holds the literal %VAR% text and round-trips intact.
	switch valType {
	case registry.EXPAND_SZ:
		err = key.SetExpandStringValue("Path", updated)
	default:
		err = key.SetStringValue("Path", updated)
	}
	if err != nil {
		return false, fmt.Errorf("write PATH: %w", err)
	}

	broadcastEnvironmentChange()

	// Also update this process, so a `opensave` invoked from the very shell
	// that ran the install resolves without reopening it.
	_ = os.Setenv("PATH", os.Getenv("PATH")+string(os.PathListSeparator)+dir)
	return true, nil
}

// broadcastEnvironmentChange tells running programs the environment moved.
// Without it, Explorer (and anything it launches afterwards) keeps handing
// out the old PATH until the next sign-out. Already-open consoles never
// update regardless — they copied the environment when they started — which
// is why the hint still says to open a new window.
func broadcastEnvironmentChange() {
	const (
		hwndBroadcast   = 0xFFFF
		wmSettingChange = 0x001A
		smtoAbortIfHung = 0x0002
	)
	env, err := windows.UTF16PtrFromString("Environment")
	if err != nil {
		return
	}
	user32 := windows.NewLazySystemDLL("user32.dll")
	proc := user32.NewProc("SendMessageTimeoutW")
	if err := proc.Find(); err != nil {
		return
	}
	var result uintptr
	// Best effort: a hung top-level window must not wedge the installer, so
	// this aborts rather than waiting on one.
	_, _, _ = proc.Call(
		uintptr(hwndBroadcast), uintptr(wmSettingChange), 0,
		uintptr(unsafe.Pointer(env)),
		uintptr(smtoAbortIfHung), uintptr(5000),
		uintptr(unsafe.Pointer(&result)),
	)
}
