//go:build !windows

package main

// userIsBusy has no reliable answer outside Windows — no desktop-wide "a
// full-screen game has the screen" signal that works across Linux desktops
// and macOS alike — so it never holds anything back there.
func userIsBusy() bool { return false }
