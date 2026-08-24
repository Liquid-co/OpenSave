package presets

import (
	"os"
	"path/filepath"
	"testing"
)

func mkWrapperDirs(t *testing.T, root string, rel ...string) {
	t.Helper()
	for _, r := range rel {
		if err := os.MkdirAll(filepath.Join(root, r), 0o777); err != nil {
			t.Fatal(err)
		}
	}
}

// %USERPROFILE%\Saved Games holds a mix of games and studios. Offering the
// studio names the row after the publisher, leaves it without cover art (a
// studio has no Steam AppID), and where a studio ships two titles puts both in
// one synced unit — so a rollback of either rolls back both.
func TestAPublisherFolderResolvesToTheGameInsideIt(t *testing.T) {
	root := t.TempDir()
	mkWrapperDirs(t, root, filepath.Join("CD Projekt Red", "Cyberpunk 2077"))
	known := map[string]string{"cyberpunk 2077": "1091500"}

	sc := &Scanner{}
	child, ok := sc.resolveWrapperChild(root, "CD Projekt Red", known)
	if !ok || child != "Cyberpunk 2077" {
		t.Fatalf("resolveWrapperChild = (%q, %v), want (\"Cyberpunk 2077\", true)", child, ok)
	}
}

// A folder the manifest recognises is never descended into. "God of War" and
// "The Last of Us Part I" each hold one subfolder here — a profile id — and
// descending would offer that id as the game.
func TestAKnownGameIsNeverDescendedInto(t *testing.T) {
	root := t.TempDir()
	mkWrapperDirs(t, root, filepath.Join("God of War", "76561198000000000"))
	known := map[string]string{"god of war": "1593500"}

	sc := &Scanner{}
	if child, ok := sc.resolveWrapperChild(root, "God of War", known); ok {
		t.Errorf("descended into a known game, offering %q as the title", child)
	}
}

// An unknown folder with no known child is left exactly as it was.
// "ThomasAndFriends" and "TrainSimWorld2EGS" are real saves the manifest has
// never heard of, and guessing at them would lose them.
func TestAnUnknownFolderWithNoKnownChildIsLeftAlone(t *testing.T) {
	root := t.TempDir()
	mkWrapperDirs(t, root, filepath.Join("ThomasAndFriends", "Saved"))
	known := map[string]string{"cyberpunk 2077": "1091500"}

	sc := &Scanner{}
	if child, ok := sc.resolveWrapperChild(root, "ThomasAndFriends", known); ok {
		t.Errorf("descended into an unrecognised folder, offering %q", child)
	}
}

// Two known children means the folder really does hold several games, and
// picking one would silently drop the other. The caller keeps the folder.
func TestAStudioHoldingTwoGamesIsNotResolvedToOne(t *testing.T) {
	root := t.TempDir()
	mkWrapperDirs(t, root,
		filepath.Join("Big Studio", "Game One"),
		filepath.Join("Big Studio", "Game Two"),
	)
	known := map[string]string{"game one": "1", "game two": "2"}

	sc := &Scanner{}
	if child, ok := sc.resolveWrapperChild(root, "Big Studio", known); ok {
		t.Errorf("resolved a two-game studio folder to just %q", child)
	}
}

// Punctuation differs constantly between a save folder and a store name, and
// the shared normaliser is what makes them agree: it drops an apostrophe
// without joining the words either side, so the manifest's "Baldur's Gate 3"
// and a folder called "Baldurs Gate 3" land on one key.
func TestMatchingIgnoresPunctuation(t *testing.T) {
	root := t.TempDir()
	mkWrapperDirs(t, root, filepath.Join("Some Studio", "Baldurs Gate 3"))
	known := map[string]string{normalizeGameName("Baldur's Gate 3"): "1086940"}

	sc := &Scanner{}
	child, ok := sc.resolveWrapperChild(root, "Some Studio", known)
	if !ok || child != "Baldurs Gate 3" {
		t.Errorf("resolveWrapperChild = (%q, %v), want (\"Baldurs Gate 3\", true) — "+
			"the two spellings must normalise to the same key", child, ok)
	}
}

// With no manifest (hermetic tests, or a first run before any download) the
// rule must simply not fire rather than guessing.
func TestNoManifestMeansNoDescending(t *testing.T) {
	root := t.TempDir()
	mkWrapperDirs(t, root, filepath.Join("CD Projekt Red", "Cyberpunk 2077"))

	sc := &Scanner{}
	if _, ok := sc.resolveWrapperChild(root, "CD Projekt Red", nil); ok {
		t.Error("descended with no manifest to justify it")
	}
}
