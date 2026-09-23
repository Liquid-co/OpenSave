package snapshot

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A save that compresses is compressed; one that does not is stored as it
// always was; and either way it comes back byte for byte.

// jsonSave is the shape of a lot of real saves: text, repetitive keys.
func jsonSave(entries int) []byte {
	var b strings.Builder
	b.WriteString(`{"slots":[`)
	for i := 0; i < entries; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"slot":%d,"level":%d,"hp":%d,"inventory":["sword","shield","potion","potion"],"flags":{"met_npc":true,"boss_%d":false}}`, i, i%40, 100-i%100, i)
	}
	b.WriteString(`]}`)
	return []byte(b.String())
}

func randomBytes(t *testing.T, n int) []byte {
	t.Helper()
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return b
}

func methodsIn(t *testing.T, zipPath string) map[string]uint16 {
	t.Helper()
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	out := map[string]uint16{}
	for _, f := range r.File {
		out[f.Name] = f.Method
	}
	return out
}

func TestSnapshotCompressesWhatCompressesAndStoresTheRest(t *testing.T) {
	src := t.TempDir()
	files := map[string][]byte{
		"profile.json":   jsonSave(3000),          // ~300 KB of text: well past the probe
		"compressed.sav": randomBytes(t, 200_000), // already compressed, as many saves are
		"tiny.cfg":       []byte("vsync=1\n"),     // too small to be worth it
		"empty.dat":      {},
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(src, name), body, 0o666); err != nil {
			t.Fatal(err)
		}
	}

	archive := filepath.Join(t.TempDir(), "snap.zip")
	_, captured, err := ZipPathCapturing(src, archive)
	if err != nil {
		t.Fatal(err)
	}

	methods := methodsIn(t, archive)
	want := map[string]uint16{
		"profile.json":   zip.Deflate,
		"compressed.sav": zip.Store,
		"tiny.cfg":       zip.Store,
		"empty.dat":      zip.Store,
	}
	for name, m := range want {
		if methods[name] != m {
			t.Errorf("%s stored with method %d, want %d", name, methods[name], m)
		}
	}

	// The hashes recorded are of the file, not of what went into the archive.
	for _, c := range captured {
		sum := sha256.Sum256(files[c.Path])
		if c.Hash != hex.EncodeToString(sum[:]) {
			t.Errorf("%s: recorded hash %s is not the file's", c.Path, c.Hash)
		}
	}

	// And everything comes back exactly.
	dst := t.TempDir()
	if err := UnzipTo(archive, dst); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		got, err := os.ReadFile(filepath.Join(dst, name))
		if err != nil {
			t.Fatalf("%s did not come back: %v", name, err)
		}
		if !bytes.Equal(got, body) {
			t.Errorf("%s came back different: %d bytes, want %d", name, len(got), len(body))
		}
	}

	// The point of it: the text save costs a fraction of what it did.
	info, _ := os.Stat(archive)
	raw := 0
	for _, b := range files {
		raw += len(b)
	}
	jsonSize := len(files["profile.json"])
	if saved := raw - int(info.Size()); saved < jsonSize/2 {
		t.Errorf("archive is %d bytes for %d bytes of files; the text save should have shrunk by at least half", info.Size(), raw)
	}
}

// The probe reads the start of the file and the rest follows it. A file just
// over the probe's size must come back whole, with nothing doubled or lost at
// the seam.
func TestSnapshotFileAcrossTheProbeBoundary(t *testing.T) {
	for _, size := range []int{compressProbeBytes - 1, compressProbeBytes, compressProbeBytes + 1, 3*compressProbeBytes + 17} {
		src := t.TempDir()
		body := bytes.Repeat([]byte("abcdefgh"), size/8+1)[:size]
		if err := os.WriteFile(filepath.Join(src, "save.dat"), body, 0o666); err != nil {
			t.Fatal(err)
		}
		archive := filepath.Join(t.TempDir(), "snap.zip")
		if _, _, err := ZipPathCapturing(src, archive); err != nil {
			t.Fatal(err)
		}
		dst := t.TempDir()
		if err := UnzipTo(archive, dst); err != nil {
			t.Fatal(err)
		}
		got, _ := os.ReadFile(filepath.Join(dst, "save.dat"))
		if !bytes.Equal(got, body) {
			t.Errorf("size %d: came back as %d bytes, or with different content", size, len(got))
		}
	}
}
