package snapshot

import (
	"archive/zip"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// damage changes the archive the way a failing disk or an interrupted write
// would: one byte flipped in the middle, or the end cut off.
func damage(t *testing.T, path string, truncate bool) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if truncate {
		raw = raw[:len(raw)/2]
	} else {
		// Inside a file's stored bytes: a flip in a header field the reader
		// does not use would harm nothing, and is rightly not reported.
		r, err := zip.OpenReader(path)
		if err != nil {
			t.Fatal(err)
		}
		f := r.File[0]
		at, err := f.DataOffset()
		r.Close()
		if err != nil {
			t.Fatal(err)
		}
		raw[at+int64(f.CompressedSize64)/2] ^= 0xFF
	}
	if err := os.WriteFile(path, raw, 0o666); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyFindsWhatIsWrong(t *testing.T) {
	for _, c := range []struct {
		name string
		harm func(t *testing.T, path string)
	}{
		{"whole", func(*testing.T, string) {}},
		{"a byte flipped", func(t *testing.T, p string) { damage(t, p, false) }},
		{"cut short", func(t *testing.T, p string) { damage(t, p, true) }},
		{"missing", func(t *testing.T, p string) { os.Remove(p) }},
	} {
		t.Run(c.name, func(t *testing.T) {
			env := setup(t)
			writeSave(t, env.saveDir, "slot1.sav", strings.Repeat("a long and varied save, ", 400))
			writeSave(t, env.saveDir, "slot2.sav", strings.Repeat("another save entirely. ", 400))
			snap, err := env.mgr.Create("game1", "", false)
			if err != nil {
				t.Fatal(err)
			}
			c.harm(t, snap.ZipPath)

			err = env.mgr.Verify(snap)
			recorded, _ := env.store.GetSnapshot(snap.ID)
			if c.name == "whole" {
				if err != nil || recorded.Problem != "" || recorded.CheckedMs == 0 {
					t.Errorf("a whole snapshot: err=%v, recorded problem=%q checked=%d", err, recorded.Problem, recorded.CheckedMs)
				}
				return
			}
			if !errors.Is(err, ErrDamaged) {
				t.Errorf("verify = %v, want it found damaged", err)
			}
			if recorded.Problem == "" {
				t.Errorf("the damage was not recorded")
			}
		})
	}
}

// A damaged snapshot is refused before the save folder is touched: finding out
// part-way through a restore would leave neither save.
func TestRestoreRefusesADamagedSnapshotAndChangesNothing(t *testing.T) {
	env := setup(t)
	writeSave(t, env.saveDir, "slot1.sav", strings.Repeat("the old save, ", 400))
	snap, err := env.mgr.Create("game1", "", false)
	if err != nil {
		t.Fatal(err)
	}
	damage(t, snap.ZipPath, false)
	writeSave(t, env.saveDir, "slot1.sav", "the save as it is now")

	if _, err := env.mgr.Restore("game1", snap.ID); !errors.Is(err, ErrDamaged) {
		t.Fatalf("restore = %v, want refused as damaged", err)
	}
	raw, _ := os.ReadFile(filepath.Join(env.saveDir, "slot1.sav"))
	if string(raw) != "the save as it is now" {
		t.Errorf("the save was changed by a refused restore: %q", raw)
	}
	if recorded, _ := env.store.GetSnapshot(snap.ID); recorded.Problem == "" {
		t.Errorf("the refused snapshot was not marked damaged")
	}
}
