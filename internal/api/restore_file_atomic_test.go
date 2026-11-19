package api

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/opensave/opensave/internal/delta"
)

// A file restored out of a snapshot that fails part-way — here, an entry whose
// checksum does not match — leaves the file that was there untouched, and no
// temporary file behind.
func TestExtractSingleFile_FailingPartWayLeavesTheFile(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "snap.zip")
	out, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(out)
	body := []byte("the snapshot's copy of the save")
	f, err := w.CreateRaw(&zip.FileHeader{
		Name: "slot1.sav", Method: zip.Store,
		CRC32: 0xdeadbeef, CompressedSize64: uint64(len(body)), UncompressedSize64: uint64(len(body)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out.Close()

	save := t.TempDir()
	live := filepath.Join(save, "slot1.sav")
	if err := os.WriteFile(live, []byte("current progress"), 0o666); err != nil {
		t.Fatal(err)
	}

	if err := extractSingleFile(archive, "slot1.sav", "slot1.sav", save); err == nil {
		t.Fatal("a damaged entry was restored without error")
	}
	if got, _ := os.ReadFile(live); string(got) != "current progress" {
		t.Errorf("the save after a failed file restore = %q, want it untouched", got)
	}
	if _, err := os.Stat(live + delta.TmpSuffix); !os.IsNotExist(err) {
		t.Error("a temporary file was left beside the save")
	}
}
