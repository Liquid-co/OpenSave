package e2e

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/opensave/opensave/testutil"
)

// Obscure shapes a save folder can take once the folder is user-editable.
// These are deliberately awkward: single-file saves, one side a file and the
// other a directory, names that differ only in case. Each has a code path of
// its own, and none of them is exercised by an ordinary sync test.

// A save that is one file rather than a folder is a supported shape
// (delta.ResolveLocalSaveFilePath), and BuildManifest keys it by the file's
// base name rather than by a path. Two devices that name that file
// differently — an emulator writing <profile>.sav — therefore key the same
// logical save under different names, and the engine sees each side holding a
// file the other lacks.
//
// Measured behaviour: it reports a conflict on every sync and never
// converges, even when both files start byte-identical. That is a real
// limitation (the fix for a user is to track the containing FOLDER, which
// works — see the directory tests), but it is the safe failure: what must
// never happen is a silent winner or B's file disappearing.
//
// So this pins the safety, not convergence. If someone later makes these
// converge, this test should be rewritten to assert that — deliberately.
func TestObscure_SingleFileSavesWithDifferentNames(t *testing.T) {
	a := testutil.NewTestDaemon(t, "Obscure-File-A")
	b := testutil.NewTestDaemon(t, "Obscure-File-B")
	a.PairWith(b)

	dirA := filepath.Join(a.SaveDir, "emu", "saves")
	dirB := filepath.Join(b.SaveDir, "emulator", "saves")
	for _, d := range []string{dirA, dirB} {
		if err := os.MkdirAll(d, 0o777); err != nil {
			t.Fatal(err)
		}
	}
	fileA := filepath.Join(dirA, "alice.sav")
	fileB := filepath.Join(dirB, "bob.sav")
	// Starting from agreement. Two single-file saves that already differ have
	// no shared history, and the engine correctly answers "conflict" rather
	// than picking a winner — that is not what this test is about. The
	// question here is whether a later edit crosses between two files whose
	// NAMES differ, since the manifest keys a single-file save by base name.
	mustWrite(t, fileA, "shared start")
	mustWrite(t, fileB, "shared start")

	gameA := trackAt(a, "EmuGame", fileA)
	trackAt(b, "EmuGame", fileB)
	syncTo(a, gameA, b.NodeID())

	mustWrite(t, fileA, "A's world")
	a.API(http.MethodPost, "/api/games/"+gameA+"/snapshot",
		map[string]string{"comment": "edited"}, nil)
	status, _ := syncTo(a, gameA, b.NodeID())

	// Whatever it decides, it must decide it out loud. A sync that answered
	// "in_sync" here would be claiming agreement between two files that hold
	// different bytes.
	if status != "conflict" {
		t.Errorf("sync answered %q; two single-file saves under different names hold "+
			"different bytes and cannot silently be called agreed", status)
	}

	// B's own save must survive untouched. Losing it is the outcome that
	// would actually cost someone their game.
	got, err := os.ReadFile(fileB)
	if err != nil {
		t.Fatalf("B's own save file is gone after syncing with a peer whose "+
			"single-file save has a different name: %v", err)
	}
	if string(got) != "shared start" && string(got) != "A's world" {
		t.Errorf("B's save = %q — neither its own content nor the peer's, so something "+
			"overwrote it with a third thing", got)
	}
	// And A's filename must not be conjured up beside B's save: the manifest
	// key is a name, not a path, and writing it as one would leave a second
	// save file the game never reads.
	if _, err := os.Stat(filepath.Join(dirB, "alice.sav")); err == nil {
		t.Errorf("A's filename was recreated in B's folder — the manifest key was " +
			"treated as a path rather than a name")
	}
}

// One side tracks a single file, the other a whole folder. Nothing prevents a
// user setting this up from the Manage tab, and the two sides then disagree
// about what the save even is.
func TestObscure_OneSideFileOtherSideDirectory(t *testing.T) {
	a := testutil.NewTestDaemon(t, "Obscure-Mixed-A")
	b := testutil.NewTestDaemon(t, "Obscure-Mixed-B")
	a.PairWith(b)

	dirA := filepath.Join(a.SaveDir, "single")
	if err := os.MkdirAll(dirA, 0o777); err != nil {
		t.Fatal(err)
	}
	fileA := filepath.Join(dirA, "only.sav")
	mustWrite(t, fileA, "from the file side")

	dirB := filepath.Join(b.SaveDir, "folder", "BBB")
	if err := os.MkdirAll(dirB, 0o777); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dirB, "existing.sav"), "B had this")

	gameA := trackAt(a, "MixedGame", fileA)
	gameB := trackAt(b, "MixedGame", dirB)
	syncTo(a, gameA, b.NodeID())

	// Settled rather than polled for an arrival. What this test asserts is
	// what must NOT have happened, and there is no file whose appearance
	// marks the end — so polling for one only ever burns its whole timeout
	// and slows the suite down for no signal. SettleSync returns the moment
	// both devices are quiet.
	testutil.SettleSync(t, gameA, a)
	testutil.SettleSync(t, gameB, b)

	// The load-bearing assertion is not which layout wins, but that B's
	// existing save is not destroyed by a mismatch neither side asked for.
	if _, err := os.Stat(filepath.Join(dirB, "existing.sav")); err != nil {
		t.Errorf("B's pre-existing save was removed when a file-shaped peer synced "+
			"into a folder-shaped save: %v", err)
	}
	// And B's root must still be a directory — replacing it with a file would
	// take the whole save folder with it.
	if st, err := os.Stat(dirB); err != nil || !st.IsDir() {
		t.Errorf("B's save folder is no longer a directory (err %v) — a file-shaped "+
			"peer overwrote the root itself", err)
	}
}

// Two files whose names differ only in case. Legal and distinct on Linux and
// macOS; the same file on Windows. A peer holding both sends two entries that
// collide on arrival, and the second write silently lands on the first.
func TestObscure_NamesDifferingOnlyInCase(t *testing.T) {
	a := testutil.NewTestDaemon(t, "Obscure-Case-A")
	b := testutil.NewTestDaemon(t, "Obscure-Case-B")
	a.PairWith(b)

	rootA := filepath.Join(a.SaveDir, "case", "AAA")
	rootB := filepath.Join(b.SaveDir, "case", "BBB")
	for _, d := range []string{rootA, rootB} {
		if err := os.MkdirAll(d, 0o777); err != nil {
			t.Fatal(err)
		}
	}

	// On a case-insensitive filesystem the second write just replaces the
	// first, and only one file exists. That is itself the situation under
	// test: whatever arrives must be internally consistent.
	mustWrite(t, filepath.Join(rootA, "Save.sav"), "upper S")
	mustWrite(t, filepath.Join(rootA, "save.sav"), "lower s")

	entries, _ := os.ReadDir(rootA)
	t.Logf("sender holds %d file(s) differing only in case", len(entries))

	gameA := trackAt(a, "CaseGame", rootA)
	trackAt(b, "CaseGame", rootB)
	syncTo(a, gameA, b.NodeID())

	// However many the sender has, the receiver must end up with the same
	// count — not a file that keeps flipping contents on every sync.
	if !testutil.WaitFor(45*time.Second, func() bool {
		got, err := os.ReadDir(rootB)
		return err == nil && len(got) == len(entries)
	}) {
		got, _ := os.ReadDir(rootB)
		names := []string{}
		for _, e := range got {
			names = append(names, e.Name())
		}
		t.Errorf("sender has %d file(s), receiver has %d (%s) — a case-only "+
			"difference did not survive the round trip",
			len(entries), len(got), strings.Join(names, ", "))
	}
}

// The working half of the same shape, and the reason the limitation above is
// a limitation rather than a broken feature: when both devices name the file
// the same, a single-file save syncs like any other. Worth pinning, because
// the single-file path is a separate branch in both the manifest builder and
// the pull loop, and nothing else covers it.
func TestObscure_SingleFileSavesWithMatchingNamesDoSync(t *testing.T) {
	a := testutil.NewTestDaemon(t, "Obscure-FileOK-A")
	b := testutil.NewTestDaemon(t, "Obscure-FileOK-B")
	a.PairWith(b)

	// Same file name, but reached through folders that share nothing — the
	// combination the Manage tab makes easy to create.
	dirA := filepath.Join(a.SaveDir, "emu", "profileA")
	dirB := filepath.Join(b.SaveDir, "another", "layout", "profileB")
	for _, d := range []string{dirA, dirB} {
		if err := os.MkdirAll(d, 0o777); err != nil {
			t.Fatal(err)
		}
	}
	fileA := filepath.Join(dirA, "game.srm")
	fileB := filepath.Join(dirB, "game.srm")
	mustWrite(t, fileA, "shared start")
	mustWrite(t, fileB, "shared start")

	gameA := trackAt(a, "SRMGame", fileA)
	trackAt(b, "SRMGame", fileB)
	syncTo(a, gameA, b.NodeID())

	mustWrite(t, fileA, "A played a turn")
	a.API(http.MethodPost, "/api/games/"+gameA+"/snapshot",
		map[string]string{"comment": "turn"}, nil)
	syncTo(a, gameA, b.NodeID())

	if !testutil.WaitFor(45*time.Second, func() bool {
		d, err := os.ReadFile(fileB)
		return err == nil && string(d) == "A played a turn"
	}) {
		got, err := os.ReadFile(fileB)
		t.Errorf("a single-file save did not reach the peer: B = %q (err %v)", got, err)
	}
	// It must land in B's own file, not create A's folder layout on B.
	if _, err := os.Stat(filepath.Join(b.SaveDir, "emu")); err == nil {
		t.Error("A's folder layout was recreated on B")
	}
}

// One folder, one game. The guard exists because two trackers on the same
// folder means two watchers, duplicate snapshots, and a device syncing against
// itself — and a user-editable save path is the easy way to create that by
// accident, by pointing a second game at a folder already tracked.
//
// The textual comparison catches the spellings pinned below. Aliases that no
// string comparison can see — a junction or symlink naming the same directory
// — are caught separately by os.SameFile, and covered by the two junction
// tests further down.
func TestObscure_TheSameFolderCannotBeTrackedTwice(t *testing.T) {
	d := testutil.NewTestDaemon(t, "Obscure-Dupe")

	real := filepath.Join(d.SaveDir, "RealGame")
	if err := os.MkdirAll(real, 0o777); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(real, "save.sav"), "x")
	trackAt(d, "FirstCopy", real)

	for label, alias := range map[string]string{
		"a trailing separator": real + string(os.PathSeparator),
		"a .. round trip":      filepath.Join(real, "..", "RealGame"),
		"upper case":           strings.ToUpper(real),
		"lower case":           strings.ToLower(real),
	} {
		var out struct {
			ID string `json:"id"`
		}
		code := d.APIStatus(http.MethodPost, "/api/games",
			map[string]string{"name": "Second-" + label, "savePath": alias}, &out)
		if code >= 200 && code < 300 && out.ID != "" {
			t.Errorf("%s (%s) was accepted as a second game on an already-tracked "+
				"folder — two watchers now run on one save", label, alias)
		}
	}
}

// The alias the textual comparison could not see. A junction (or a symlink)
// names the same directory by a different string, so before save paths were
// canonicalised it was accepted as a second game on an already-tracked
// folder — two watchers, two sets of snapshots, one save.
//
// Windows users hit this by relocating a save folder to another drive with
// `mklink /J`, which is the standard trick for freeing space on C:.
func TestObscure_AJunctionCannotDoubleTrackAFolder(t *testing.T) {
	d := testutil.NewTestDaemon(t, "Obscure-Junction")

	real := filepath.Join(d.SaveDir, "RealGame")
	if err := os.MkdirAll(real, 0o777); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(real, "save.sav"), "x")

	link := filepath.Join(d.SaveDir, "LinkedGame")
	if err := makeDirLink(link, real); err != nil {
		t.Skipf("cannot create a directory link on this machine: %v", err)
	}

	trackAt(d, "FirstCopy", real)

	var out struct {
		ID string `json:"id"`
	}
	code := d.APIStatus(http.MethodPost, "/api/games",
		map[string]string{"name": "ViaTheLink", "savePath": link}, &out)
	if code >= 200 && code < 300 && out.ID != "" {
		t.Errorf("a junction to an already-tracked folder was accepted as a second "+
			"game (id %s) — %s and %s are the same directory, so two watchers now "+
			"run on one save", out.ID, link, real)
	}
}

// And the reverse order: the link tracked first, the real path second. The
// stored path is the alias here, so this only passes if BOTH sides of the
// comparison are canonicalised rather than just the incoming one.
func TestObscure_TheRealPathCannotDoubleTrackAJunction(t *testing.T) {
	d := testutil.NewTestDaemon(t, "Obscure-Junction-Rev")

	real := filepath.Join(d.SaveDir, "RealGame2")
	if err := os.MkdirAll(real, 0o777); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(real, "save.sav"), "x")

	link := filepath.Join(d.SaveDir, "LinkedGame2")
	if err := makeDirLink(link, real); err != nil {
		t.Skipf("cannot create a directory link on this machine: %v", err)
	}

	trackAt(d, "ViaTheLinkFirst", link)

	var out struct {
		ID string `json:"id"`
	}
	code := d.APIStatus(http.MethodPost, "/api/games",
		map[string]string{"name": "RealSecond", "savePath": real}, &out)
	if code >= 200 && code < 300 && out.ID != "" {
		t.Errorf("the real folder was accepted as a second game (id %s) when its "+
			"junction %s was already tracked — the stored path was not canonicalised",
			out.ID, link)
	}
}

// macOS spells filenames decomposed and Windows/Linux spell them composed, so
// the same save arrives under a name the receiver does not recognise. Before
// manifest keys were normalised, each side pulled the other's spelling and the
// folder ended up with two files that render identically — and neither side
// ever converged, because every sync re-added the pair.
//
// Stageable on one machine: NTFS stores both spellings happily (verified —
// writing both produces two directory entries), so A's file is named the way a
// Mac would name it and B's the way Windows would.
const (
	nfcCafe = "caf\u00e9.sav"  // composed: Windows, Linux
	nfdCafe = "cafe\u0301.sav" // decomposed: macOS
)

func TestObscure_DecomposedAndComposedNamesAreOneFile(t *testing.T) {
	a := testutil.NewTestDaemon(t, "Obscure-NFD-A")
	b := testutil.NewTestDaemon(t, "Obscure-NFC-B")
	a.PairWith(b)

	rootA := filepath.Join(a.SaveDir, "mac", "AAA")
	rootB := filepath.Join(b.SaveDir, "win", "BBB")
	for _, d := range []string{rootA, rootB} {
		if err := os.MkdirAll(d, 0o777); err != nil {
			t.Fatal(err)
		}
	}
	// Byte-identical contents, spelled as each platform would spell the name.
	mustWrite(t, filepath.Join(rootA, nfdCafe), "shared save")
	mustWrite(t, filepath.Join(rootB, nfcCafe), "shared save")

	gameA := trackAt(a, "AccentGame", rootA)
	trackAt(b, "AccentGame", rootB)
	status, _ := syncTo(a, gameA, b.NodeID())

	// The load-bearing assertion. Two devices holding the SAME save must be
	// recognised as agreeing. Without a shared spelling each side sees a file
	// the other lacks, and the divergence guard answers "conflict" — on this
	// sync and on every one after it, so the pair never converges and no edit
	// ever crosses. Measured: that is exactly what happened before manifest
	// keys were composed.
	if status == "conflict" {
		t.Errorf("sync answered %q for two devices holding identical saves whose "+
			"filenames differ only in Unicode normalisation — they can never "+
			"converge while this is a conflict", status)
	}

	// And neither side may grow a second copy under the other's spelling.
	for label, root := range map[string]string{"sender": rootA, "receiver": rootB} {
		ents, err := os.ReadDir(root)
		if err != nil {
			t.Fatal(err)
		}
		if len(ents) != 1 {
			names := []string{}
			for _, e := range ents {
				names = append(names, fmt.Sprintf("% x", []byte(e.Name())))
			}
			t.Errorf("the %s holds %d files (%s) — the two spellings of one save "+
				"became two files", label, len(ents), strings.Join(names, " | "))
		}
	}
}

// And an edit still crosses: matching the two spellings is only useful if the
// receiver's existing file is the one updated, rather than a second file being
// created beside it under the agreed spelling.
func TestObscure_AnEditReachesTheDifferentlySpelledFile(t *testing.T) {
	a := testutil.NewTestDaemon(t, "Obscure-NFD-Edit-A")
	b := testutil.NewTestDaemon(t, "Obscure-NFC-Edit-B")
	a.PairWith(b)

	rootA := filepath.Join(a.SaveDir, "mac2")
	rootB := filepath.Join(b.SaveDir, "win2")
	for _, d := range []string{rootA, rootB} {
		if err := os.MkdirAll(d, 0o777); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite(t, filepath.Join(rootA, nfdCafe), "shared save")
	mustWrite(t, filepath.Join(rootB, nfcCafe), "shared save")

	gameA := trackAt(a, "AccentEdit", rootA)
	trackAt(b, "AccentEdit", rootB)
	syncTo(a, gameA, b.NodeID())

	mustWrite(t, filepath.Join(rootA, nfdCafe), "edited on the Mac")
	a.API(http.MethodPost, "/api/games/"+gameA+"/snapshot",
		map[string]string{"comment": "edit"}, nil)
	syncTo(a, gameA, b.NodeID())

	// The receiver's own file — the composed one it already had — must hold
	// the edit, and it must still be the only file there.
	if !testutil.WaitFor(45*time.Second, func() bool {
		d, err := os.ReadFile(filepath.Join(rootB, nfcCafe))
		return err == nil && string(d) == "edited on the Mac"
	}) {
		got, err := os.ReadFile(filepath.Join(rootB, nfcCafe))
		t.Errorf("the receiver's own spelling was not updated: %q (err %v)", got, err)
	}
	if ents, err := os.ReadDir(rootB); err == nil && len(ents) != 1 {
		names := []string{}
		for _, e := range ents {
			names = append(names, fmt.Sprintf("% x", []byte(e.Name())))
		}
		t.Errorf("the receiver holds %d files (%s) — the edit created a second copy "+
			"instead of updating the existing one", len(ents), strings.Join(names, " | "))
	}
}
