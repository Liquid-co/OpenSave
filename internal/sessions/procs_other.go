//go:build !windows && !linux

package sessions

import "errors"

// listProcesses is not written for this system yet; nothing is ever seen
// playing, and `opensave wrap` still marks sessions.
func listProcesses() ([]Proc, error) {
	return nil, errors.New("finding running games is not supported on this system")
}
