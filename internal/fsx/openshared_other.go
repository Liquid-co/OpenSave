//go:build !windows

package fsx

import "os"

// On Unix an open descriptor keeps the inode alive but never blocks the
// directory operations, so the standard open already has the property this
// package is about.
func openShared(path string) (*os.File, error) {
	return os.Open(path)
}
