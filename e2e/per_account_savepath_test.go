package e2e

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/opensave/opensave/testutil"
)

// Some games keep their saves in a folder named after the player's own account
// — Satisfactory under <LocalAppData>/FactoryGame/Saved/SaveGames/<epic-id>,
// and plenty of other Unreal titles the same way. That folder name is
// different on every machine, and the game only reads the one matching whoever
// is signed in.
//
// So the answer for two friends sharing a world is that each device points at
// its OWN account folder. That only works if sync places files by their
// position inside each device's save root and never carries the root's own
// name across — otherwise A's save would arrive on B inside a folder named
// after A's account, where B's game would never look for it.
//
// Every other e2e test happens to exercise this, since each daemon gets its
// own temp SaveDir. None of them pin it, and none use roots whose LAST
// segment differs, which is the part that matters here. Changing the save
// folder is now a supported edit in the app's Manage tab, so this is the
// guarantee that edit rests on.
func TestPeersWithDifferentAccountFoldersConverge(t *testing.T) {
	a := testutil.NewTestDaemon(t, "PerAccount-A")
	b := testutil.NewTestDaemon(t, "PerAccount-B")
	a.PairWith(b)

	// The distinguishing detail: the roots differ in their final segment, the
	// way two players' account folders do.
	const (
		accountA = "76561198000000001"
		accountB = "76561198000000002"
	)
	rootA := filepath.Join(a.SaveDir, "SaveGames", accountA)
	rootB := filepath.Join(b.SaveDir, "SaveGames", accountB)
	for _, dir := range []string{rootA, rootB} {
		if err := os.MkdirAll(dir, 0o777); err != nil {
			t.Fatal(err)
		}
	}

	if err := os.WriteFile(filepath.Join(rootA, "world.sav"), []byte("shared-world"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Same name on both, so they match; different roots, as two players would
	// have. Tracked through the API because the harness's TrackGame uses the
	// daemon's own SaveDir, and a custom root is the whole point here.
	var gameA, gameB struct {
		ID string `json:"id"`
	}
	a.API(http.MethodPost, "/api/games",
		map[string]string{"name": "Satisfactory", "savePath": rootA}, &gameA)
	b.API(http.MethodPost, "/api/games",
		map[string]string{"name": "Satisfactory", "savePath": rootB}, &gameB)
	if gameA.ID == "" || gameB.ID == "" {
		t.Fatalf("tracking failed: a=%q b=%q", gameA.ID, gameB.ID)
	}

	syncTo(a, gameA.ID, b.NodeID())

	landed := filepath.Join(rootB, "world.sav")
	if !testutil.WaitFor(45*time.Second, func() bool {
		data, err := os.ReadFile(landed)
		return err == nil && string(data) == "shared-world"
	}) {
		got, err := os.ReadFile(landed)
		t.Fatalf("the save never arrived in the peer's OWN account folder %s (got %q, err %v) — "+
			"pointing each device at its own folder is the advice for per-account games, "+
			"and it depends on this", landed, got, err)
	}

	// The other half, and the one that would actually break the advice: A's
	// account folder must not have been recreated on B. If it were, B would
	// hold the save at a path its game never reads, and the sync would look
	// like it had worked.
	stray := filepath.Join(b.SaveDir, "SaveGames", accountA)
	if _, err := os.Stat(stray); err == nil {
		t.Errorf("the peer's root name travelled: %s exists on B — a save there is "+
			"invisible to a game that only reads %s", stray, accountB)
	}
}
