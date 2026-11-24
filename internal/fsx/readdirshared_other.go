//go:build !windows

package fsx

import "os"

// On Unix an open folder never blocks removing it or anything in it.
func readDirShared(path string) ([]os.DirEntry, error) {
	return os.ReadDir(path)
}
