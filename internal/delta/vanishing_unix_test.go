//go:build !windows

package delta

import (
	"os"
	"testing"
)

// makeUnreadable removes read permission, which a non-root user cannot get
// past. Returns false when that would not work (running as root).
func makeUnreadable(t *testing.T, path string) (func(), bool) {
	t.Helper()
	if os.Geteuid() == 0 {
		return nil, false
	}
	if err := os.Chmod(path, 0o000); err != nil {
		return nil, false
	}
	return func() { _ = os.Chmod(path, 0o666) }, true
}
