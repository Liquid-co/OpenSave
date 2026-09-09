package delta

import (
	"os"
	"syscall"
	"testing"
)

// makeUnreadable holds the file the way a game holds its save while writing:
// open with no sharing at all, so nothing else can read it.
func makeUnreadable(t *testing.T, path string) (func(), bool) {
	t.Helper()
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, false
	}
	h, err := syscall.CreateFile(p, syscall.GENERIC_READ|syscall.GENERIC_WRITE,
		0, // no FILE_SHARE_* at all: exclusive
		nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, false
	}
	f := os.NewFile(uintptr(h), path)
	// Confirm it really is unreadable, so the test cannot pass vacuously.
	if probe, probeErr := os.Open(path); probeErr == nil {
		probe.Close()
		f.Close()
		return nil, false
	}
	return func() { f.Close() }, true
}
