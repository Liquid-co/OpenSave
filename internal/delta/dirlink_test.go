package delta

import (
	"os"
	"os/exec"
	"runtime"
	"testing"
)

// makeDirLink makes link a link to the folder target: a junction on Windows,
// which needs no special rights there, and a symbolic link elsewhere.
func makeDirLink(t *testing.T, link, target string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
			t.Skipf("cannot make a junction here: %v %s", err, out)
		}
		return
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot make a symbolic link here: %v", err)
	}
}
