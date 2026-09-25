package fsx

import "os"

// TryLock takes an exclusive lock on the file at path, creating it if need
// be, without waiting: ok is false when another holder has it — another
// process, or another TryLock in this one. unlock releases it.
//
// For work that two processes on one data folder must not do at once — the
// app and a command-line run of `opensave`, say. The lock goes with the open
// file, so a process that dies holding it releases it.
func TryLock(path string) (unlock func(), ok bool, err error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o666)
	if err != nil {
		return nil, false, err
	}
	ok, err = tryLock(f)
	if err != nil || !ok {
		f.Close()
		return nil, false, err
	}
	return func() {
		unlockFile(f)
		f.Close()
	}, true, nil
}
