//go:build windows

package e2e

import (
	"fmt"
	"os/exec"
	"strings"
)

// makeDirLink creates a directory junction. os.Symlink on Windows needs either
// Developer Mode or the create-symlink privilege, which a test machine may not
// have; a junction needs neither and is the alias real users create.
func makeDirLink(link, target string) error {
	out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput()
	if err != nil {
		return fmt.Errorf("mklink /J: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}
