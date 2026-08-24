package presets

import "testing"

func measured(id, name, appID, path string, files int) DiscoveredSave {
	return DiscoveredSave{
		ID: id, Name: name, AppID: appID, SavePath: path, Type: "game",
		Measured: true, FileCount: files,
	}
}

// The usual case: a game's save lives in one folder and the others are noise a
// launcher or a crack created speculatively. Those go.
func TestRedundantEmptiesAreDroppedWhenTheGameHasARealFolder(t *testing.T) {
	saves := []DiscoveredSave{
		measured("g1", "Balatro", "2379780", `C:\a\remote`, 12),
		measured("g1", "Balatro", "2379780", `C:\b\remote`, 0),
		measured("g1", "Balatro", "2379780", `C:\c\remote`, 0),
	}
	kept := WithoutRedundantEmpty(saves)
	if len(kept) != 1 || kept[0].SavePath != `C:\a\remote` {
		t.Fatalf("want only the folder holding something, got %+v", paths(kept))
	}
	if n := CountRedundantEmpty(saves); n != 2 {
		t.Errorf("CountRedundantEmpty = %d, want 2", n)
	}
}

// The case that made a 2.2.x upgrade look like lost detection: nothing has
// been written to ANY of a title's folders yet, so its empties are the only
// rows it has. Dropping them removes the game from the listing rather than
// tidying it.
func TestATitleWithNothingAnywhereKeepsItsRows(t *testing.T) {
	saves := []DiscoveredSave{
		measured("g1", "Elden Ring", "1245620", `C:\rune\1245620\remote`, 0),
		measured("g2", "Balatro", "2379780", `C:\a\remote`, 7),
	}
	kept := WithoutRedundantEmpty(saves)
	if len(kept) != 2 {
		t.Fatalf("Elden Ring has nothing anywhere and must still be listed; got %+v", paths(kept))
	}
	if n := CountRedundantEmpty(saves); n != 0 {
		t.Errorf("nothing is redundant here, CountRedundantEmpty = %d", n)
	}
}

// Two titles must not rescue each other: emptiness is decided per game, by the
// same key Group uses.
func TestEmptinessIsDecidedPerTitle(t *testing.T) {
	saves := []DiscoveredSave{
		measured("g1", "Has Content", "111", `C:\one`, 5),
		measured("g2", "Has Nothing", "222", `C:\two`, 0),
	}
	kept := WithoutRedundantEmpty(saves)
	if len(kept) != 2 {
		t.Errorf("a different game holding files must not hide this one; got %+v", paths(kept))
	}
}

// Unmeasured is not empty. Hiding a folder nobody could read is how a real
// save goes missing.
func TestUnmeasuredLocationsSurvive(t *testing.T) {
	saves := []DiscoveredSave{
		measured("g1", "Game", "111", `C:\real`, 3),
		{ID: "g1", Name: "Game", AppID: "111", SavePath: `C:\unreadable`, Type: "game"},
	}
	kept := WithoutRedundantEmpty(saves)
	if len(kept) != 2 {
		t.Errorf("an unmeasured folder was dropped as if it were empty; got %+v", paths(kept))
	}
}

// Order is what the numbered `add <n>` resolves against, so it has to hold.
func TestOrderIsPreserved(t *testing.T) {
	saves := []DiscoveredSave{
		measured("a", "A", "1", `C:\1`, 1),
		measured("b", "B", "2", `C:\2`, 0),
		measured("c", "C", "3", `C:\3`, 1),
	}
	kept := WithoutRedundantEmpty(saves)
	if len(kept) != 3 || kept[0].SavePath != `C:\1` || kept[2].SavePath != `C:\3` {
		t.Errorf("order changed: %+v", paths(kept))
	}
}

func paths(s []DiscoveredSave) []string {
	out := make([]string, 0, len(s))
	for _, d := range s {
		out = append(out, d.SavePath)
	}
	return out
}
