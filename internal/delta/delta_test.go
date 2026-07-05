package delta

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestManifestUnmarshalsJSFractionalMtimes reproduces the exact payload
// shape the original Node daemon sends: fs.statSync().mtimeMs values are
// FRACTIONAL floats. Decoding these must not fail (cross-version sync
// broke on this in the field).
func TestManifestUnmarshalsJSFractionalMtimes(t *testing.T) {
	jsPayload := `{
		"timestamp": "2026-07-05T19:22:45.872Z",
		"latestMtime": 1783279365872.0251,
		"files": {
			"slot1.sav": {
				"size": 26,
				"hash": "abc",
				"blocks": [{"index": 0, "hash": "abc", "length": 26}],
				"blockSize": 65536,
				"mtime": 1783279365872.0251
			}
		},
		"dirs": ["config"]
	}`

	var m Manifest
	if err := json.Unmarshal([]byte(jsPayload), &m); err != nil {
		t.Fatalf("JS manifest must decode: %v", err)
	}
	if int64(m.LatestMtime) != 1783279365872 {
		t.Errorf("LatestMtime = %d, want truncated 1783279365872", int64(m.LatestMtime))
	}
	if int64(m.Files["slot1.sav"].MtimeMs) != 1783279365872 {
		t.Errorf("file mtime = %d, want truncated 1783279365872", int64(m.Files["slot1.sav"].MtimeMs))
	}

	// And our own marshaling must emit whole integers JS can compare.
	out, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(out, []byte(".0251")) {
		t.Error("marshaled manifest must not contain fractional mtimes")
	}
	if !bytes.Contains(out, []byte(`"latestMtime":1783279365872`)) {
		t.Errorf("marshaled manifest missing integer latestMtime: %s", out)
	}
}

func TestBlockSizeFor(t *testing.T) {
	cases := []struct {
		size int64
		want int
	}{
		{1024, defaultBlockSize},
		{mediumFileThreshold, defaultBlockSize},
		{mediumFileThreshold + 1, mediumBlockSize},
		{largeFileThreshold, mediumBlockSize},
		{largeFileThreshold + 1, largeBlockSize},
	}
	for _, c := range cases {
		if got := BlockSizeFor(c.size); got != c.want {
			t.Errorf("BlockSizeFor(%d) = %d, want %d", c.size, got, c.want)
		}
	}
}

func TestHashFile_DeterministicAndBlockAligned(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "save.dat")
	data := bytes.Repeat([]byte("A"), defaultBlockSize+37) // spans 2 blocks
	if err := os.WriteFile(path, data, 0o666); err != nil {
		t.Fatal(err)
	}

	entry, err := HashFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if entry.Size != int64(len(data)) {
		t.Errorf("Size = %d, want %d", entry.Size, len(data))
	}
	if len(entry.Blocks) != 2 {
		t.Fatalf("got %d blocks, want 2", len(entry.Blocks))
	}
	if entry.Blocks[0].Length != defaultBlockSize {
		t.Errorf("block 0 length = %d, want %d", entry.Blocks[0].Length, defaultBlockSize)
	}
	if entry.Blocks[1].Length != 37 {
		t.Errorf("block 1 length = %d, want 37", entry.Blocks[1].Length)
	}

	entry2, err := HashFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if entry.Hash != entry2.Hash {
		t.Error("hash should be deterministic across repeated hashing of the same content")
	}
}

func TestDiffManifests_AddedChangedDeleted(t *testing.T) {
	local := Manifest{Files: map[string]FileEntry{
		"unchanged.dat": {Hash: "same"},
		"only_local.dat": {Hash: "local-only"},
		"changed.dat": {
			Hash:      "old",
			BlockSize: defaultBlockSize,
			Blocks: []Block{
				{Index: 0, Hash: "a", Length: defaultBlockSize},
				{Index: 1, Hash: "b", Length: 10},
			},
		},
	}}
	remote := Manifest{Files: map[string]FileEntry{
		"unchanged.dat": {Hash: "same"},
		"new_remote.dat": {Hash: "new", Blocks: []Block{{Index: 0, Hash: "x", Length: 5}}},
		"changed.dat": {
			Hash:      "new",
			BlockSize: defaultBlockSize,
			Blocks: []Block{
				{Index: 0, Hash: "a", Length: defaultBlockSize}, // unchanged block
				{Index: 1, Hash: "c", Length: 12},                // changed block
			},
		},
	}}

	diff := DiffManifests(local, remote)

	pullByPath := map[string]FileDiff{}
	for _, d := range diff.FilesToPull {
		pullByPath[d.RelPath] = d
	}
	if _, ok := pullByPath["unchanged.dat"]; ok {
		t.Error("unchanged.dat should not be in FilesToPull")
	}
	newRemote, ok := pullByPath["new_remote.dat"]
	if !ok || !newRemote.Added || len(newRemote.ModifiedBlocks) != 1 {
		t.Errorf("new_remote.dat diff wrong: %+v (ok=%v)", newRemote, ok)
	}
	changed, ok := pullByPath["changed.dat"]
	if !ok {
		t.Fatal("changed.dat should be in FilesToPull")
	}
	if len(changed.ModifiedBlocks) != 1 || changed.ModifiedBlocks[0] != 1 {
		t.Errorf("changed.dat ModifiedBlocks = %v, want [1] (only block 1 differs)", changed.ModifiedBlocks)
	}

	pushByPath := map[string]FileDiff{}
	for _, d := range diff.FilesToPush {
		pushByPath[d.RelPath] = d
	}
	if _, ok := pushByPath["only_local.dat"]; !ok {
		t.Error("only_local.dat should be in FilesToPush (exists locally, missing remotely)")
	}
	if _, ok := pushByPath["unchanged.dat"]; ok {
		t.Error("unchanged.dat should not be in FilesToPush")
	}
}

func TestIsSafePath_BlocksTraversal(t *testing.T) {
	base := filepath.Join(t.TempDir(), "saves")
	if err := os.MkdirAll(base, 0o777); err != nil {
		t.Fatal(err)
	}

	if !IsSafePath(base, "sub/file.dat") {
		t.Error("normal nested relative path should be safe")
	}
	if !IsSafePath(base, ".") {
		t.Error("base dir itself should be safe")
	}
	if IsSafePath(base, "../outside.txt") {
		t.Error("parent traversal should be blocked")
	}
	if IsSafePath(base, "../../etc/passwd") {
		t.Error("deep parent traversal should be blocked")
	}
}

func TestPatchFile_ReconstructsAndVerifiesHash(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "save.dat")

	original := bytes.Repeat([]byte("O"), defaultBlockSize+20)
	if err := os.WriteFile(filePath, original, 0o666); err != nil {
		t.Fatal(err)
	}
	localEntry, err := HashFile(filePath)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate the remote having changed only the second (tail) block.
	newTail := bytes.Repeat([]byte("N"), 20)
	remoteEntry := FileEntry{
		BlockSize: localEntry.BlockSize,
		Blocks: []Block{
			localEntry.Blocks[0], // unchanged, must be copied from local file
			{Index: 1, Hash: hashBytes(newTail), Length: len(newTail)},
		},
	}
	wholeExpected := append(append([]byte{}, original[:defaultBlockSize]...), newTail...)
	remoteEntry.Hash = hashBytes(wholeExpected)

	err = PatchFile(filePath, remoteEntry, []BlockSource{{Index: 1, Data: newTail}})
	if err != nil {
		t.Fatalf("PatchFile error = %v", err)
	}

	got, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, wholeExpected) {
		t.Error("patched file content does not match expected reconstruction")
	}
	if _, err := os.Stat(filePath + ".opensave.tmp"); !os.IsNotExist(err) {
		t.Error("temp file should not remain after a successful patch")
	}
}

func TestPatchFile_RejectsCorruptReconstruction(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "save.dat")
	if err := os.WriteFile(filePath, []byte("original"), 0o666); err != nil {
		t.Fatal(err)
	}

	remoteEntry := FileEntry{
		BlockSize: defaultBlockSize,
		Blocks:    []Block{{Index: 0, Hash: hashBytes([]byte("expected-data")), Length: 13}},
		Hash:      hashBytes([]byte("expected-data")),
	}

	err := PatchFile(filePath, remoteEntry, []BlockSource{{Index: 0, Data: []byte("wrong-data!!!")}})
	if err == nil {
		t.Fatal("expected integrity check failure, got nil error")
	}

	got, readErr := os.ReadFile(filePath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "original" {
		t.Error("original file must be left untouched when patch verification fails")
	}
	if _, err := os.Stat(filePath + ".opensave.tmp"); !os.IsNotExist(err) {
		t.Error("temp file should be cleaned up after a failed patch")
	}
}

func TestTranslatePathToLocal_CustomRuleTakesPriority(t *testing.T) {
	got := TranslatePathToLocal(`D:\Remote\Saves\game1`, []TranslationRule{
		{FromPattern: `D:\Remote\Saves`, ToPattern: `/mnt/local/saves`},
	})
	want := "/mnt/local/saves/game1"
	if got != want {
		t.Errorf("TranslatePathToLocal = %q, want %q", got, want)
	}
}
