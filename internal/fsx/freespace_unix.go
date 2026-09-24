//go:build !windows

package fsx

import "golang.org/x/sys/unix"

// FreeBytes returns the free space (available to unprivileged
// users) on the filesystem holding dir. ok is false when it can't be
// determined, in which case callers skip the free-space check.
func FreeBytes(dir string) (bytes uint64, ok bool) {
	var st unix.Statfs_t
	if err := unix.Statfs(dir, &st); err != nil {
		return 0, false
	}
	return st.Bavail * uint64(st.Bsize), true
}
