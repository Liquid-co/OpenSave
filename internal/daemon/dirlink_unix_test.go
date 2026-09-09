//go:build !windows

package daemon

import "os"

func makeDirLink(link, target string) error { return os.Symlink(target, link) }
