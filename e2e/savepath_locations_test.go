package e2e

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/opensave/opensave/testutil"
)

// Changing a game's save folder is now an edit anyone can make from the Manage
// tab, which means two devices routinely end up with roots that share nothing:
// different depths, different names, different spelling. These pin that the
// only thing which travels is a file's position INSIDE the root.

// trackAt tracks a game at an explicit folder and returns its id. The harness's
// TrackGame uses the daemon's own SaveDir, and a custom root is the point here.
func trackAt(td *testutil.TestDaemon, name, path string) string {
	td.T.Helper()
	var out struct {
		ID string `json:"id"`
	}
	td.API(http.MethodPost, "/api/games",
		map[string]string{"name": name, "savePath": path}, &out)
	if out.ID == "" {
		td.T.Fatalf("tracking %q at %s returned no id (%s)", name, path, td.LastError())
	}
	return out.ID
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Roots at different depths, with different names, holding a nested tree. The
// tree has to arrive intact and rooted at the peer's own folder — a save two
// directories down is the normal shape for these games, not an edge case.
func TestDifferingRoots_NestedTreeArrivesIntact(t *testing.T) {
	a := testutil.NewTestDaemon(t, "Roots-Nested-A")
	b := testutil.NewTestDaemon(t, "Roots-Nested-B")
	a.PairWith(b)

	// Deliberately unlike each other in depth and in every segment name.
	rootA := filepath.Join(a.SaveDir, "FactoryGame", "Saved", "SaveGames", "76561198000000001")
	rootB := filepath.Join(b.SaveDir, "profile-2")
	for _, d := range []string{rootA, rootB} {
		if err := os.MkdirAll(d, 0o777); err != nil {
			t.Fatal(err)
		}
	}

	files := map[string]string{
		"world.sav":                  "top-level",
		"backup/world_old.sav":       "one down",
		"settings/deep/nested.cfg":   "two down",
		"settings/deep/deeper/x.dat": "three down",
	}
	for rel, content := range files {
		mustWrite(t, filepath.Join(rootA, filepath.FromSlash(rel)), content)
	}

	gameA := trackAt(a, "Satisfactory", rootA)
	trackAt(b, "Satisfactory", rootB)
	syncTo(a, gameA, b.NodeID())

	for rel, want := range files {
		rel, want := rel, want
		landed := filepath.Join(rootB, filepath.FromSlash(rel))
		if !testutil.WaitFor(45*time.Second, func() bool {
			data, err := os.ReadFile(landed)
			return err == nil && string(data) == want
		}) {
			got, err := os.ReadFile(landed)
			t.Errorf("%s did not arrive under the peer's own root: got %q, err %v", rel, got, err)
		}
	}

	// None of A's root segments may be recreated on B. A save under a
	// rebuilt "SaveGames/<A's id>" is invisible to B's copy of the game.
	for _, stray := range []string{"FactoryGame", "SaveGames", "76561198000000001"} {
		if _, err := os.Stat(filepath.Join(b.SaveDir, stray)); err == nil {
			t.Errorf("A's root segment %q was recreated on B — the root travelled", stray)
		}
	}
}

// Spaces and non-ASCII in both the folder path and the file names. Users have
// these (OneDrive folders, localized profile names), and a path mangled in
// transit lands the save somewhere the game does not read.
func TestDifferingRoots_SpacesAndUnicodeSurvive(t *testing.T) {
	a := testutil.NewTestDaemon(t, "Roots-Unicode-A")
	b := testutil.NewTestDaemon(t, "Roots-Unicode-B")
	a.PairWith(b)

	rootA := filepath.Join(a.SaveDir, "My Games", "Épic Saves", "compte un")
	rootB := filepath.Join(b.SaveDir, "Mis Partidas", "cuenta dos")
	for _, d := range []string{rootA, rootB} {
		if err := os.MkdirAll(d, 0o777); err != nil {
			t.Fatal(err)
		}
	}

	files := map[string]string{
		"save one.sav":          "spaces in the name",
		"naïve/日本語.dat":         "unicode dir and name",
		"with space/a b c.save": "both",
	}
	for rel, content := range files {
		mustWrite(t, filepath.Join(rootA, filepath.FromSlash(rel)), content)
	}

	gameA := trackAt(a, "Unicode Game", rootA)
	trackAt(b, "Unicode Game", rootB)
	syncTo(a, gameA, b.NodeID())

	for rel, want := range files {
		rel, want := rel, want
		landed := filepath.Join(rootB, filepath.FromSlash(rel))
		if !testutil.WaitFor(45*time.Second, func() bool {
			data, err := os.ReadFile(landed)
			return err == nil && string(data) == want
		}) {
			got, err := os.ReadFile(landed)
			t.Errorf("%q did not survive the trip: got %q, err %v", rel, got, err)
		}
	}
}

// Empty files and binary content. An empty save is a real state (a game that
// has just created its slot), and a save file is binary far more often than
// not — a transfer that only handles text would corrupt every one of them.
func TestDifferingRoots_EmptyAndBinaryFiles(t *testing.T) {
	a := testutil.NewTestDaemon(t, "Roots-Binary-A")
	b := testutil.NewTestDaemon(t, "Roots-Binary-B")
	a.PairWith(b)

	rootA := filepath.Join(a.SaveDir, "slot", "AAA")
	rootB := filepath.Join(b.SaveDir, "slot", "BBB")
	for _, d := range []string{rootA, rootB} {
		if err := os.MkdirAll(d, 0o777); err != nil {
			t.Fatal(err)
		}
	}

	// Every byte value, including NULs and things that look like separators.
	binary := make([]byte, 512)
	for i := range binary {
		binary[i] = byte(i % 256)
	}
	if err := os.WriteFile(filepath.Join(rootA, "empty.sav"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootA, "binary.sav"), binary, 0o644); err != nil {
		t.Fatal(err)
	}

	gameA := trackAt(a, "Binary Game", rootA)
	trackAt(b, "Binary Game", rootB)
	syncTo(a, gameA, b.NodeID())

	if !testutil.WaitFor(45*time.Second, func() bool {
		got, err := os.ReadFile(filepath.Join(rootB, "binary.sav"))
		return err == nil && bytes.Equal(got, binary)
	}) {
		got, _ := os.ReadFile(filepath.Join(rootB, "binary.sav"))
		t.Errorf("binary save corrupted or missing: %d bytes arrived, want %d", len(got), len(binary))
	}
	if !testutil.WaitFor(45*time.Second, func() bool {
		st, err := os.Stat(filepath.Join(rootB, "empty.sav"))
		return err == nil && st.Size() == 0
	}) {
		t.Error("the empty save never arrived — a zero-byte file is a real save state")
	}
}

// Edits have to flow back the other way too. A world passed between two
// players moves A->B and then B->A, and the second leg uses the same
// root-independent addressing as the first.
func TestDifferingRoots_ChangesFlowBothWays(t *testing.T) {
	a := testutil.NewTestDaemon(t, "Roots-BothWays-A")
	b := testutil.NewTestDaemon(t, "Roots-BothWays-B")
	a.PairWith(b)

	rootA := filepath.Join(a.SaveDir, "acct", "111")
	rootB := filepath.Join(b.SaveDir, "different", "layout", "222")
	for _, d := range []string{rootA, rootB} {
		if err := os.MkdirAll(d, 0o777); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite(t, filepath.Join(rootA, "world.sav"), "turn-1-by-A")

	gameA := trackAt(a, "PassTheSave", rootA)
	gameB := trackAt(b, "PassTheSave", rootB)
	syncTo(a, gameA, b.NodeID())

	if !testutil.WaitFor(45*time.Second, func() bool {
		d, err := os.ReadFile(filepath.Join(rootB, "world.sav"))
		return err == nil && string(d) == "turn-1-by-A"
	}) {
		t.Fatal("first leg A->B never landed")
	}

	// B takes its turn and sends it back.
	mustWrite(t, filepath.Join(rootB, "world.sav"), "turn-2-by-B")
	syncTo(b, gameB, a.NodeID())

	if !testutil.WaitFor(45*time.Second, func() bool {
		d, err := os.ReadFile(filepath.Join(rootA, "world.sav"))
		return err == nil && string(d) == "turn-2-by-B"
	}) {
		got, _ := os.ReadFile(filepath.Join(rootA, "world.sav"))
		t.Errorf("the return leg B->A never landed: A still has %q", got)
	}
}

// Deletion is the operation with the most to lose when the roots differ: it
// resolves a peer-supplied relative path and removes what it finds. It has to
// remove the right file under the local root, and nothing outside it.
func TestDifferingRoots_DeletionPropagatesAndStaysInsideTheRoot(t *testing.T) {
	a := testutil.NewTestDaemon(t, "Roots-Delete-A")
	b := testutil.NewTestDaemon(t, "Roots-Delete-B")
	a.PairWith(b)

	rootA := filepath.Join(a.SaveDir, "acctA", "inner")
	rootB := filepath.Join(b.SaveDir, "totally", "other", "acctB")
	for _, d := range []string{rootA, rootB} {
		if err := os.MkdirAll(d, 0o777); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite(t, filepath.Join(rootA, "keep.sav"), "keep me")
	mustWrite(t, filepath.Join(rootA, "old", "drop.sav"), "delete me")

	// A file OUTSIDE both roots, to prove a delete cannot wander up and out.
	bystander := filepath.Join(b.SaveDir, "bystander.txt")
	mustWrite(t, bystander, "not part of any save")

	gameA := trackAt(a, "DeleteGame", rootA)
	gameB := trackAt(b, "DeleteGame", rootB)
	syncTo(a, gameA, b.NodeID())

	if !testutil.WaitFor(45*time.Second, func() bool {
		_, err := os.Stat(filepath.Join(rootB, "old", "drop.sav"))
		return err == nil
	}) {
		t.Fatal("setup: the file to be deleted never reached B")
	}

	// Now remove it on A and sync again.
	if err := os.Remove(filepath.Join(rootA, "old", "drop.sav")); err != nil {
		t.Fatal(err)
	}
	a.API(http.MethodPost, "/api/games/"+gameA+"/snapshot",
		map[string]string{"comment": "dropped a save"}, nil)
	syncTo(a, gameA, b.NodeID())

	if !testutil.WaitFor(45*time.Second, func() bool {
		_, err := os.Stat(filepath.Join(rootB, "old", "drop.sav"))
		return os.IsNotExist(err)
	}) {
		t.Errorf("the deletion never reached B — %s is still there",
			filepath.Join(rootB, "old", "drop.sav"))
	}

	// The other save, and anything outside the root, must be untouched.
	if got, err := os.ReadFile(filepath.Join(rootB, "keep.sav")); err != nil || string(got) != "keep me" {
		t.Errorf("an unrelated save was disturbed: %q, err %v", got, err)
	}
	if _, err := os.Stat(bystander); err != nil {
		t.Errorf("a file outside the save root was removed: %v — a peer-supplied "+
			"relative path escaped the root", err)
	}
	_ = gameB
}

// Same file name at several depths. Files are addressed by their relative
// path, so these are four distinct saves — if anything keyed them by base
// name instead, they would collapse into one and three would be lost.
func TestDifferingRoots_SameNameAtDifferentDepthsStayDistinct(t *testing.T) {
	a := testutil.NewTestDaemon(t, "Roots-Collide-A")
	b := testutil.NewTestDaemon(t, "Roots-Collide-B")
	a.PairWith(b)

	rootA := filepath.Join(a.SaveDir, "one", "AAA")
	rootB := filepath.Join(b.SaveDir, "two", "three", "BBB")
	for _, d := range []string{rootA, rootB} {
		if err := os.MkdirAll(d, 0o777); err != nil {
			t.Fatal(err)
		}
	}
	same := map[string]string{
		"save.sav":              "root copy",
		"slot1/save.sav":        "slot one",
		"slot2/save.sav":        "slot two",
		"slot2/nested/save.sav": "slot two nested",
	}
	for rel, content := range same {
		mustWrite(t, filepath.Join(rootA, filepath.FromSlash(rel)), content)
	}

	gameA := trackAt(a, "CollideGame", rootA)
	trackAt(b, "CollideGame", rootB)
	syncTo(a, gameA, b.NodeID())

	for rel, want := range same {
		rel, want := rel, want
		landed := filepath.Join(rootB, filepath.FromSlash(rel))
		if !testutil.WaitFor(45*time.Second, func() bool {
			d, err := os.ReadFile(landed)
			return err == nil && string(d) == want
		}) {
			got, err := os.ReadFile(landed)
			t.Errorf("%s = %q (err %v), want %q — four saves share a base name and "+
				"must stay distinct", rel, got, err, want)
		}
	}
}

// Volume. A real save folder is not three files: emulator and Unreal saves run
// to hundreds across many directories, and the per-file bookkeeping that makes
// root-independent addressing work has to hold at that size.
func TestDifferingRoots_ManyFilesAcrossManyDirectories(t *testing.T) {
	a := testutil.NewTestDaemon(t, "Roots-Volume-A")
	b := testutil.NewTestDaemon(t, "Roots-Volume-B")
	a.PairWith(b)

	rootA := filepath.Join(a.SaveDir, "vol", "AAA")
	rootB := filepath.Join(b.SaveDir, "elsewhere", "entirely", "BBB")
	for _, d := range []string{rootA, rootB} {
		if err := os.MkdirAll(d, 0o777); err != nil {
			t.Fatal(err)
		}
	}

	want := map[string]string{}
	for dir := 0; dir < 12; dir++ {
		for f := 0; f < 15; f++ {
			rel := fmt.Sprintf("bank%02d/slot%02d.sav", dir, f)
			content := fmt.Sprintf("dir-%d-file-%d", dir, f)
			want[rel] = content
			mustWrite(t, filepath.Join(rootA, filepath.FromSlash(rel)), content)
		}
	}

	gameA := trackAt(a, "VolumeGame", rootA)
	trackAt(b, "VolumeGame", rootB)
	syncTo(a, gameA, b.NodeID())

	// Wait on the whole set, then report precisely what is missing — a
	// per-file wait would spend 45s on each of 180 files in the bad case.
	ok := testutil.WaitFor(90*time.Second, func() bool {
		for rel, content := range want {
			d, err := os.ReadFile(filepath.Join(rootB, filepath.FromSlash(rel)))
			if err != nil || string(d) != content {
				return false
			}
		}
		return true
	})
	if !ok {
		missing, wrong := 0, 0
		for rel, content := range want {
			d, err := os.ReadFile(filepath.Join(rootB, filepath.FromSlash(rel)))
			switch {
			case err != nil:
				missing++
			case string(d) != content:
				wrong++
			}
		}
		t.Errorf("of %d files across 12 directories, %d never arrived and %d arrived with "+
			"the wrong contents", len(want), missing, wrong)
	}
}
