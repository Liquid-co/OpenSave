package syncengine

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"

	"github.com/opensave/opensave/internal/delta"
)

// A save file the game holds open cannot be replaced either, and Windows says
// "access denied" for that too. The folder still takes writes, so it is not
// called unwritable: no message, no wait, and the next pass tries again
// (asked in review on #114).
func TestUnwritable_AFileInUseIsNotAnUnwritableFolder(t *testing.T) {
	env := setupEngine(t)
	write(t, env.localDir, "save.dat", "old")
	agreed, _ := delta.BuildManifest(env.localDir)
	env.engine.persistLineage("game1", env.peer.ID, agreed, agreed)
	if err := env.store.SetAgreedHash("game1", env.peer.ID, agreed.ManifestHash()); err != nil {
		t.Fatal(err)
	}
	write(t, env.remoteDir, "save.dat", "newer from the peer")

	// Held open for reading and writing but not deleting, as a running game
	// holds its save: replacing it is then refused as "access denied".
	p, _ := windows.UTF16PtrFromString(filepath.Join(env.localDir, "save.dat"))
	h, err := windows.CreateFile(p, windows.GENERIC_READ|windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	held := true
	defer func() {
		if held {
			windows.CloseHandle(h)
		}
	}()

	var logs []string
	env.engine.Log = func(level, msg string) { logs = append(logs, msg) }
	res, _ := env.engine.SyncGame(context.Background(), "game1", []Peer{env.peer})
	if r := res[env.peer.ID]; r.Status == "unwritable" {
		t.Errorf("a file in use was taken for an unwritable folder: %+v", r)
	}
	for _, l := range logs {
		if strings.Contains(l, "may not write to its save folder") {
			t.Errorf("said the folder cannot be written: %s", l)
		}
	}
	if _, waiting := env.engine.unwritableUntil("game1"); waiting {
		t.Error("pulls into the save wait, though only a file was in use")
	}

	// The game lets go: the next pass takes the save at once.
	windows.CloseHandle(h)
	held = false
	env.engine.SyncGame(context.Background(), "game1", []Peer{env.peer})
	if got := readFile(t, env.localDir, "save.dat"); got != "newer from the peer" {
		t.Errorf("after the file was let go, save.dat = %q", got)
	}
}
