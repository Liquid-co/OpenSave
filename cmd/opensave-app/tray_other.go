//go:build !windows && !linux

package main

import (
	"context"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// No tray on this platform (macOS would need main-thread NSApplication
// integration). Closing the window quits normally.
func (a *App) startTray() {}
func (a *App) stopTray()  {}

func (a *App) beforeClose(ctx context.Context) bool { return false }

// showIfTrayNeverAppears: there is no tray here to wait for, so a hidden
// start is shown at once rather than left unreachable.
func (a *App) showIfTrayNeverAppears() { wailsruntime.WindowShow(a.ctx) }
