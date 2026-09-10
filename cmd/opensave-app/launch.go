package main

import (
	"os"

	"github.com/opensave/opensave/internal/sysintegration"
)

// launchedHidden reports whether this process was asked to start without
// showing its window.
//
// Set by the autostart entry and by nothing else: a person double-clicking the
// icon wants to see the app, and a boot-time launch wants the opposite. Read
// straight from os.Args rather than the flag package, because Wails owns the
// process's argument handling and a second parser would fight it.
func launchedHidden() bool {
	for _, arg := range os.Args[1:] {
		if arg == sysintegration.StartHiddenFlag {
			return true
		}
	}
	return false
}
