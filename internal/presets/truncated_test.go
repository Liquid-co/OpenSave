package presets

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// The bug behind "some games wouldn't pop up until I'd auto-scanned ten times,
// and some titles would just disappear".
//
// A walk that runs out of budget before reaching any file reports Measured with
// a count of zero, which without Truncated is indistinguishable from a folder
// that genuinely holds nothing — so a save full of files was hidden as empty.
// Whether a given location runs out depends on how many others were measured
// first, the disk that minute, and goroutine scheduling, so the same machine
// answers differently every scan.
func TestATruncatedMeasurementIsNotEmpty(t *testing.T) {
	root := t.TempDir()
	// Hundreds of folders before the first file: a save with a directory per
	// slot or per profile, which is an ordinary shape.
	for i := 0; i < 400; i++ {
		if err := os.MkdirAll(filepath.Join(root, "slot"+strconv.Itoa(i)), 0o777); err != nil {
			t.Fatal(err)
		}
	}
	saves := filepath.Join(root, "zzz_saves")
	if err := os.MkdirAll(saves, 0o777); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		if err := os.WriteFile(filepath.Join(saves, "save"+strconv.Itoa(i)+".dat"),
			[]byte("real save"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	d := DiscoveredSave{ID: "g", Name: "Real Game", SavePath: root, Type: "game"}
	// A budget already spent, as it is for the last locations of a big scan.
	measureInto(&d, time.Now().Add(-time.Hour))

	if !d.Truncated {
		t.Fatalf("expected the walk to be cut short; measured=%v count=%d",
			d.Measured, d.FileCount)
	}
	if d.IsEmpty() {
		t.Error("a folder holding 20 save files reports empty because its measurement " +
			"ran out of budget — the game disappears from the scan, and comes back on " +
			"a later one purely by luck of timing")
	}
}

// A location that was cut short must survive the filter too, or the fix above
// only moves the problem one layer up.
func TestATruncatedLocationIsNotFilteredOut(t *testing.T) {
	saves := []DiscoveredSave{
		{ID: "g1", Name: "Game", AppID: "1", SavePath: `C:\a`, Measured: true, FileCount: 5},
		{ID: "g1", Name: "Game", AppID: "1", SavePath: `C:\b`, Measured: true, FileCount: 0, Truncated: true},
	}
	kept := WithoutRedundantEmpty(saves)
	if len(kept) != 2 {
		t.Errorf("a location whose measurement was cut short was dropped as empty: %d kept", len(kept))
	}
}

// The file cap also sets Truncated, and a location that counted 20,000 files
// must still be able to be non-empty — the two uses of the flag must not be
// confused.
func TestTheFileCapDoesNotMakeALocationUnknown(t *testing.T) {
	d := DiscoveredSave{Measured: true, FileCount: statFileCap, Truncated: true}
	if d.IsEmpty() {
		t.Error("a location holding statFileCap files reported empty")
	}
	// And a genuinely empty, fully walked folder is still empty.
	e := DiscoveredSave{Measured: true, FileCount: 0}
	if !e.IsEmpty() {
		t.Error("a folder that was fully walked and holds nothing is no longer empty")
	}
	// Unmeasured stays unknown.
	u := DiscoveredSave{}
	if u.IsEmpty() {
		t.Error("an unmeasured location reported empty")
	}
}

// Two scans of the same unchanged machine must agree about which games exist.
// This is the property the report was really about: not that one scan was
// wrong, but that they disagreed.
func TestRepeatedMeasurementsAgree(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 400; i++ {
		if err := os.MkdirAll(filepath.Join(root, "slot"+strconv.Itoa(i)), 0o777); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "zzz.dat"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	// One with budget to spare, one with none — the two ends of what a real
	// scan does depending on what it measured first.
	generous := DiscoveredSave{ID: "g", SavePath: root, Type: "game"}
	measureInto(&generous, time.Now().Add(time.Minute))
	starved := DiscoveredSave{ID: "g", SavePath: root, Type: "game"}
	measureInto(&starved, time.Now().Add(-time.Hour))

	if generous.IsEmpty() != starved.IsEmpty() {
		t.Errorf("the same folder is empty=%v with budget and empty=%v without — "+
			"a game appears or disappears depending on scan timing",
			generous.IsEmpty(), starved.IsEmpty())
	}
}
