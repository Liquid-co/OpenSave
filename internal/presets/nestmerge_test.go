package presets

import (
	"path/filepath"
	"strings"
	"testing"
)

func disc(name, appID, path string) DiscoveredSave {
	return DiscoveredSave{
		ID: name, Name: name, AppID: appID, SavePath: hostPath(path), Type: "game",
		Measured: true, FileCount: 1,
	}
}

// hostPath rewrites the Windows-shaped paths these tests are written with into
// the separator the host actually uses.
//
// The paths below are real examples from real machines, and they read best as
// the users' own spelling — but nesting is detected with filepath.Separator,
// which is correct (a backslash is an ordinary character in a Linux filename,
// so accepting it as a separator there would be wrong). Left literal, every
// one of these tests silently became Windows-only: on Linux no path was ever a
// prefix of another, nothing nested, and five tests failed on the first CI run
// that included them.
func hostPath(p string) string {
	return strings.ReplaceAll(p, `\`, string(filepath.Separator))
}

func groupsOf(saves []DiscoveredSave) map[string][]string {
	out := map[string][]string{}
	for _, s := range saves {
		out[s.GroupID] = append(out[s.GroupID], s.Name)
	}
	return out
}

// The same game found twice under two names, which groupKey cannot join
// because one row has an AppID and the other has only a folder name. A save
// folder inside a save folder is one game's data.
func TestNestedFoldersBecomeOneGame(t *testing.T) {
	saves := []DiscoveredSave{
		disc("MOUSE", "", `C:\LocalLow\Fumi Games\MOUSE`),
		disc("MOUSE: P.I. For Hire", "2416450", `C:\LocalLow\Fumi Games\MOUSE\Save`),
	}
	Group(saves)

	if saves[0].GroupID != saves[1].GroupID {
		t.Fatalf("nested folders stayed in separate groups: %v", groupsOf(saves))
	}
	// The row a user tracks has to carry what the group worked out, or the
	// merge changes nothing where it matters.
	if saves[0].AppID != "2416450" {
		t.Errorf("the outer row has AppID %q, want the group's 2416450 — tracking it "+
			"would produce a game with no cover", saves[0].AppID)
	}
	if saves[0].Name != "MOUSE: P.I. For Hire" {
		t.Errorf("the outer row is named %q, want the store name", saves[0].Name)
	}
}

// An Unreal game is discovered under its project codename, and the manifest
// entry for the real title points inside that folder. Containment resolves the
// codename with no mapping table anywhere.
func TestAnUnrealCodenameResolvesByContainment(t *testing.T) {
	saves := []DiscoveredSave{
		disc("Sandfall (Epic/Unreal Save)", "", `C:\Local\Sandfall\Saved\SaveGames`),
		disc("Clair Obscur: Expedition 33", "1903340", `C:\Local\Sandfall\Saved\SaveGames\76561197960271872`),
	}
	Group(saves)

	if saves[0].AppID != "1903340" || saves[0].Name != "Clair Obscur: Expedition 33" {
		t.Errorf("the codename row is %q/%q, want the real title and its AppID",
			saves[0].Name, saves[0].AppID)
	}
}

// Two games really can nest. Windows paths are case-insensitive, so
// Documents\Trackmania and Documents\TrackMania are one directory — and the
// 2008 game's folders sit inside the 2020 game's.
//
// Merging them would offer both as one thing to track, making a single sync
// unit out of two games' saves. Differing AppIDs say they are different games,
// and that outranks nesting.
func TestTwoGamesThatNestAreNotMerged(t *testing.T) {
	saves := []DiscoveredSave{
		disc("Trackmania (2020)", "2225070", `C:\Documents\Trackmania`),
		disc("Trackmania United Forever (Profiles)", "7200", `C:\Documents\TrackMania\Profiles`),
		disc("Trackmania United Forever (Scores)", "7200", `C:\Documents\TrackMania\Scores`),
	}
	Group(saves)

	if saves[0].GroupID == saves[1].GroupID {
		t.Errorf("two games with different AppIDs were merged: %v", groupsOf(saves))
	}
	if saves[1].GroupID != saves[2].GroupID {
		t.Errorf("one game's own folders were split apart: %v", groupsOf(saves))
	}
	// And neither may be renamed into the other.
	if saves[1].Name == saves[0].Name || saves[1].AppID != "7200" {
		t.Errorf("United Forever became %q/%q", saves[1].Name, saves[1].AppID)
	}
}

// A shared parent is not sameness. Everything under Documents\My Games shares
// one, and treating that as evidence would merge a whole library.
func TestSiblingsUnderOneParentAreNotMerged(t *testing.T) {
	saves := []DiscoveredSave{
		disc("Game A", "", `C:\Documents\My Games\GameA`),
		disc("Game B", "", `C:\Documents\My Games\GameB`),
		disc("Game C", "111", `C:\Documents\My Games\GameC`),
	}
	Group(saves)

	seen := map[string]bool{}
	for _, s := range saves {
		if seen[s.GroupID] {
			t.Fatalf("siblings were merged into one game: %v", groupsOf(saves))
		}
		seen[s.GroupID] = true
	}
}

// A row with no AppID must not take one from a nested row that has none
// either, and must not be renamed by it.
func TestMergingTwoUnknownRowsInventsNothing(t *testing.T) {
	saves := []DiscoveredSave{
		disc("Outer", "", `C:\Games\Thing`),
		disc("Inner", "", `C:\Games\Thing\Saves`),
	}
	Group(saves)

	if saves[0].GroupID != saves[1].GroupID {
		t.Error("nested folders with no AppID should still be one game")
	}
	if saves[0].AppID != "" || saves[1].AppID != "" {
		t.Errorf("an AppID appeared from nowhere: %q / %q", saves[0].AppID, saves[1].AppID)
	}
}

// A chain deeper than two levels has to end up as one game, not two pairs.
func TestADeepChainBecomesOneGame(t *testing.T) {
	saves := []DiscoveredSave{
		disc("Top", "", `C:\Games\Deep`),
		disc("Middle", "", `C:\Games\Deep\Saved`),
		disc("Bottom", "555", `C:\Games\Deep\Saved\SaveGames\Slot1`),
	}
	Group(saves)

	if saves[0].GroupID != saves[1].GroupID || saves[1].GroupID != saves[2].GroupID {
		t.Fatalf("a nesting chain did not collapse to one game: %v", groupsOf(saves))
	}
	if saves[0].AppID != "555" {
		t.Errorf("the AppID did not reach the top of the chain: %q", saves[0].AppID)
	}
}

// Rows that do not nest and share no key keep their own identity.
func TestUnrelatedGamesAreLeftAlone(t *testing.T) {
	saves := []DiscoveredSave{
		disc("One", "", `C:\A\One`),
		disc("Two", "", `D:\B\Two`),
	}
	Group(saves)
	if saves[0].GroupID == saves[1].GroupID {
		t.Error("unrelated games were merged")
	}
}
