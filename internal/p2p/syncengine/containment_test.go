package syncengine

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
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

// A link inside the save folder leads out of it. The other device naming a
// file behind it — here, its own real folder of the same name — must not get
// that file written where the link points.
func TestSync_APullDoesNotWriteThroughALink(t *testing.T) {
	env := setupEngine(t)
	outside := filepath.Join(filepath.Dir(env.localDir), "outside")
	write(t, outside, "x.sav", "not part of any save")
	makeDirLink(t, filepath.Join(env.localDir, "link"), outside)

	write(t, env.localDir, "slot1.sav", "local")
	write(t, env.remoteDir, "slot1.sav", "local")
	write(t, env.remoteDir, "link/x.sav", "the other device's bytes")
	write(t, env.remoteDir, "link/new.sav", "a file that would be created out there")

	_, _ = env.engine.SyncWithPeer(context.Background(), "game1", env.peer)

	if got, _ := os.ReadFile(filepath.Join(outside, "x.sav")); string(got) != "not part of any save" {
		t.Errorf("a file outside the save folder was overwritten through a link: %q", got)
	}
	if _, err := os.Stat(filepath.Join(outside, "new.sav")); err == nil {
		t.Error("a file was created outside the save folder through a link")
	}
}
