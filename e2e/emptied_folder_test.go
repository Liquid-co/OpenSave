package e2e

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/opensave/opensave/testutil"
)

// emptyFolder deletes every file in a save folder and leaves the folder:
// what an uninstaller, a game resetting its saves, or a person clearing the
// wrong folder does.
func emptyFolder(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		testutil.RemoveTree(t, filepath.Join(dir, e.Name()))
	}
}

// syncBothWays has each device reach for the other a few times, as they
// would on their own. A held device answers its own sync with a refusal,
// which is the point, so statuses are not checked here.
func syncBothWays(t *testing.T, gameID string, a, b *testutil.TestDaemon) {
	t.Helper()
	for i := 0; i < 3; i++ {
		a.APIStatus(http.MethodPost, "/api/games/"+gameID+"/sync", nil, nil)
		b.APIStatus(http.MethodPost, "/api/games/"+gameID+"/sync", nil, nil)
		time.Sleep(time.Second)
	}
	testutil.SettleSync(t, gameID, a, b)
}

var emptiedSave = map[string]string{
	"slot1.sav":        "hours of play",
	"slot2.sav":        "more hours",
	"profile/user.cfg": "settings",
}

func stillHasIt(t *testing.T, d *testutil.TestDaemon, what string) {
	t.Helper()
	for rel, want := range emptiedSave {
		if got := d.ReadSave(rel); got != want {
			t.Errorf("%s: %s is %q, want %q", what, rel, got, want)
		}
	}
}

type emptiedInfo struct {
	Files     int      `json:"files"`
	Locations []string `json:"locations"`
}

func emptiedState(t *testing.T, d *testutil.TestDaemon, gameID string) *emptiedInfo {
	t.Helper()
	var games map[string]struct {
		Emptied *emptiedInfo `json:"emptied"`
	}
	d.API(http.MethodGet, "/api/games", nil, &games)
	return games[gameID].Emptied
}

// emptiedAndHeld empties A's save and has both devices try to sync.
func emptiedAndHeld(t *testing.T, name string) (a, b *testutil.TestDaemon, gameID string) {
	t.Helper()
	a, b, gameID = pairAndTrack(t, name, emptiedSave)
	testutil.SettleSync(t, gameID, a, b)
	emptyFolder(t, a.SaveDir)
	syncBothWays(t, gameID, a, b)
	stillHasIt(t, b, "the other device, while the emptied save is held back")
	if e := emptiedState(t, a, gameID); e == nil || e.Files != 3 {
		t.Fatalf("the emptied device says %+v; want the game held back over 3 files", e)
	}
	return a, b, gameID
}

// Every save file deleted at once on one device is held back from the other
// until someone says whether it was meant: the other device keeps its copy,
// and this device's own sync says why it will not run.
func TestEmptiedFolder_IsHeldBackFromTheOtherDevice(t *testing.T) {
	a, b, gameID := emptiedAndHeld(t, "Emptied Held Game")
	var refusal struct {
		Error string `json:"error"`
	}
	if code := a.APIStatus(http.MethodPost, "/api/games/"+gameID+"/sync", nil, &refusal); code != http.StatusConflict {
		t.Errorf("syncing the held game here = %d %q, want a refusal that says why", code, refusal.Error)
	}
	if e := emptiedState(t, b, gameID); e != nil {
		t.Errorf("the device that still has the files says it was emptied: %+v", e)
	}

	// The list the CLI reads.
	var list []struct {
		GameID string `json:"gameId"`
	}
	a.API(http.MethodGet, "/api/emptied", nil, &list)
	if len(list) != 1 || list[0].GameID != gameID {
		t.Errorf("the emptied list is %+v", list)
	}
}

// "Delete them there too": the deletion goes on, and the device that
// received it does not ask its own user again.
func TestEmptiedFolder_ConfirmedDeletionGoesOn(t *testing.T) {
	a, b, gameID := emptiedAndHeld(t, "Emptied Confirmed Game")
	a.API(http.MethodPost, "/api/games/"+gameID+"/emptied", map[string]string{"answer": "delete"}, nil)
	syncBothWays(t, gameID, a, b)
	for rel := range emptiedSave {
		if _, err := os.Stat(filepath.Join(b.SaveDir, filepath.FromSlash(rel))); err == nil {
			t.Errorf("after the deletion was confirmed, the other device still has %s", rel)
		}
	}
	if e := emptiedState(t, a, gameID); e != nil {
		t.Errorf("still held after the answer: %+v", e)
	}
	if e := emptiedState(t, b, gameID); e != nil {
		t.Errorf("the device the confirmed deletion reached asks again: %+v", e)
	}

	// Answered once, it is not a standing permission: files again, then
	// emptied again, is a new question.
	a.WriteSave("slot1.sav", "a new game")
	syncBothWays(t, gameID, a, b)
	emptyFolder(t, a.SaveDir)
	syncBothWays(t, gameID, a, b)
	if got := b.ReadSave("slot1.sav"); got != "a new game" {
		t.Errorf("the second emptying went through unasked: the other device's slot1.sav is %q", got)
	}
	if emptiedState(t, a, gameID) == nil {
		t.Error("the second emptying is not held")
	}
}

// "Put them back": the newest snapshot with the files is restored here, and
// the other device is not touched.
func TestEmptiedFolder_PutBackRestoresThem(t *testing.T) {
	a, b, gameID := emptiedAndHeld(t, "Emptied Put Back Game")
	var res struct {
		Restored string `json:"restored"`
	}
	a.API(http.MethodPost, "/api/games/"+gameID+"/emptied", map[string]string{"answer": "restore"}, &res)
	if res.Restored == "" {
		t.Error("no snapshot was restored")
	}
	stillHasIt(t, a, "the emptied device after putting them back")
	syncBothWays(t, gameID, a, b)
	stillHasIt(t, b, "the other device after putting them back")
	stillHasIt(t, a, "the emptied device after syncing")
	if e := emptiedState(t, a, gameID); e != nil {
		t.Errorf("still held after the answer: %+v", e)
	}
}

// With no snapshot to restore from, "put them back" fetches the files from
// the other device — rather than the sync reading them as deleted here.
func TestEmptiedFolder_PutBackWithoutASnapshotFetchesThem(t *testing.T) {
	a, b, gameID := emptiedAndHeld(t, "Emptied Fetch Game")
	var snaps map[string]struct {
		Branches map[string]struct {
			Snapshots []struct {
				ID string `json:"id"`
			} `json:"snapshots"`
		} `json:"branches"`
	}
	a.API(http.MethodGet, "/api/games", nil, &snaps)
	for _, br := range snaps[gameID].Branches {
		for _, s := range br.Snapshots {
			if err := a.Daemon.Store.SetSnapshotCheck(s.ID, time.Now().UnixMilli(), "damaged for this test"); err != nil {
				t.Fatal(err)
			}
		}
	}
	var res struct {
		Restored string `json:"restored"`
		Fetching int    `json:"fetching"`
	}
	a.API(http.MethodPost, "/api/games/"+gameID+"/emptied", map[string]string{"answer": "restore"}, &res)
	if res.Restored != "" || res.Fetching != 3 {
		t.Errorf("answer = %+v; want nothing restored and 3 files to fetch", res)
	}
	syncBothWays(t, gameID, a, b)
	stillHasIt(t, b, "the other device")
	stillHasIt(t, a, "the emptied device, from the other")
}

// Files put back by hand — from the Recycle Bin, say — end the hold: nothing
// would be deleted any more.
func TestEmptiedFolder_FilesBackByHandEndTheHold(t *testing.T) {
	a, b, gameID := emptiedAndHeld(t, "Emptied Recycled Game")
	for rel, body := range emptiedSave {
		a.WriteSave(rel, body)
	}
	syncBothWays(t, gameID, a, b)
	if e := emptiedState(t, a, gameID); e != nil {
		t.Errorf("still held with every file back: %+v", e)
	}
	stillHasIt(t, b, "the other device")
}

// A new save written into the emptied folder — the game starting over — is
// not an answer: the old saves on the other device are still in question.
func TestEmptiedFolder_ANewSaveIsNotAnAnswer(t *testing.T) {
	a, b, gameID := emptiedAndHeld(t, "Emptied New Start Game")
	a.WriteSave("slot1.sav", "a fresh start")
	syncBothWays(t, gameID, a, b)
	stillHasIt(t, b, "the other device, after a new save in the emptied folder")
	if emptiedState(t, a, gameID) == nil {
		t.Error("a new save ended the hold")
	}
}

// The receiving side: a device emptied without knowing what it held — one
// from before holding back, or one whose record of what it shares with this
// device has not caught up — sends its empty folder as it is. The device
// that would do the deleting keeps its copies until it is told the emptying
// was meant.
func TestEmptiedFolder_AnUnconfirmedEmptyFolderIsNotTakenAsDeletions(t *testing.T) {
	a, b, gameID := pairAndTrack(t, "Emptied Unknowing Game", emptiedSave)
	testutil.SettleSync(t, gameID, a, b)
	// A forgets what it shares with B, so it has nothing to hold back, and
	// starts no sync of its own.
	a.API(http.MethodPatch, "/api/games/"+gameID, map[string]any{"autoSync": false}, nil)
	if err := a.Daemon.Store.ForgetGameSyncState(gameID); err != nil {
		t.Fatal(err)
	}
	emptyFolder(t, a.SaveDir)

	var res struct {
		Results map[string]struct {
			Status string `json:"status"`
		} `json:"results"`
	}
	for i := 0; i < 3; i++ {
		b.API(http.MethodPost, "/api/games/"+gameID+"/sync", nil, &res)
		time.Sleep(500 * time.Millisecond)
	}
	testutil.SettleSync(t, gameID, a, b)
	stillHasIt(t, b, "the other device, after an unconfirmed empty folder")
	for _, r := range res.Results {
		if r.Status != "peer_holding" {
			t.Errorf("the sync says %q, want peer_holding", r.Status)
		}
	}
}
