//go:build !windows

package winreg

// A device without a registry still carries captured keys through its
// snapshots: a Linux machine syncing with a Windows one stores what Windows
// captured and hands it back on restore. It simply has nothing of its own to
// read or write, which is why these report ErrUnsupported rather than failing
// silently — a caller that wanted a capture needs to know it did not get one.

func available() bool { return false }

func capture(path string) (Key, bool, error) { return Key{}, false, ErrUnsupported }

func restore(k Key) error { return ErrUnsupported }
