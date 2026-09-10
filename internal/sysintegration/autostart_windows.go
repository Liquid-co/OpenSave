//go:build windows

// Package sysintegration handles OS-level integration: start-on-boot
// registration and (on Windows) the system tray.
package sysintegration

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows/registry"
)

const runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`
const runValueName = "OpenSave"

// SetAutostart registers or removes OpenSave in the current user's Run
// key (no admin rights needed).
func SetAutostart(enabled bool) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return fmt.Errorf("open Run key: %w", err)
	}
	defer key.Close()

	if !enabled {
		err := key.DeleteValue(runValueName)
		if err != nil && err != registry.ErrNotExist {
			return fmt.Errorf("remove autostart entry: %w", err)
		}
		return nil
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}
	// Launched hidden: at boot the app belongs in the tray, not over
	// whatever the person sat down to do. See StartHiddenFlag.
	want := `"` + exe + `" ` + StartHiddenFlag
	// Skipped when already right. This is also called at every startup to
	// repair an entry written before the flag existed, or one pointing at a
	// binary that has since moved, and a registry write per launch for a
	// value that has not changed is noise in anyone's audit log.
	if current, _, readErr := key.GetStringValue(runValueName); readErr == nil && current == want {
		return nil
	}
	if err := key.SetStringValue(runValueName, want); err != nil {
		return fmt.Errorf("set autostart entry: %w", err)
	}
	return nil
}

// AutostartEnabled reports whether the Run entry exists.
func AutostartEnabled() bool {
	key, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()
	_, _, err = key.GetStringValue(runValueName)
	return err == nil
}
