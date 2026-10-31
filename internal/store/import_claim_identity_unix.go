//go:build !windows

package store

import (
	"fmt"
	"os"
	"syscall"

	"github.com/opensave/opensave/internal/fsx"
)

// Unix DB inode identity is stable across hardlinks.
func nativeDBFileIdentity(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", fmt.Errorf("unsupported DB file identity on this platform")
	}
	return fmt.Sprintf("unix:%d:%d", st.Dev, st.Ino), nil
}

// flock on the database inode is independent of SQLite's fcntl range locks,
// and every hardlink of that file contends for the same flock.
func claimNativeDB(path string) (func(), error) {
	release, ok, err := fsx.TryLock(path)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("%w: another import or registration is active", ErrGamePathOwned)
	}
	return release, nil
}
