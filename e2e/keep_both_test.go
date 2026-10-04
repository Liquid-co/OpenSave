package e2e

import (
	"archive/zip"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/opensave/opensave/internal/snapshot"
	"github.com/opensave/opensave/testutil"
)

// "Keep both" is the recommended answer to a conflict, and answering it on
// both devices emptied both saves: each switched itself onto a branch named
// after the other and filled with the other's version, the two branches had
// different names, and each then followed the other's branch by creating it
// empty — and read its empty folder, against their old record of shared files,
// as every file deleted, which it passed on. See the "merge-branch" case in
// syncengine/conflict.go and store.SwitchActiveBranch.

type branchState struct {
	ActiveBranch string `json:"activeBranch"`
	Branches     map[string]struct {
		Snapshots []struct {
			ZipPath string `json:"zipPath"`
		} `json:"snapshots"`
	} `json:"branches"`
	Emptied any `json:"emptied"`
}

func gameState(t *testing.T, d *testutil.TestDaemon, gameID string) branchState {
	t.Helper()
	var games map[string]branchState
	d.API(http.MethodGet, "/api/games", nil, &games)
	return games[gameID]
}

// sideBranch is the one branch other than main, and "" when there is none.
func sideBranch(st branchState) string {
	for name := range st.Branches {
		if name != "main" {
			return name
		}
	}
	return ""
}

// branchHolds is what a file holds in a branch's newest snapshot: what
// switching to that branch would put back.
func branchHolds(t *testing.T, st branchState, branch, rel string) string {
	t.Helper()
	snaps := st.Branches[branch].Snapshots
	if len(snaps) == 0 {
		return ""
	}
	// The API lists a branch's snapshots oldest first (gamePayload), so the
	// newest — what a switch puts back — is the last.
	newest := snaps[len(snaps)-1]
	path, done, err := snapshot.OpenArchive(newest.ZipPath)
	if err != nil {
		t.Fatalf("open %s: %v", newest.ZipPath, err)
	}
	defer done()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if snapshot.ArchiveEntryRelPath(f.Name) != rel {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(rc)
		rc.Close()
		return string(b)
	}
	return ""
}

func conflictOnBoth(t *testing.T, name string) (a, b *testutil.TestDaemon, gameID string) {
	t.Helper()
	a, b, gameID = pairAndTrack(t, name, map[string]string{"slot1.sav": "shared", "profile.sav": "p"})
	logOnFailure(t, a, b)
	time.Sleep(syncSettleWindow)
	a.WriteSave("slot1.sav", "A-version")
	b.WriteSave("slot1.sav", "B-version")
	if status, _ := syncTo(a, gameID, b.NodeID()); status != "conflict" {
		t.Fatalf("expected a conflict on A, got %q", status)
	}
	return a, b, gameID
}

func keepBoth(d, other *testutil.TestDaemon, gameID string) {
	d.API(http.MethodPost, "/api/games/"+gameID+"/resolve-conflict", map[string]string{
		"peerId": other.NodeID(), "resolution": "merge-branch",
	}, nil)
}

// Keep both, on one device: it goes on playing its own version at once — no
// branch switch, no save folder emptied and refilled — and has the other's
// beside it. The other device is asked in turn.
func TestKeepBoth_KeepsPlayingYoursAndKeepsTheirsBeside(t *testing.T) {
	a, b, gameID := conflictOnBoth(t, "KeepOne")
	keepBoth(a, b, gameID)

	// The answer is applied in the background. Watched throughout: A's save
	// is its own version at every moment, never emptied and refilled and never
	// B's for a while — which is what the old switch-and-overwrite did.
	seen := map[string]bool{}
	if !testutil.WaitFor(30*time.Second, func() bool {
		seen[a.ReadSave("slot1.sav")] = true
		return !hasConflict(a, gameID) && sideBranch(gameState(t, a, gameID)) != ""
	}) {
		t.Fatal("A's Keep both never finished")
	}
	for v := range seen {
		if v != "A-version" {
			t.Errorf("while Keep both was applied, A's save was %q; it must stay A's own", v)
		}
	}
	st := gameState(t, a, gameID)
	if st.ActiveBranch != "main" {
		t.Errorf("A moved to branch %q; Keep both keeps it where it was", st.ActiveBranch)
	}
	side := sideBranch(st)
	if side == "" {
		t.Fatal("A has no branch holding B's version")
	}
	if got := branchHolds(t, st, side, "slot1.sav"); got != "B-version" {
		t.Errorf("A's branch %q holds slot1=%q, want B's version", side, got)
	}

	if !testutil.WaitFor(30*time.Second, func() bool { return hasConflict(b, gameID) }) {
		t.Fatal("B was never asked")
	}
	if got := b.ReadSave("slot1.sav"); got != "B-version" {
		t.Errorf("B's save changed to %q before B answered", got)
	}
	if a.ReadSave("slot1.sav") != "A-version" {
		t.Errorf("A's save changed to %q while B was being asked", a.ReadSave("slot1.sav"))
	}
}

// Keep both, on both devices. Nobody's save is emptied or held, both stay on
// their branch, they end up agreeing on one version, and the other is kept.
func TestKeepBoth_OnBothDevicesEmptiesNeither(t *testing.T) {
	a, b, gameID := conflictOnBoth(t, "KeepBoth")
	if status, _ := syncTo(b, gameID, a.NodeID()); status != "conflict" {
		t.Fatalf("expected a conflict on B too, got %q", status)
	}
	keepBoth(a, b, gameID)
	keepBoth(b, a, gameID)

	// As they would: a few rounds from each side.
	for i := 0; i < 3; i++ {
		time.Sleep(2 * time.Second)
		for _, d := range []*testutil.TestDaemon{a, b} {
			var out map[string]any
			d.API(http.MethodPost, "/api/games/"+gameID+"/sync", nil, &out)
		}
	}
	if !testutil.WaitFor(45*time.Second, func() bool {
		sa := a.ReadSave("slot1.sav")
		return sa != "" && sa == b.ReadSave("slot1.sav") && a.ReadSave("profile.sav") == "p" && b.ReadSave("profile.sav") == "p"
	}) {
		t.Fatalf("they never agreed on a whole save: A slot1=%q profile=%q, B slot1=%q profile=%q",
			a.ReadSave("slot1.sav"), a.ReadSave("profile.sav"), b.ReadSave("slot1.sav"), b.ReadSave("profile.sav"))
	}
	settledOnBoth(t, gameID, a, b)
	for _, pair := range [][2]*testutil.TestDaemon{{a, b}, {b, a}} {
		d, other := pair[0], pair[1]
		st := gameState(t, d, gameID)
		if st.Emptied != nil {
			t.Errorf("%s is holding the game back as if its save had been deleted", d.Name())
		}
		if st.ActiveBranch != "main" {
			t.Errorf("%s ended on branch %q", d.Name(), st.ActiveBranch)
		}
		want := "B-version"
		if other == a {
			want = "A-version"
		}
		if side := sideBranch(st); side == "" || branchHolds(t, st, side, "slot1.sav") != want {
			t.Errorf("%s does not keep %s's version beside its own (branch %q)", d.Name(), other.Name(), side)
		}
	}
}

// A device following the other onto a branch it has not got creates it empty.
// Read against the old branch's record of shared files, that empty folder
// said every file had been deleted here, and the sync deleted them on the
// device it was following. Following must fetch them instead.
func TestBranchFollow_FetchesTheOtherDevicesSaveNotDeletesIt(t *testing.T) {
	a, b, gameID := pairAndTrack(t, "Follow", map[string]string{"slot1.sav": "main-save", "profile.sav": "main-profile"})
	logOnFailure(t, a, b)
	time.Sleep(syncSettleWindow)

	// A starts a fresh run on a new branch. Its own syncs are kept quiet, so
	// B is the one that syncs first and follows.
	a.API(http.MethodPost, "/api/settings", map[string]any{"syncOnWatch": false}, nil)
	a.API(http.MethodPost, "/api/games/"+gameID+"/branch", map[string]any{"name": "new-game-plus", "copyCurrentSave": false}, nil)
	a.API(http.MethodPost, "/api/games/"+gameID+"/branch/switch", map[string]any{"name": "new-game-plus"}, nil)
	a.WriteSave("slot1.sav", "ngplus-save")
	a.WriteSave("profile.sav", "ngplus-profile")

	syncTo(b, gameID, a.NodeID())

	if !testutil.WaitFor(30*time.Second, func() bool {
		return b.ReadSave("slot1.sav") == "ngplus-save" && b.ReadSave("profile.sav") == "ngplus-profile"
	}) {
		t.Errorf("B never took A's new run: slot1=%q profile=%q", b.ReadSave("slot1.sav"), b.ReadSave("profile.sav"))
	}
	settledOnBoth(t, gameID, a, b)
	if got, got2 := a.ReadSave("slot1.sav"), a.ReadSave("profile.sav"); got != "ngplus-save" || got2 != "ngplus-profile" {
		t.Fatalf("following A's branch deleted A's save: slot1=%q profile=%q", got, got2)
	}
	stB := gameState(t, b, gameID)
	if stB.ActiveBranch != "new-game-plus" {
		t.Errorf("B is on %q, want the branch it followed", stB.ActiveBranch)
	}
	if stB.Emptied != nil || gameState(t, a, gameID).Emptied != nil {
		t.Error("a device is holding the game back as if its save had been deleted")
	}
	if got := branchHolds(t, stB, "main", "slot1.sav"); got != "main-save" {
		t.Errorf("B's main branch keeps slot1=%q, want the save it had before following", got)
	}
}
