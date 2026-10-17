package syncengine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/opensave/opensave/internal/snapshot"
	"github.com/opensave/opensave/internal/store"
)

// A choice made on this device that the peer hasn't acted on yet is not
// undone by this device's own next sync (choices.go, GitHub #81 and #85), and
// the note that keeps it never outlives its reason.

func readSave(t *testing.T, dir, rel string) string {
	t.Helper()
	b, _ := os.ReadFile(filepath.Join(dir, rel))
	return string(b)
}

// keptMine is this device having answered a conflict with "Keep mine"; the
// peer, whose own user hasn't decided, still holds its version.
func keptMine(t *testing.T) *engineEnv {
	t.Helper()
	env := setupEngine(t)
	write(t, env.localDir, "save.dat", "local version")
	write(t, env.remoteDir, "save.dat", "remote version")
	if err := env.store.UpdatePeerLastSynced(env.peer.ID, "2026-01-01T00:00:00.000Z"); err != nil {
		t.Fatal(err)
	}
	if res, err := env.engine.SyncWithPeer(context.Background(), "game1", env.peer); err != nil || res.Status != "conflict" {
		t.Fatalf("setup: expected a conflict, got %+v %v", res, err)
	}
	if _, err := env.engine.ResolveConflict(context.Background(), "game1", env.peer.ID, "keep-local"); err != nil {
		t.Fatal(err)
	}
	return env
}

// #81: the next sync, before the peer has decided, took the peer's version.
func TestKeepMine_SurvivesASyncBeforeThePeerDecides(t *testing.T) {
	env := keptMine(t)
	triggers := env.transport.pullTriggers
	res, err := env.engine.SyncWithPeer(context.Background(), "game1", env.peer)
	if err != nil {
		t.Fatal(err)
	}
	if got := readSave(t, env.localDir, "save.dat"); got != "local version" {
		t.Fatalf("\"Keep mine\" undone by the next sync: save.dat is %q (status %q)", got, res.Status)
	}
	if env.transport.pullTriggers == triggers {
		t.Error("the peer was not asked again to take this device's version")
	}
}

// Kept across a restart: a PC switched off overnight keeps its choice.
func TestKeepMine_SurvivesARestart(t *testing.T) {
	env := keptMine(t)
	restarted := New(env.store, snapshot.New(env.store), env.transport)
	if _, err := restarted.SyncWithPeer(context.Background(), "game1", env.peer); err != nil {
		t.Fatal(err)
	}
	if got := readSave(t, env.localDir, "save.dat"); got != "local version" {
		t.Fatalf("\"Keep mine\" undone after a restart: save.dat is %q", got)
	}
}

// The peer takes this device's version: the note has done its job and goes.
func TestKeepMine_ThePeerTakingItClearsTheNote(t *testing.T) {
	env := keptMine(t)
	write(t, env.remoteDir, "save.dat", "local version")
	res, err := env.engine.SyncWithPeer(context.Background(), "game1", env.peer)
	if err != nil || res.Status != "in_sync" {
		t.Fatalf("status %+v %v, want in_sync", res, err)
	}
	if n := env.store.PeerNote("game1", env.peer.ID, store.NoteDeclined); n != "" {
		t.Errorf("note left behind after the peer took this device's version: %q", n)
	}
}

// A stale note can't block a real update: once the peer's save is anything
// other than the version declined, it is taken as usual, and the note goes.
func TestKeepMine_APeerThatChangesIsTakenAsUsual(t *testing.T) {
	env := keptMine(t)
	write(t, env.remoteDir, "save.dat", "remote, played on")
	if _, err := env.engine.SyncWithPeer(context.Background(), "game1", env.peer); err != nil {
		t.Fatal(err)
	}
	if got := readSave(t, env.localDir, "save.dat"); got != "remote, played on" {
		t.Errorf("the peer's new save was not taken: save.dat is %q", got)
	}
	if n := env.store.PeerNote("game1", env.peer.ID, store.NoteDeclined); n != "" {
		t.Errorf("note left behind after the peer changed its save: %q", n)
	}
}

// switchedByHand is this device starting a new run on a new branch, the peer
// still on main.
func switchedByHand(t *testing.T) *engineEnv {
	t.Helper()
	env := setupEngine(t)
	write(t, env.localDir, "slot1.sav", "main-save")
	write(t, env.remoteDir, "slot1.sav", "main-save")
	if res, err := env.engine.SyncWithPeer(context.Background(), "game1", env.peer); err != nil || res.Status != "in_sync" {
		t.Fatalf("setup: %+v %v", res, err)
	}
	if _, err := env.engine.Snapshots.CreateBranch("game1", "new-game-plus", false); err != nil {
		t.Fatal(err)
	}
	if err := env.engine.Snapshots.SwitchBranch("game1", "new-game-plus"); err != nil {
		t.Fatal(err)
	}
	env.engine.NoteBranchSwitch("game1", "main")
	write(t, env.localDir, "slot1.sav", "ngplus-save")
	return env
}

func activeBranch(t *testing.T, env *engineEnv) string {
	t.Helper()
	g, err := env.store.GetGame("game1")
	if err != nil {
		t.Fatal(err)
	}
	return g.ActiveBranch
}

// #85: the next sync followed the peer back to the branch left.
func TestBranchSwitch_SurvivesThisDevicesNextSync(t *testing.T) {
	env := switchedByHand(t)
	triggers := env.transport.pullTriggers
	if _, err := env.engine.SyncWithPeer(context.Background(), "game1", env.peer); err != nil {
		t.Fatal(err)
	}
	if b, got := activeBranch(t, env), readSave(t, env.localDir, "slot1.sav"); b != "new-game-plus" || got != "ngplus-save" {
		t.Fatalf("switch undone by the next sync: on %q with slot1 %q", b, got)
	}
	if env.transport.pullTriggers == triggers {
		t.Error("the peer was not asked to follow")
	}
}

// The peer follows: the note goes, and the two sync as usual.
func TestBranchSwitch_ThePeerFollowingClearsTheNote(t *testing.T) {
	env := switchedByHand(t)
	env.transport.remoteBranch = "new-game-plus"
	write(t, env.remoteDir, "slot1.sav", "ngplus-save")
	if _, err := env.engine.SyncWithPeer(context.Background(), "game1", env.peer); err != nil {
		t.Fatal(err)
	}
	if n := env.store.PeerNote("game1", env.peer.ID, store.NoteLeftBranch); n != "" {
		t.Errorf("note left behind after the peer followed: %q", n)
	}
}

// A stale note can't block a real update: a peer that moved on to another
// branch of its own is followed as before.
func TestBranchSwitch_APeerThatMovedOnIsFollowed(t *testing.T) {
	env := switchedByHand(t)
	if _, err := env.engine.Snapshots.CreateBranch("game1", "speedrun", false); err != nil {
		t.Fatal(err)
	}
	env.transport.remoteBranch = "speedrun"
	if _, err := env.engine.SyncWithPeer(context.Background(), "game1", env.peer); err != nil {
		t.Fatal(err)
	}
	if b := activeBranch(t, env); b != "speedrun" {
		t.Errorf("a peer on a branch of its own was not followed: this device is on %q", b)
	}
	if n := env.store.PeerNote("game1", env.peer.ID, store.NoteLeftBranch); n != "" {
		t.Errorf("note left behind after the peer moved on: %q", n)
	}
}

// A further switch by hand replaces the notes, a "Keep mine" included: it was
// about a save on the branch left behind.
func TestBranchSwitch_AFurtherSwitchReplacesTheNotes(t *testing.T) {
	env := keptMine(t)
	if env.store.PeerNote("game1", env.peer.ID, store.NoteDeclined) == "" {
		t.Fatal("setup: no note after \"Keep mine\"")
	}
	env.engine.NoteBranchSwitch("game1", "main")
	if n := env.store.PeerNote("game1", env.peer.ID, store.NoteDeclined); n != "" {
		t.Errorf("the \"Keep mine\" note outlived a branch switch: %q", n)
	}
	if n := env.store.PeerNote("game1", env.peer.ID, store.NoteLeftBranch); n != "main" {
		t.Errorf("left-branch note %q, want main", n)
	}
	env.engine.NoteBranchSwitch("game1", "new-game-plus")
	if n := env.store.PeerNote("game1", env.peer.ID, store.NoteLeftBranch); n != "new-game-plus" {
		t.Errorf("after a second switch the note is %q, want the branch left last", n)
	}
}

// Untrack and unpair take the notes with them.
func TestNotes_GoWithUntrackAndUnpair(t *testing.T) {
	env := keptMine(t)
	env.engine.NoteBranchSwitch("game1", "main")
	_ = env.store.SetPeerNote("game1", env.peer.ID, store.NoteDeclined, "x")
	if err := env.store.ForgetGameSyncState("game1"); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{store.NoteDeclined, store.NoteLeftBranch} {
		if n := env.store.PeerNote("game1", env.peer.ID, kind); n != "" {
			t.Errorf("untrack left a %s note: %q", kind, n)
		}
	}

	_ = env.store.SetPeerNote("game1", env.peer.ID, store.NoteDeclined, "x")
	if err := env.store.UnpairPeer(env.peer.ID); err != nil {
		t.Fatal(err)
	}
	if n := env.store.PeerNote("game1", env.peer.ID, store.NoteDeclined); n != "" {
		t.Errorf("unpair left a note: %q", n)
	}
}
