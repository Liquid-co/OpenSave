//go:build windows

package daemon

import (
	"fmt"
	"os/exec"
	"strings"
)

// makeDirLink creates a directory junction. os.Symlink on Windows needs
// Developer Mode or a privilege a test machine may not have; a junction needs
// neither, and is the alias real users create when moving a folder to another
// drive.
func makeDirLink(link, target string) error {
	out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput()
	if err != nil {
		return fmt.Errorf("mklink /J: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}
