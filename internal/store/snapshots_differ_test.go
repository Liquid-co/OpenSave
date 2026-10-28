package store

import "testing"

// Two snapshots compared by the database: identical, one file changed, one
// changed only in a path left out, and one file held by one side alone.
func TestSnapshotsDiffer(t *testing.T) {
	s := openTestStore(t)
	if err := s.CreateGame(Game{ID: "g", Name: "g", SavePath: t.TempDir(), ActiveBranch: "main"}); err != nil {
		t.Fatal(err)
	}
	record := func(id string, files map[string]string) {
		t.Helper()
		if err := s.CreateSnapshot(Snapshot{ID: id, GameID: "g", BranchName: "main", Timestamp: "2026-10-26T00:00:00.000Z", ZipPath: id + ".zip"}); err != nil {
			t.Fatal(err)
		}
		var cf []CapturedFile
		for p, h := range files {
			cf = append(cf, CapturedFile{Path: p, Hash: h})
		}
		if err := s.RecordSnapshotFiles(id, cf); err != nil {
			t.Fatal(err)
		}
	}
	record("a", map[string]string{"slot1.sav": "1", "video.cfg": "x"})
	record("same", map[string]string{"slot1.sav": "1", "video.cfg": "x"})
	record("saved", map[string]string{"slot1.sav": "2", "video.cfg": "x"})
	record("cfgOnly", map[string]string{"slot1.sav": "1", "video.cfg": "y"})
	record("more", map[string]string{"slot1.sav": "1", "video.cfg": "x", "slot2.sav": "9"})

	for _, c := range []struct {
		b    string
		want bool
	}{{"same", false}, {"saved", true}, {"cfgOnly", false}, {"more", true}} {
		got, err := s.SnapshotsDiffer("a", c.b, "", []string{"video.cfg"})
		if err != nil || got != c.want {
			t.Errorf("a vs %s, video.cfg left out: %v %v, want %v", c.b, got, err, c.want)
		}
	}
	if got, _ := s.SnapshotsDiffer("a", "cfgOnly", "", nil); !got {
		t.Error("with nothing left out, a changed file was not a difference")
	}
	if h, _ := s.SnapshotPathHashes("cfgOnly", "", []string{"video.cfg", "missing.cfg"}); h["video.cfg"] != "y" || len(h) != 1 {
		t.Errorf("path hashes = %v", h)
	}
	if !s.SnapshotHasFiles("a") || s.SnapshotHasFiles("no-such-snapshot") {
		t.Error("SnapshotHasFiles")
	}
}
