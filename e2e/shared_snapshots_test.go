package e2e

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/opensave/opensave/testutil"
)

// Older snapshots share the files they have in common (internal/snapshot/
// shared.go): their zips are replaced by a list and a store of shared files.
// Everything that reads a snapshot must still get the whole archive — the
// file browser, a single-file restore, a full restore, the cloud and an
// export — and get it exactly as it was written.
func TestSharedSnapshots_EverythingThatReadsOneGetsItWhole(t *testing.T) {
	td := testutil.NewTestDaemon(t, "Shared")
	slot := func() string {
		b := make([]byte, 40<<10)
		rand.Read(b)
		return string(b)
	}
	for _, name := range []string{"slot1.sav", "slot2.sav", "slot3.sav", "slot4.sav"} {
		td.WriteSave(name, slot())
	}
	td.WriteSave("settings.ini", "volume=7\n")
	gameID := td.TrackGame("Shared Game")

	type snap struct {
		ID      string `json:"id"`
		ZipPath string `json:"zipPath"`
	}
	var snaps []snap
	var originals []map[string][]byte
	var firstSlot4 string
	for i := 0; i < 3; i++ {
		if i > 0 {
			td.WriteSave("slot4.sav", slot())
		} else {
			firstSlot4 = td.ReadSave("slot4.sav")
		}
		var s snap
		td.API(http.MethodPost, "/api/games/"+gameID+"/snapshot", map[string]string{"comment": "take " + string(rune('A'+i))}, &s)
		if s.ZipPath == "" {
			t.Fatal("setup: no snapshot")
		}
		snaps = append(snaps, s)
		originals = append(originals, zipContents(t, readFile(t, s.ZipPath)))
	}

	// The daemon's own pass, without waiting the hour it leaves new ones.
	if _, err := td.Daemon.Snapshots.CompactAll(context.Background(), 0, 0); err != nil {
		t.Fatal(err)
	}
	for _, s := range snaps[:2] {
		if _, err := os.Stat(s.ZipPath); err == nil {
			t.Fatalf("%s was not compacted", s.ID)
		}
	}
	if _, err := os.Stat(snaps[2].ZipPath); err != nil {
		t.Fatal("the newest snapshot was compacted; it stays whole")
	}

	var storage struct {
		TotalBytes int64 `json:"totalBytes"`
		DiskBytes  int64 `json:"diskBytes"`
	}
	td.API(http.MethodGet, "/api/storage", nil, &storage)
	if storage.DiskBytes <= 0 || storage.DiskBytes >= storage.TotalBytes {
		t.Errorf("storage: %d on disk of %d, want less once snapshots share files", storage.DiskBytes, storage.TotalBytes)
	}

	// The file browser.
	var files []struct {
		Path string `json:"path"`
		Size int64  `json:"size"`
	}
	td.API(http.MethodGet, "/api/games/"+gameID+"/snapshot/"+snaps[0].ID+"/files", nil, &files)
	listed := map[string]int64{}
	for _, f := range files {
		listed[f.Path] = f.Size
	}
	if listed["slot4.sav"] != 40<<10 || listed["settings.ini"] != int64(len("volume=7\n")) {
		t.Errorf("the file browser lists %v", listed)
	}

	// One file back.
	td.WriteSave("slot4.sav", "the save as it is now")
	td.API(http.MethodPost, "/api/games/"+gameID+"/snapshot/"+snaps[0].ID+"/restore-file", map[string]string{"relPath": "slot4.sav"}, nil)
	if td.ReadSave("slot4.sav") != firstSlot4 {
		t.Error("restoring one file from a compacted snapshot did not put that file back")
	}

	// The whole snapshot back.
	td.WriteSave("slot2.sav", "changed since")
	if code := td.APIStatus(http.MethodPost, "/api/games/"+gameID+"/rollback", map[string]string{"snapshotId": snaps[1].ID}, nil); code != http.StatusOK {
		t.Fatalf("restoring a compacted snapshot: %d", code)
	}
	for name, body := range originals[1] {
		if strings.HasSuffix(name, "/") {
			continue
		}
		if got := td.ReadSave(name); got != string(body) {
			t.Errorf("after restoring: %s is not what the snapshot held", name)
		}
	}

	// The cloud, turned on only now, so the compacted snapshots go up through
	// the catch-up upload rather than as they were taken.
	dir := useLocalCloud(t, td)
	td.API(http.MethodPost, "/api/cloud/sync-local/"+gameID, map[string]any{}, nil)
	for i, s := range snaps[:2] {
		name := gameID + "__main__" + s.ID + ".zip"
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("%s is not in the cloud: %v (there: %s)", name, err, strings.Join(cloudFiles(t, dir), ", "))
		}
		sameContents(t, "the cloud copy of "+s.ID, zipContents(t, raw), originals[i])
	}

	// An export.
	target := filepath.Join(testutil.TempDir(t), "all.sscb")
	td.API(http.MethodPost, "/api/backup/export", map[string]string{"targetPath": target}, nil)
	// An .sscb is a zip, brotli-compressed.
	packed, err := io.ReadAll(brotli.NewReader(bytes.NewReader(readFile(t, target))))
	if err != nil {
		t.Fatal(err)
	}
	export, err := zip.NewReader(bytes.NewReader(packed), int64(len(packed)))
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, f := range export.File {
		for i, s := range snaps[:2] {
			if strings.HasSuffix(f.Name, "__"+s.ID+".zip") {
				rc, _ := f.Open()
				raw, _ := io.ReadAll(rc)
				rc.Close()
				sameContents(t, "the exported copy of "+s.ID, zipContents(t, raw), originals[i])
				found++
			}
		}
	}
	if found != 2 {
		t.Errorf("the export holds %d of the 2 compacted snapshots", found)
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// zipContents is every file in an archive by name.
func zipContents(t *testing.T, raw []byte) map[string][]byte {
	t.Helper()
	r, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]byte{}
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("%s: %v", f.Name, err)
		}
		out[f.Name] = body
	}
	return out
}

func sameContents(t *testing.T, what string, got, want map[string][]byte) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s: %d entries, want %d", what, len(got), len(want))
	}
	for name, body := range want {
		if !bytes.Equal(got[name], body) {
			t.Errorf("%s: %s differs", what, name)
		}
	}
}

// An upload held back by a pause keeps only where the zip was, and a pause
// can last until the app restarts; by the time it ends the snapshot may have
// been compacted. It still goes up, whole.
func TestSharedSnapshots_AnUploadHeldByAPauseGoesUpWhole(t *testing.T) {
	td := testutil.NewTestDaemon(t, "SharedPause")
	dir := useLocalCloud(t, td)
	slot := func() string {
		b := make([]byte, 40<<10)
		rand.Read(b)
		return string(b)
	}
	td.WriteSave("slot1.sav", slot())
	td.WriteSave("slot2.sav", slot())
	gameID := td.TrackGame("Shared Pause Game")
	if !testutil.WaitFor(30*time.Second, func() bool { return len(cloudFiles(t, dir)) > 0 }) {
		t.Fatal("setup: nothing ever reached the cloud folder")
	}

	td.API(http.MethodPost, "/api/sync/pause", map[string]any{"untilRestart": true}, nil)
	var held struct {
		ID      string `json:"id"`
		ZipPath string `json:"zipPath"`
	}
	td.WriteSave("slot2.sav", slot())
	td.API(http.MethodPost, "/api/games/"+gameID+"/snapshot", map[string]string{"comment": "held"}, &held)
	want := zipContents(t, readFile(t, held.ZipPath))
	td.WriteSave("slot2.sav", slot())
	td.API(http.MethodPost, "/api/games/"+gameID+"/snapshot", map[string]string{"comment": "newer"}, nil)

	if _, err := td.Daemon.Snapshots.CompactAll(context.Background(), 0, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(held.ZipPath); err == nil {
		t.Fatal("setup: the held snapshot was not compacted")
	}

	td.API(http.MethodPost, "/api/sync/resume", map[string]any{}, nil)
	name := gameID + "__main__" + held.ID + ".zip"
	if !testutil.WaitFor(30*time.Second, func() bool {
		_, err := os.Stat(filepath.Join(dir, name))
		return err == nil
	}) {
		t.Fatalf("the held snapshot never reached the cloud: %v", cloudFiles(t, dir))
	}
	// Written by the upload; wait for it to be whole.
	var got map[string][]byte
	testutil.WaitFor(10*time.Second, func() bool {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return false
		}
		r, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
		if err != nil || len(r.File) != len(want) {
			return false
		}
		got = zipContents(t, raw)
		return true
	})
	sameContents(t, "the held snapshot's cloud copy", got, want)
}
