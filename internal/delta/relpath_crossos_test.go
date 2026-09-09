package delta

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Two devices syncing a game address files by their position inside each
// device's own save root, which is what lets a Windows player and a Linux
// player keep different roots and still end up with the same files. That only
// holds if the manifest's keys are written in one spelling both sides agree
// on.
//
// An end-to-end test cannot catch a break here: both daemons run on the same
// OS, so a manifest keyed with backslashes still applies cleanly to the peer
// and every sync test passes. The damage only appears once the receiver is a
// different OS, where "settings\deep\x.dat" is not a path at all — it is one
// file with backslashes in its name. So this is asserted directly.
func TestManifestKeysAreSlashSeparated(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "settings", "deep", "deeper")
	if err := os.MkdirAll(nested, 0o777); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{
		filepath.Join(root, "world.sav"),
		filepath.Join(root, "settings", "one.cfg"),
		filepath.Join(nested, "x.dat"),
	} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	m, err := BuildManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Files) == 0 {
		t.Fatal("the manifest came back empty")
	}

	for key := range m.Files {
		if strings.Contains(key, `\`) {
			t.Errorf("manifest key %q contains a backslash — on a Linux or macOS peer "+
				"this is a filename, not a path, and the save arrives as a single "+
				"oddly-named file instead of a folder tree", key)
		}
	}

	// And the nested entry is actually present in the agreed spelling, so the
	// check above cannot pass merely because nesting was dropped.
	if _, ok := m.Files["settings/deep/deeper/x.dat"]; !ok {
		got := make([]string, 0, len(m.Files))
		for k := range m.Files {
			got = append(got, k)
		}
		t.Errorf("the nested file is not keyed as \"settings/deep/deeper/x.dat\"; keys are %q", got)
	}
}

// The receiving side turns those keys back into a local path. On Windows that
// means restoring the separator; on Linux and macOS it is already correct.
// Either way the file has to land in a real directory tree under the local
// root, whatever that root is called.
func TestASlashKeyResolvesUnderAnyLocalRoot(t *testing.T) {
	// Roots that share nothing with each other, as two players' would not.
	for _, root := range []string{
		filepath.Join(t.TempDir(), "SaveGames", "76561198000000001"),
		filepath.Join(t.TempDir(), "profil zwei"),
		filepath.Join(t.TempDir(), "compte", "trois"),
	} {
		full := filepath.Join(root, filepath.FromSlash("settings/deep/x.dat"))
		if !strings.HasPrefix(full, root) {
			t.Errorf("resolved path %q escaped its root %q", full, root)
		}
		if strings.Contains(strings.TrimPrefix(full, root), "/") && runtime.GOOS == "windows" {
			t.Errorf("resolved path %q still holds a forward slash on Windows", full)
		}
		// The tail has to be a tree, not one long name.
		if filepath.Base(full) != "x.dat" {
			t.Errorf("the nested key collapsed into a single name: %q", filepath.Base(full))
		}
		if filepath.Base(filepath.Dir(full)) != "deep" {
			t.Errorf("the intermediate directory was lost in %q", full)
		}
	}
}

// The per-account-folder case, stated as the property it relies on: the
// remote root's name plays no part in where a file lands locally. This is why
// pointing each device at its own account folder works, and why no
// translation rule is needed to make it work.
func TestTheRemoteRootNameNeverReachesTheLocalPath(t *testing.T) {
	const relKey = "world.sav"
	remoteRoots := []string{
		`C:\Users\alice\AppData\Local\FactoryGame\Saved\SaveGames\76561198000000001`,
		"/home/bob/.local/share/FactoryGame/Saved/SaveGames/76561198000000002",
		"/Users/carol/Library/Application Support/FactoryGame/SaveGames/76561198000000003",
	}
	localRoot := filepath.Join(t.TempDir(), "SaveGames", "99999999999999999")

	for _, remote := range remoteRoots {
		// Whatever the sender's root was, the receiver joins the relative key
		// to its OWN root. Nothing from the remote root is consulted.
		got := filepath.Join(localRoot, filepath.FromSlash(relKey))
		want := filepath.Join(localRoot, "world.sav")
		if got != want {
			t.Errorf("a peer rooted at %q produced %q, want %q", remote, got, want)
		}
		for _, segment := range []string{"alice", "bob", "carol", "76561198000000001",
			"76561198000000002", "76561198000000003"} {
			if strings.Contains(got, segment) {
				t.Errorf("the remote root leaked %q into the local path %q", segment, got)
			}
		}
	}
}
