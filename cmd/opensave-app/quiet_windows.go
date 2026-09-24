//go:build windows

package main

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// Whether now is a bad moment to put a window in front of someone.
//
// Windows keeps this answer itself: SHQueryUserNotificationState is what it
// consults before showing its own notifications, and it says "busy" while a
// full-screen game or presentation has the screen. Asking it beats guessing
// from our own side — a game's save file being open is a poor sign that the
// game is on screen, and many games do not hold theirs open at all.

var procQueryNotificationState = windows.NewLazySystemDLL("shell32.dll").NewProc("SHQueryUserNotificationState")

// QUERY_USER_NOTIFICATION_STATE values that mean the screen is taken.
const (
	qunsBusy             = 2 // a full-screen application
	qunsRunningD3DFull   = 3 // a full-screen Direct3D application: most games
	qunsPresentationMode = 4 // presentation settings are on
	qunsApp              = 7 // a full-screen Windows Store app
)

// notificationState is the raw answer, or ok=false when Windows would not say.
var notificationState = func() (state uint32, ok bool) {
	if err := procQueryNotificationState.Find(); err != nil {
		return 0, false
	}
	hr, _, _ := procQueryNotificationState.Call(uintptr(unsafe.Pointer(&state)))
	return state, hr == 0
}

// userIsBusy reports whether a full-screen game, presentation or app has the
// screen. Not knowing counts as not busy: silence is the wrong default for
// something someone asked to be told about.
func userIsBusy() bool {
	state, ok := notificationState()
	if !ok {
		return false
	}
	switch state {
	case qunsBusy, qunsRunningD3DFull, qunsPresentationMode, qunsApp:
		return true
	}
	return false
}
