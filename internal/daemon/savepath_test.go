package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opensave/opensave/internal/store"
)

// internal/daemon had no test files at all, and ValidateSavePath is the guard
// standing between a mistyped folder and OpenSave snapshotting, watching, and
// syncing an entire user profile. Everything downstream — the watcher's
// recursive walk, the snapshot zipper, a restore that empties the target
// first — assumes the path it was handed came past this check.
//
// These test it directly rather than through a running daemon, so a failure
// names the rule that broke.

func newTestDaemon(t *testing.T) *Daemon {
	t.Helper()
	d, err := New(Options{HomeOverride: t.TempDir(), DisableDiscovery: true})
	if err != nil {
		t.Fatalf("daemon.New: %v", err)
	}
	t.Cleanup(d.Stop)
	return d
}

func TestValidateSavePathRefusesNonFolders(t *testing.T) {
	d := newTestDaemon(t)

	for label, path := range map[string]string{
		"empty":       "",
		"whitespace":  "   ",
		"nonexistent": filepath.Join(t.TempDir(), "does", "not", "exist"),
	} {
		if _, err := d.ValidateSavePath(path); err == nil {
			t.Errorf("%s path was accepted; tracking would then watch and snapshot "+
				"something that is not there", label)
		}
	}
}

// A drive root would put every file on the volume inside one game's save.
func TestValidateSavePathRefusesADriveRoot(t *testing.T) {
	d := newTestDaemon(t)

	root := filepath.VolumeName(t.TempDir()) + string(filepath.Separator)
	if root == string(filepath.Separator) && filepath.VolumeName(t.TempDir()) == "" {
		root = "/" // unix
	}
	if _, err := d.ValidateSavePath(root); err == nil {
		t.Errorf("the drive root %q was accepted as a game's save folder", root)
	}
}

// The profile folders. Tracking one of these means snapshotting a user's
// whole Documents or AppData tree on every save the game writes.
func TestValidateSavePathRefusesProfileFolders(t *testing.T) {
	d := newTestDaemon(t)

	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home directory on this machine")
	}

	candidates := []string{
		home,
		filepath.Join(home, "Documents"),
		filepath.Join(home, "Desktop"),
		filepath.Join(home, "Downloads"),
		filepath.Join(home, "AppData"),
		filepath.Join(home, "AppData", "Roaming"),
		filepath.Join(home, "AppData", "Local"),
		filepath.Join(home, "AppData", "LocalLow"),
		filepath.Join(home, "Saved Games"),
	}
	for _, env := range []string{"WINDIR", "PROGRAMFILES", "PROGRAMDATA"} {
		if v := os.Getenv(env); v != "" {
			candidates = append(candidates, v)
		}
	}

	checked := 0
	for _, path := range candidates {
		if _, statErr := os.Stat(path); statErr != nil {
			continue // not present on this machine; nothing to assert
		}
		checked++
		if _, err := d.ValidateSavePath(path); err == nil {
			t.Errorf("%q was accepted as a game's save folder — a save there would "+
				"drag the whole profile into every snapshot", path)
		}
	}
	if checked == 0 {
		t.Skip("none of the profile folders exist here")
	}
	t.Logf("checked %d profile/system folders", checked)
}

// Only the folders themselves are refused, not everything under them. Real
// saves live inside exactly these trees, so a prefix rule would refuse most
// of the games OpenSave exists to track.
func TestValidateSavePathAllowsAGameFolderInsideAProfileFolder(t *testing.T) {
	d := newTestDaemon(t)

	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home directory")
	}
	parent := filepath.Join(home, "AppData", "LocalLow")
	if _, statErr := os.Stat(parent); statErr != nil {
		t.Skip("no AppData\\LocalLow here")
	}
	// A real folder under it, created and cleaned up.
	game := filepath.Join(parent, "opensave-savepath-test")
	if err := os.MkdirAll(game, 0o777); err != nil {
		t.Skipf("cannot create a folder under %s: %v", parent, err)
	}
	t.Cleanup(func() { os.RemoveAll(game) })

	if _, err := d.ValidateSavePath(game); err != nil {
		t.Errorf("a game folder inside AppData\\LocalLow was refused (%v) — that is "+
			"where a large share of real saves live", err)
	}
}

// Snapshotting OpenSave's own data folder would put every snapshot inside the
// thing being snapshotted.
func TestValidateSavePathRefusesItsOwnDataFolder(t *testing.T) {
	d := newTestDaemon(t)

	dataDir := d.Paths.HomeDir
	child := filepath.Join(dataDir, "backups")
	if err := os.MkdirAll(child, 0o777); err != nil {
		t.Fatal(err)
	}

	for label, path := range map[string]string{
		"the data folder itself": dataDir,
		"a folder inside it":     child,
		"its parent":             filepath.Dir(dataDir),
	} {
		if _, err := d.ValidateSavePath(path); err == nil {
			t.Errorf("%s (%s) was accepted — snapshots would recurse into themselves",
				label, path)
		}
	}
}

// One folder, one game.
func TestValidateSavePathRefusesAFolderAlreadyTracked(t *testing.T) {
	d := newTestDaemon(t)

	dir := filepath.Join(t.TempDir(), "GameSaves")
	if err := os.MkdirAll(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	abs, err := d.ValidateSavePath(dir)
	if err != nil {
		t.Fatalf("a plain folder was refused: %v", err)
	}
	if err := d.Store.CreateGame(store.Game{
		ID: "first", Name: "First Game", SavePath: abs, ActiveBranch: "main",
	}); err != nil {
		t.Fatalf("CreateGame: %v", err)
	}

	for label, alias := range map[string]string{
		"the same path":        dir,
		"a trailing separator": dir + string(filepath.Separator),
		"a .. round trip":      filepath.Join(dir, "..", "GameSaves"),
		"a different case":     strings.ToUpper(dir),
	} {
		if _, err := d.ValidateSavePath(alias); err == nil {
			t.Errorf("%s (%s) was accepted as a second game on an already-tracked "+
				"folder — two watchers would run on one save", label, alias)
		}
	}
}

// A restore target may not exist yet (importing a backup onto a fresh
// machine), but the shape rules still apply — a restore empties its target
// before unpacking, so pointing one at a profile folder is worse than
// tracking it.
func TestCheckRestoreTargetAllowsMissingPathsButNotDangerousShapes(t *testing.T) {
	d := newTestDaemon(t)

	missing := filepath.Join(t.TempDir(), "not", "created", "yet")
	if _, err := d.CheckRestoreTarget(missing); err != nil {
		t.Errorf("a not-yet-created restore target was refused (%v) — importing a "+
			"backup onto a fresh machine needs this", err)
	}

	home, herr := os.UserHomeDir()
	if herr == nil && home != "" {
		if _, err := d.CheckRestoreTarget(home); err == nil {
			t.Error("the home folder was accepted as a restore target — a restore " +
				"empties its target first")
		}
	}
	if _, err := d.CheckRestoreTarget(""); err == nil {
		t.Error("an empty restore target was accepted")
	}
}

// A junction (or symlink) naming a profile folder is a different string for
// the same directory, so the textual comparison alone did not see it: the home
// folder itself was refused and `mklink /J somewhere C:\Users\me` was
// accepted. Tracking a profile that way means the watcher walking it,
// snapshots zipping it, and — the part that actually destroys something — a
// restore emptying it before unpacking.
func TestValidateSavePathRefusesALinkToAProfileFolder(t *testing.T) {
	d := newTestDaemon(t)

	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home directory")
	}
	// The direct path must be refused, or this test proves nothing.
	if _, err := d.ValidateSavePath(home); err == nil {
		t.Fatal("setup: the home folder itself was accepted, so the link case " +
			"cannot show anything")
	}

	targets := map[string]string{"the home folder": home}
	if docs := filepath.Join(home, "Documents"); dirExists(docs) {
		targets["Documents"] = docs
	}

	for label, target := range targets {
		link := filepath.Join(t.TempDir(), "link")
		if err := makeDirLink(link, target); err != nil {
			t.Skipf("cannot create a directory link here: %v", err)
		}
		if _, err := d.ValidateSavePath(link); err == nil {
			t.Errorf("a link to %s (%s -> %s) was accepted as a game's save folder — "+
				"a restore into it would empty %s", label, link, target, target)
		}
	}
}

// The same hole around OpenSave's own data folder: snapshots living inside the
// folder being snapshotted recurse.
func TestValidateSavePathRefusesALinkToItsOwnDataFolder(t *testing.T) {
	d := newTestDaemon(t)

	link := filepath.Join(t.TempDir(), "datalink")
	if err := makeDirLink(link, d.Paths.HomeDir); err != nil {
		t.Skipf("cannot create a directory link here: %v", err)
	}
	if _, err := d.ValidateSavePath(link); err == nil {
		t.Errorf("a link to OpenSave's data folder (%s -> %s) was accepted",
			link, d.Paths.HomeDir)
	}
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}
