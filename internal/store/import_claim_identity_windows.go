//go:build windows

package store

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// A Windows volume/file index remains stable across NTFS hardlinks.
func nativeDBFileIdentity(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(f.Fd()), &info); err != nil {
		return "", err
	}
	return fmt.Sprintf("windows:%d:%d:%d", info.VolumeSerialNumber, info.FileIndexHigh, info.FileIndexLow), nil
}

// A named mutex keyed by the file index contends across hardlink spellings
// without locking a byte SQLite itself uses.
func claimNativeDB(path string) (func(), error) {
	identity, err := nativeDBFileIdentity(path)
	if err != nil {
		return nil, err
	}
	name, err := windows.UTF16PtrFromString(`Local\opensave-path-claim-` + identity)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateMutex(nil, false, name)
	if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return nil, err
	}
	wait, waitErr := windows.WaitForSingleObject(handle, 0)
	if waitErr != nil {
		windows.CloseHandle(handle)
		return nil, waitErr
	}
	if wait != windows.WAIT_OBJECT_0 {
		windows.CloseHandle(handle)
		return nil, fmt.Errorf("%w: another import or registration is active", ErrGamePathOwned)
	}
	return func() {
		windows.ReleaseMutex(handle)
		windows.CloseHandle(handle)
	}, nil
}
