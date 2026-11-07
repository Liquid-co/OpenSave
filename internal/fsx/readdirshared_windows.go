package fsx

import (
	"os"
	"syscall"
)

// readDirShared lists the folder through a handle opened with
// FILE_SHARE_DELETE (FILE_FLAG_BACKUP_SEMANTICS is what opens a folder). Any
// failure to open it so falls back to os.ReadDir, as openShared does, and for
// the same reason: os.ReadDir's path handling is not to be second-guessed.
func readDirShared(path string) ([]os.DirEntry, error) {
	f, err := openDirShared(path)
	if err != nil {
		return os.ReadDir(path)
	}
	defer f.Close()
	return f.ReadDir(-1)
}

func openDirShared(path string) (*os.File, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := syscall.CreateFile(
		p,
		syscall.GENERIC_READ,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil,
		syscall.OPEN_EXISTING,
		syscall.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(h), path), nil
}
