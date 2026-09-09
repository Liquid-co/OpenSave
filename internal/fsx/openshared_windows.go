package fsx

import (
	"os"
	"syscall"
)

// openShared is os.Open plus FILE_SHARE_DELETE.
//
// CreateFileW directly, because the share mode is fixed inside syscall.Open
// and no flag on os.OpenFile reaches it.
//
// On any failure this falls back to os.Open rather than reporting the
// CreateFileW error. That is not laziness about error handling — it is what
// keeps the change strictly non-regressive. os.Open carries long-path fixups
// and name resolution that a hand-rolled CreateFileW does not, and getting
// those subtly wrong would break opening a save rather than merely failing to
// improve it. A first attempt at this prefixed long paths with \?\ by hand
// and broke every path containing an 8.3 short name, because \?\ bypasses the
// name resolution that expands them. The fallback costs one failed syscall on
// a path that was going to fail anyway, and the error the caller sees is
// os.Open's, exactly as before.
func openShared(path string) (*os.File, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return os.Open(path)
	}
	h, err := syscall.CreateFile(
		p,
		syscall.GENERIC_READ,
		// The third flag is the point of this whole file.
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil,
		syscall.OPEN_EXISTING,
		syscall.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return os.Open(path)
	}
	return os.NewFile(uintptr(h), path), nil
}
