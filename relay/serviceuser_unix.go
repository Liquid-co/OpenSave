//go:build !windows

package relay

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
	"syscall"
)

// chownTo gives path to the named account, reporting whether anything changed.
//
// A no-op when the file already belongs to them, so re-running setup does not
// claim to have fixed something it did not touch.
func chownTo(path, username string) (changed bool, err error) {
	u, err := user.Lookup(username)
	if err != nil {
		return false, fmt.Errorf("look up service account %q: %w", username, err)
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return false, fmt.Errorf("service account %q has a non-numeric uid %q", username, u.Uid)
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		gid = -1
	}

	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	if st, ok := info.Sys().(*syscall.Stat_t); ok && int(st.Uid) == uid {
		return false, nil
	}
	if err := os.Chown(path, uid, gid); err != nil {
		return false, fmt.Errorf("hand %s to %q: %w", path, username, err)
	}
	return true, nil
}

// FileOwner names the account owning path, or "" when that cannot be
// determined. Used to explain a mismatch rather than merely assert one.
func FileOwner(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return ""
	}
	if u, err := user.LookupId(strconv.Itoa(int(st.Uid))); err == nil {
		return u.Username
	}
	return strconv.Itoa(int(st.Uid))
}
