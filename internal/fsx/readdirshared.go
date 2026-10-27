package fsx

import "os"

// ReadDirShared is os.ReadDir that lets another program delete the folder, or
// anything in it, while it is being listed.
//
// The same default as OpenShared's, for folders: on Windows os.ReadDir opens
// the folder without FILE_SHARE_DELETE, and while it is open the folder cannot
// be removed — "being used by another process". The watcher lists a folder
// whenever something inside it changes (checkFolders), so it was listing the
// folders a restore was emptying, and the restore failed; a game deleting a
// folder of its own saves at that moment fails the same way.
func ReadDirShared(path string) ([]os.DirEntry, error) {
	return readDirShared(path)
}
