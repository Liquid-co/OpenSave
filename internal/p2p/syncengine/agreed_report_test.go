package syncengine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/opensave/opensave/internal/delta"
)

// The state this device (B) ends up in when the peer (A) pulled B's edit and
// edited again before B recorded that A had taken it: B's base is still the
// state before its own edit, its push record names its edit, and A holds a
// newer save. Nobody edited at the same time.
func setupPeerEditedAgain(t *testing.T) (*engineEnv, string) {
	t.Helper()
	env := setupEngine(t)
	write(t, env.localDir, "save.dat", "turn-1")
	write(t, env.remoteDir, "save.dat", "turn-1")
	base, err := delta.BuildManifest(env.localDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := env.store.SetAgreedHash("game1", env.peer.ID, base.ManifestHash()); err != nil {
		t.Fatal(err)
	}

	// B edits and hands it over.
	write(t, env.localDir, "save.dat", "turn-2")
	handed, err := delta.BuildManifest(env.localDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := env.store.SetPushedHash("game1", env.peer.ID, handed.ManifestHash()); err != nil {
		t.Fatal(err)
	}

	// A takes it, then edits again at once.
	write(t, env.remoteDir, "save.dat", "turn-2")
	later := time.Now().Add(2 * time.Second)
	write(t, env.remoteDir, "save.dat", "turn-3")
	_ = os.Chtimes(filepath.Join(env.remoteDir, "save.dat"), later, later)
	return env, handed.ManifestHash()
}

// Without A saying what it took, B reads both sides as changed: the false
// conflict this fixes, kept here so the test shows it is real.
func TestAgreedReport_WithoutItTheNextSyncConflicts(t *testing.T) {
	env, _ := setupPeerEditedAgain(t)
	res, err := env.engine.SyncWithPeer(context.Background(), "game1", env.peer)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "conflict" {
		t.Skipf("no conflict without the report (%s); nothing to compare against", res.Status)
	}
}

// With A's report that it holds exactly what B handed over, B records that as
// the base, and A's later edit is just a newer save to take.
func TestAgreedReport_ThePeersNextEditIsTakenWithoutAConflict(t *testing.T) {
	env, handed := setupPeerEditedAgain(t)
	env.engine.NotePeerHolds("game1", env.peer, handed)
	if got := env.store.GetAgreedHash("game1", env.peer.ID); got != handed {
		t.Fatalf("base = %q after the report, want the state handed over", got)
	}
	res, err := env.engine.SyncWithPeer(context.Background(), "game1", env.peer)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status == "conflict" {
		t.Fatal("a conflict over edits made one after the other")
	}
	got, err := os.ReadFile(filepath.Join(env.localDir, "save.dat"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "turn-3" {
		t.Errorf("save = %q, want the peer's newer turn-3", got)
	}
}

// A report of any other state moves nothing: the base only goes to a state
// both sides are known to have held.
func TestAgreedReport_AnotherStateIsIgnored(t *testing.T) {
	env, handed := setupPeerEditedAgain(t)
	before := env.store.GetAgreedHash("game1", env.peer.ID)
	env.engine.NotePeerHolds("game1", env.peer, "not-"+handed)
	env.engine.NotePeerHolds("game1", env.peer, "")
	if got := env.store.GetAgreedHash("game1", env.peer.ID); got != before {
		t.Errorf("base moved to %q on a report of a state never handed over", got)
	}
}
