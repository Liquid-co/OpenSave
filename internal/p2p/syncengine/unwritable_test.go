package syncengine

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/opensave/opensave/internal/delta"
)

// makeUnwritable makes a pull into the save fail the way a folder under
// Program Files does: with "access denied". On Windows a folder denied writing cannot
// be written; elsewhere a folder without write permission takes no new file.
func makeUnwritable(t *testing.T, dir string) (restore func()) {
	t.Helper()
	if runtime.GOOS == "windows" {
		// Everyone (S-1-1-0) denied writing in the folder: what a folder under
		// Program Files is to a program not run as administrator.
		if out, err := exec.Command("icacls", dir, "/deny", "*S-1-1-0:(OI)(CI)(WD,AD)").CombinedOutput(); err != nil {
			t.Fatalf("icacls: %v %s", err, out)
		}
		restore = func() { exec.Command("icacls", dir, "/remove:d", "*S-1-1-0").Run() }
		t.Cleanup(restore)
		return restore
	}
	if os.Geteuid() == 0 {
		t.Skip("root writes anywhere")
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	restore = func() { os.Chmod(dir, 0o777) }
	t.Cleanup(restore)
	return restore
}

// A save folder that cannot be written is said once, plainly, and pulls into
// it wait instead of failing on every pass.
func TestUnwritable_SaidOnceAndNotRetriedEveryPass(t *testing.T) {
	env := setupEngine(t)
	write(t, env.localDir, "save.dat", "old")
	agreed, _ := delta.BuildManifest(env.localDir)
	env.engine.persistLineage("game1", env.peer.ID, agreed, agreed)
	if err := env.store.SetAgreedHash("game1", env.peer.ID, agreed.ManifestHash()); err != nil {
		t.Fatal(err)
	}
	write(t, env.remoteDir, "save.dat", "newer from the peer")
	stampFile(t, env.remoteDir, "save.dat", time.Now().Add(time.Hour))
	restore := makeUnwritable(t, env.localDir)

	var logs []string
	env.engine.Log = func(level, msg string) { logs = append(logs, level+": "+msg) }

	for i := 0; i < 3; i++ {
		res, _ := env.engine.SyncGame(context.Background(), "game1", []Peer{env.peer})
		t.Logf("pass %d: %+v", i, res[env.peer.ID])
	}
	n := 0
	for _, l := range logs {
		if strings.Contains(l, "may not write to its save folder") {
			n++
		}
		t.Log(l)
	}
	if n != 1 {
		t.Errorf("the unwritable folder was said %d times over three passes, want once", n)
	}

	// Rights given, and the wait over: the next pass takes the peer's save.
	restore()
	env.engine.unwritableMu.Lock()
	env.engine.unwritable["game1"] = time.Now().Add(-time.Second)
	env.engine.unwritableMu.Unlock()
	res, _ := env.engine.SyncGame(context.Background(), "game1", []Peer{env.peer})
	if got := readFile(t, env.localDir, "save.dat"); got != "newer from the peer" {
		t.Errorf("after the wait the save was not taken: %+v, save.dat=%q", res[env.peer.ID], got)
	}
}
