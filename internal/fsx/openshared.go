// Package fsx holds filesystem helpers whose correct behaviour differs by
// platform in ways the standard library does not expose.
package fsx

import "os"

// OpenShared opens a file for reading, letting another program delete it
// while it is open.
//
// This exists because of what Windows does by default. Go's os.Open asks for
// FILE_SHARE_READ|FILE_SHARE_WRITE and not FILE_SHARE_DELETE, so for as long
// as OpenSave holds a save file open — hashing it, reading blocks to send,
// archiving it into a snapshot — the game that owns it cannot delete it. That
// is a real failure for a tool whose only job is to keep saves safe: a manifest
// build runs on a timer over every tracked save, and a game deleting a save
// slot in that window gets an access-denied error from an operation that
// normally cannot fail.
//
// What this does NOT fix, and cannot: replacing the file by renaming another
// one over it. Windows refuses to replace a file that has ANY open handle,
// whatever share mode that handle was opened with — measured, not assumed;
// adding DELETE access to the handle does not change it either. Since writing
// a temp file and renaming it over the original is how a great many games save
// safely, OpenSave can still make a game's save fail simply by reading at the
// wrong moment.
//
// The only real mitigation for that is to not hold the file open, and to not
// open it at all when nothing has changed — which is what the hash cache in
// internal/delta does, and is a much larger part of the answer here than this
// function is. This closes the half that can be closed.
//
// On other platforms an open descriptor never blocked unlink or rename in the
// first place, so this is os.Open.
func OpenShared(path string) (*os.File, error) {
	return openShared(path)
}
