//go:build windows

package main

import "testing"

func TestUserIsBusyReadsWindowsNotificationState(t *testing.T) {
	defer func(orig func() (uint32, bool)) { notificationState = orig }(notificationState)

	for state, want := range map[uint32]bool{
		1: false, // QUNS_NOT_PRESENT: locked or screen saver — nobody to disturb, nothing to take over
		2: true,  // QUNS_BUSY
		3: true,  // QUNS_RUNNING_D3D_FULL_SCREEN
		4: true,  // QUNS_PRESENTATION_MODE
		5: false, // QUNS_ACCEPTS_NOTIFICATIONS
		6: false, // QUNS_QUIET_TIME
		7: true,  // QUNS_APP
	} {
		s := state
		notificationState = func() (uint32, bool) { return s, true }
		if got := userIsBusy(); got != want {
			t.Errorf("state %d: busy = %v, want %v", state, got, want)
		}
	}

	notificationState = func() (uint32, bool) { return 3, false }
	if userIsBusy() {
		t.Error("an unanswered query counted as busy")
	}
}

// The real call, on this machine: it must answer without failing. What it
// answers depends on the screen, so only that it answers is checked.
func TestTheRealNotificationStateAnswers(t *testing.T) {
	state, ok := notificationState()
	if !ok {
		t.Skip("SHQueryUserNotificationState is not available here")
	}
	if state < 1 || state > 7 {
		t.Errorf("state %d is outside the documented range", state)
	}
}
