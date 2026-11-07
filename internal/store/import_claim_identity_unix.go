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

// Never open/close the SQLite inode for our lock: closing ANY descriptor for
// that inode can discard SQLite's POSIX locks in this process. Keep the sidecar
// in place after release so concurrent callers always lock the same inode.
func claimNativeDB(path string) (func(), error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil, fmt.Errorf("unsupported DB file identity on this platform")
	}
	if st.Nlink != 1 {
		return nil, fmt.Errorf("hard-linked SQLite databases are unsupported; use one canonical database path")
	}
	release, ok, err := fsx.TryLock(path + ".path-claim.lock")
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrPathClaimBusy
	}
	return release, nil
}
