//go:build !windows

package e2e

import "os"

// makeDirLink creates a directory symlink, the Unix equivalent of the
// junction the Windows build uses.
func makeDirLink(link, target string) error {
	return os.Symlink(target, link)
}
