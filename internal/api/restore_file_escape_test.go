package api

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

// A snapshot archive is not always this machine's own — it can arrive from a
// peer or inside an imported backup, so its entry names are attacker-controlled
// in the cases that matter.
//
// extractSingleFile joined the requested name straight onto the save path, so
// an entry called "../x" wrote outside the save folder. Its sibling
// extractEntryAs has always refused those; this path had no check at all.
// Matching an entry in the archive is not protection when the archive is the
// thing under someone else's control.
func TestRestoringOneFileCannotEscapeTheSaveFolder(t *testing.T) {
	root := t.TempDir()
	savePath := filepath.Join(root, "save")
	if err := os.MkdirAll(savePath, 0o777); err != nil {
		t.Fatal(err)
	}

	for _, entry := range []string{
		"../outside.txt",
		"../../outside.txt",
		`..\outside.txt`,
		`..\..\outside.txt`,
	} {
		zipPath := filepath.Join(t.TempDir(), "snap.zip")
		f, err := os.Create(zipPath)
		if err != nil {
			t.Fatal(err)
		}
		zw := zip.NewWriter(f)
		w, err := zw.Create(entry)
		if err != nil {
			zw.Close()
			f.Close()
			continue
		}
		if _, err := w.Write([]byte("PAYLOAD")); err != nil {
			t.Fatal(err)
		}
		zw.Close()
		f.Close()

		if err := extractSingleFile(zipPath, entry, entry, savePath); err == nil {
			t.Errorf("entry %q was extracted; it points outside the save folder", entry)
		}
		// Nothing may exist above the save folder afterwards.
		for _, probe := range []string{
			filepath.Join(root, "outside.txt"),
			filepath.Join(filepath.Dir(root), "outside.txt"),
		} {
			if _, err := os.Stat(probe); err == nil {
				t.Errorf("entry %q wrote to %s", entry, probe)
				os.Remove(probe)
			}
		}
	}
}

// An ordinary file still restores, including one in a subfolder of the save.
func TestRestoringOneOrdinaryFileStillWorks(t *testing.T) {
	root := t.TempDir()
	savePath := filepath.Join(root, "save")
	if err := os.MkdirAll(savePath, 0o777); err != nil {
		t.Fatal(err)
	}

	for _, entry := range []string{"slot1.dat", "profiles/player/slot2.dat"} {
		zipPath := filepath.Join(t.TempDir(), "snap.zip")
		f, err := os.Create(zipPath)
		if err != nil {
			t.Fatal(err)
		}
		zw := zip.NewWriter(f)
		w, _ := zw.Create(entry)
		_, _ = w.Write([]byte("real save"))
		zw.Close()
		f.Close()

		if err := extractSingleFile(zipPath, entry, entry, savePath); err != nil {
			t.Fatalf("restoring %q failed: %v", entry, err)
		}
		got, err := os.ReadFile(filepath.Join(savePath, filepath.FromSlash(entry)))
		if err != nil {
			t.Fatalf("%q was not restored into the save folder: %v", entry, err)
		}
		if string(got) != "real save" {
			t.Errorf("%q = %q", entry, got)
		}
	}
}
