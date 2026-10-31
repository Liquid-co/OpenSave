//go:build windows

package store

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"

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

// claimNativeDB holds one named Windows mutex for the entire native ownership
// admission. Windows mutex ownership belongs to an OS *thread*, not a Go
// goroutine. Without pinning the thread, a second Go caller can acquire the
// same mutex recursively and bypass path admission, and releasing from a
// different thread can silently fail.
//
// A dedicated pinned goroutine owns acquisition and ReleaseMutex. Callers can
// release from any goroutine and return only after the native lock is released.
// This is nonblocking: concurrent admissions receive ErrGamePathOwned.
func claimNativeDB(path string) (func(), error) {
	identity, err := nativeDBFileIdentity(path)
	if err != nil {
		return nil, err
	}
	name, err := windows.UTF16PtrFromString(`Local\opensave-path-claim-` + identity)
	if err != nil {
		return nil, err
	}

	ready := make(chan error, 1)
	stop := make(chan struct{})
	released := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		handle, createErr := windows.CreateMutex(nil, false, name)
		if createErr != nil && !errors.Is(createErr, windows.ERROR_ALREADY_EXISTS) {
			ready <- createErr
			return
		}
		if handle == 0 {
			ready <- fmt.Errorf("create native path-claim mutex: missing handle")
			return
		}
		defer windows.CloseHandle(handle)

		status, waitErr := windows.WaitForSingleObject(handle, 0)
		if waitErr != nil {
			ready <- waitErr
			return
		}
		switch status {
		case windows.WAIT_OBJECT_0:
			// The OS thread is pinned and cannot run another Go caller,
			// preventing recursive acquisition by an unrelated goroutine.
		case windows.WAIT_ABANDONED:
			// An abandoned mutex has been acquired by this thread, but the
			// previous owner died while possibly mutating an import. Refuse
			// to treat that state as an ordinary safe admission.
			_ = windows.ReleaseMutex(handle)
			ready <- fmt.Errorf("%w: previous import claim was abandoned", ErrGamePathOwned)
			return
		default:
			ready <- fmt.Errorf("%w: another import or registration is active", ErrGamePathOwned)
			return
		}

		ready <- nil
		<-stop
		_ = windows.ReleaseMutex(handle)
		close(released)
	}()

	if err := <-ready; err != nil {
		return nil, err
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			close(stop)
			<-released
		})
	}, nil
}
