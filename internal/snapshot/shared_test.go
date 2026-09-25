package snapshot

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/opensave/opensave/internal/fsx"
	"github.com/opensave/opensave/internal/store"
)

// noise is content that does not compress: large enough, it goes to the
// shared store however the archive was compressed.
func noise(t *testing.T, n int) string {
	t.Helper()
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// archiveOf reads an archive whole: every entry's header fields that matter
// and its contents, in order.
type archived1 struct {
	name     string
	crc      uint32
	size     uint64
	modified time.Time
	body     []byte
}

func readWhole(t *testing.T, path string) []archived1 {
	t.Helper()
	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	var out []archived1
	for _, f := range r.File {
		a := archived1{name: f.Name, crc: f.CRC32, size: f.UncompressedSize64, modified: f.Modified}
		if !f.FileInfo().IsDir() {
			rc, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			a.body, err = io.ReadAll(rc)
			rc.Close()
			if err != nil {
				t.Fatalf("%s: %v", f.Name, err)
			}
		}
		out = append(out, a)
	}
	return out
}

func sameEntries(t *testing.T, what string, got, want []archived1) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: %d entries, want %d", what, len(got), len(want))
	}
	for i := range want {
		g, w := got[i], want[i]
		if g.name != w.name || g.crc != w.crc || g.size != w.size || !g.modified.Equal(w.modified) || !bytes.Equal(g.body, w.body) {
			t.Errorf("%s: entry %d is %q (crc %x, %d bytes, %v), want %q (crc %x, %d bytes, %v)",
				what, i, g.name, g.crc, g.size, g.modified, w.name, w.crc, w.size, w.modified)
		}
	}
}

func sharedFiles(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	filepath.WalkDir(filepath.Join(root, sharedDirName), func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, blobSuffix) {
			out = append(out, p)
		}
		return nil
	})
	return out
}

func compactedAt(zipPath string) bool {
	_, zipErr := os.Stat(zipPath)
	_, manErr := os.Stat(manifestPath(zipPath))
	return zipErr != nil && manErr == nil
}

// threeSnapshots takes three snapshots of a save with four large slots and
// some small files, changing one slot between each, and keeps a copy of
// each archive as it was written.
func threeSnapshots(t *testing.T, env *testEnv) ([]store.Snapshot, [][]archived1) {
	t.Helper()
	for _, slot := range []string{"slot1.sav", "slot2.sav", "slot3.sav", "slot4.sav"} {
		writeSave(t, env.saveDir, slot, noise(t, 40<<10))
	}
	writeSave(t, env.saveDir, "settings.ini", "volume=7\n")
	writeSave(t, env.saveDir, "profiles/one.cfg", "name=one\n")
	writeSave(t, env.saveDir, "copy-of-slot1.sav", "") // filled below
	var snaps []store.Snapshot
	var originals [][]archived1
	for i := 0; i < 3; i++ {
		if i > 0 {
			writeSave(t, env.saveDir, "slot4.sav", noise(t, 40<<10))
		}
		// The same content twice in one snapshot is kept once.
		body, _ := os.ReadFile(filepath.Join(env.saveDir, "slot1.sav"))
		writeSave(t, env.saveDir, "copy-of-slot1.sav", string(body))
		snap, err := env.mgr.Create("game1", "", false)
		if err != nil {
			t.Fatal(err)
		}
		snaps = append(snaps, snap)
		originals = append(originals, readWhole(t, snap.ZipPath))
	}
	return snaps, originals
}

func TestCompactionSharesUnchangedFilesAndGivesTheSameArchivesBack(t *testing.T) {
	env := setup(t)
	snaps, originals := threeSnapshots(t, env)
	var originalBytes [][]byte
	for _, s := range snaps {
		raw, _ := os.ReadFile(s.ZipPath)
		originalBytes = append(originalBytes, raw)
	}
	before := DiskBytes(snaps)

	res, err := env.mgr.CompactAll(context.Background(), 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Compacted != 2 || res.Failed != 0 {
		t.Fatalf("compaction = %+v, want the two older snapshots compacted", res)
	}
	if !compactedAt(snaps[0].ZipPath) || !compactedAt(snaps[1].ZipPath) {
		t.Fatal("the older snapshots were not compacted")
	}
	if compactedAt(snaps[2].ZipPath) {
		t.Fatal("the newest snapshot on the branch was compacted; it stays whole")
	}
	// slot1 (twice), slot2 and slot3 are the same in both; slot4 differs.
	root := storeRoot(snaps[0].ZipPath)
	if n := len(sharedFiles(t, root)); n != 5 {
		t.Errorf("%d shared files, want 5: slots 1-3 once each and both versions of slot 4", n)
	}
	after := DiskBytes(snaps)
	if after >= before || res.Freed <= 0 {
		t.Errorf("disk use %d -> %d, freed %d: sharing saved nothing", before, after, res.Freed)
	}

	for i, snap := range snaps {
		archive, done, err := OpenArchive(snap.ZipPath)
		if err != nil {
			t.Fatal(err)
		}
		sameEntries(t, snap.ID, readWhole(t, archive), originals[i])
		// Byte for byte, and so the size its record gives: an upload that
		// compares sizes must not take it for another file.
		if got, _ := os.ReadFile(archive); !bytes.Equal(got, originalBytes[i]) || int64(len(got)) != snap.SizeBytes {
			t.Errorf("%s rebuilt is %d bytes and not the archive as written (%d bytes, recorded %d)", snap.ID, len(got), len(originalBytes[i]), snap.SizeBytes)
		}
		if err := VerifyArchive(snap.ZipPath); err != nil {
			t.Errorf("verify %s: %v", snap.ID, err)
		}
		done()
		if n, err := ArchiveFileCount(snap.ZipPath); err != nil || n != 7 {
			t.Errorf("file count of %s = %d, %v; want 7", snap.ID, n, err)
		}
	}
	// The rebuilt archive was a temporary copy, and it is gone.
	archive, done, _ := OpenArchive(snaps[0].ZipPath)
	done()
	if _, err := os.Stat(archive); err == nil {
		t.Error("the rebuilt archive was left behind")
	}

	// Compare and the restore preview read the lists.
	cmp, err := env.mgr.Compare("game1", snaps[0].ID, snaps[1].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmp.Changes) != 1 || cmp.Changes[0].Path != "slot4.sav" || cmp.Unchanged != 6 {
		t.Errorf("compare = %+v, want slot4.sav changed and six unchanged", cmp)
	}
	if _, err := env.mgr.PreviewRestore("game1", snaps[0].ID); err != nil {
		t.Errorf("preview a compacted snapshot: %v", err)
	}

	// A second pass has nothing left to do.
	again, err := env.mgr.CompactAll(context.Background(), 0, 0)
	if err != nil || again.Compacted != 0 || again.Failed != 0 {
		t.Errorf("second pass = %+v, %v; want nothing done", again, err)
	}
}

func TestRestoreFromACompactedSnapshot(t *testing.T) {
	env := setup(t)
	snaps, _ := threeSnapshots(t, env)
	want, _ := os.ReadFile(filepath.Join(env.saveDir, "slot4.sav"))
	if _, err := env.mgr.CompactAll(context.Background(), 0, 0); err != nil {
		t.Fatal(err)
	}
	if !compactedAt(snaps[0].ZipPath) {
		t.Fatal("not compacted")
	}
	first := originalsBody(t, env, snaps[0])

	if _, err := env.mgr.Restore("game1", snaps[0].ID); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(env.saveDir, "slot4.sav"))
	if !bytes.Equal(got, first) || bytes.Equal(got, want) {
		t.Error("slot4.sav is not the first snapshot's")
	}
	if cfg, _ := os.ReadFile(filepath.Join(env.saveDir, "profiles", "one.cfg")); string(cfg) != "name=one\n" {
		t.Errorf("a small file came back as %q", cfg)
	}
}

// originalsBody is slot4.sav as a snapshot holds it, read through
// OpenArchive.
func originalsBody(t *testing.T, env *testEnv, snap store.Snapshot) []byte {
	t.Helper()
	archive, done, err := OpenArchive(snap.ZipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	for _, e := range readWhole(t, archive) {
		if e.name == "slot4.sav" {
			return e.body
		}
	}
	t.Fatal("no slot4.sav")
	return nil
}

// One damaged shared file damages every snapshot that holds it — which is
// the cost of sharing, and why the check must find it, and a restore must
// refuse before touching the save.
func TestADamagedSharedFileIsFoundAndRefused(t *testing.T) {
	env := setup(t)
	snaps, _ := threeSnapshots(t, env)
	if _, err := env.mgr.CompactAll(context.Background(), 0, 0); err != nil {
		t.Fatal(err)
	}
	man, err := readManifest(manifestPath(snaps[0].ZipPath))
	if err != nil {
		t.Fatal(err)
	}
	var slot2 string
	for _, e := range man.Entries {
		if e.Name == "slot2.sav" {
			slot2 = e.Blob
		}
	}
	damage(t, blobPath(sharedStore(snaps[0].ZipPath), slot2), false)
	forgetSharedChecks()

	for _, snap := range snaps[:2] {
		if err := VerifyArchive(snap.ZipPath); !errors.Is(err, ErrDamaged) || !strings.Contains(err.Error(), "slot2.sav") {
			t.Errorf("verify %s = %v, want slot2.sav found damaged", snap.ID, err)
		}
	}
	if err := VerifyArchive(snaps[2].ZipPath); err != nil {
		t.Errorf("the newest snapshot has its own copy and is whole: %v", err)
	}

	writeSave(t, env.saveDir, "slot2.sav", "the save as it is now")
	if _, err := env.mgr.Restore("game1", snaps[0].ID); err == nil || !strings.Contains(err.Error(), "nothing was changed") {
		t.Fatalf("restore = %v, want it refused", err)
	}
	if got, _ := os.ReadFile(filepath.Join(env.saveDir, "slot2.sav")); string(got) != "the save as it is now" {
		t.Error("the save was touched")
	}

	// A missing one is found the same way.
	os.Remove(blobPath(sharedStore(snaps[0].ZipPath), slot2))
	if _, _, err := OpenArchive(snaps[1].ZipPath); !errors.Is(err, ErrDamaged) {
		t.Errorf("open with a shared file missing = %v, want damaged", err)
	}
}

// The zip is only removed once the archive rebuilt in its place is known to
// hold the same files. A shared file already there that looks right by its
// header but is damaged inside must stop the compaction, not be trusted.
func TestCompactionKeepsTheZipWhenTheRebuildDoesNotMatch(t *testing.T) {
	env := setup(t)
	snaps, originals := threeSnapshots(t, env)
	// Compact the first so its shared files exist, then damage one inside,
	// and take the first snapshot's zip back as it was to compact again.
	if _, err := env.mgr.CompactAll(context.Background(), 0, 0); err != nil {
		t.Fatal(err)
	}
	man, _ := readManifest(manifestPath(snaps[1].ZipPath))
	var slot3 string
	for _, e := range man.Entries {
		if e.Name == "slot3.sav" {
			slot3 = e.Blob
		}
	}
	damage(t, blobPath(sharedStore(snaps[1].ZipPath), slot3), false)

	// A fourth snapshot makes the third one compactable; it holds slot3.
	writeSave(t, env.saveDir, "slot4.sav", noise(t, 40<<10))
	if _, err := env.mgr.Create("game1", "", false); err != nil {
		t.Fatal(err)
	}
	res, err := env.mgr.CompactAll(context.Background(), 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed != 1 {
		t.Errorf("compaction = %+v, want the third snapshot to fail", res)
	}
	if compactedAt(snaps[2].ZipPath) {
		t.Fatal("the zip was removed though its rebuild did not match")
	}
	sameEntries(t, "the third snapshot", readWhole(t, snaps[2].ZipPath), originals[2])
	if _, err := os.Stat(manifestPath(snaps[2].ZipPath)); err == nil {
		t.Error("the failed compaction left its list behind")
	}
}

func TestWhatIsNotCompacted(t *testing.T) {
	env := setup(t)
	snaps, _ := threeSnapshots(t, env)
	if err := env.store.SetSnapshotPinned(snaps[0].ID, true); err != nil {
		t.Fatal(err)
	}
	// Not old enough.
	if res, _ := env.mgr.CompactAll(context.Background(), CompactMinAge, 0); res.Compacted != 0 {
		t.Errorf("young snapshots compacted: %+v", res)
	}
	res, err := env.mgr.CompactAll(context.Background(), 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Compacted != 1 || compactedAt(snaps[0].ZipPath) || !compactedAt(snaps[1].ZipPath) {
		t.Errorf("compaction = %+v; the pinned snapshot must stay whole and the other be compacted", res)
	}

	// Nothing large enough to share: left as a zip.
	small := setup(t)
	writeSave(t, small.saveDir, "a.sav", "small")
	s1, _ := small.mgr.Create("game1", "", false)
	writeSave(t, small.saveDir, "a.sav", "smaller")
	small.mgr.Create("game1", "", false)
	if res, _ := small.mgr.CompactAll(context.Background(), 0, 0); res.Compacted != 0 || compactedAt(s1.ZipPath) {
		t.Errorf("a snapshot of small files was compacted: %+v", res)
	}
}

// Something reading a zip through OpenArchive keeps it: the compaction
// leaves both and the next pass removes the zip.
func TestAZipBeingReadIsNotRemoved(t *testing.T) {
	env := setup(t)
	snaps, originals := threeSnapshots(t, env)
	archive, done, err := OpenArchive(snaps[0].ZipPath)
	if err != nil || archive != snaps[0].ZipPath {
		t.Fatalf("open = %q, %v", archive, err)
	}
	env.mgr.CompactAll(context.Background(), 0, 0)
	if _, err := os.Stat(snaps[0].ZipPath); err != nil {
		t.Fatal("the zip was removed while it was being read")
	}
	sameEntries(t, "while reading", readWhole(t, archive), originals[0])
	done()
	done() // twice is harmless
	env.mgr.CompactAll(context.Background(), 0, 0)
	if !compactedAt(snaps[0].ZipPath) {
		t.Error("the next pass did not finish the compaction")
	}
}

func TestDeletingCompactedSnapshotsFreesWhatNothingElseUses(t *testing.T) {
	env := setup(t)
	snaps, originals := threeSnapshots(t, env)
	if _, err := env.mgr.CompactAll(context.Background(), 0, 0); err != nil {
		t.Fatal(err)
	}
	root := storeRoot(snaps[0].ZipPath)

	freed, err := env.mgr.DeleteSnapshot("game1", snaps[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	// Only the first version of slot4 was the first snapshot's alone.
	if n := len(sharedFiles(t, root)); n != 4 {
		t.Errorf("%d shared files after deleting one snapshot, want 4", n)
	}
	if freed < 40<<10 {
		t.Errorf("freed %d, want at least the slot it alone held", freed)
	}
	for _, p := range []string{snaps[0].ZipPath, manifestPath(snaps[0].ZipPath), restPath(snaps[0].ZipPath)} {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("%s is still there", p)
		}
	}
	archive, done, err := OpenArchive(snaps[1].ZipPath)
	if err != nil {
		t.Fatal(err)
	}
	sameEntries(t, "the one left", readWhole(t, archive), originals[1])
	done()

	env.mgr.DeleteSnapshot("game1", snaps[1].ID)
	if n := len(sharedFiles(t, root)); n != 0 {
		t.Errorf("%d shared files left with no compacted snapshot, want none", n)
	}
}

// A deletion while a compaction runs cannot clean up (the compaction may be
// about to name a shared file no list names yet); the next pass does.
func TestCleanUpPutOffDuringACompactionHappensNextPass(t *testing.T) {
	env := setup(t)
	snaps, _ := threeSnapshots(t, env)
	env.mgr.CompactAll(context.Background(), 0, 0)
	root := storeRoot(snaps[0].ZipPath)

	env.mgr.sharedMu.Lock()
	if _, err := env.mgr.DeleteSnapshot("game1", snaps[0].ID); err != nil {
		t.Fatal(err)
	}
	if n := len(sharedFiles(t, root)); n != 5 {
		t.Errorf("%d shared files; nothing should be collected while a compaction holds the store", n)
	}
	env.mgr.sharedMu.Unlock()

	env.mgr.CompactAll(context.Background(), 0, 0)
	if n := len(sharedFiles(t, root)); n != 4 {
		t.Errorf("%d shared files after the next pass, want 4", n)
	}
}

// Cut short between writing the list and removing the zip, a compaction
// leaves both: the zip is what is read, and the next pass finishes.
func TestAnInterruptedCompactionIsHarmless(t *testing.T) {
	env := setup(t)
	snaps, originals := threeSnapshots(t, env)
	keep, _ := os.ReadFile(snaps[0].ZipPath)
	env.mgr.CompactAll(context.Background(), 0, 0)
	if err := os.WriteFile(snaps[0].ZipPath, keep, 0o666); err != nil {
		t.Fatal(err)
	}
	archive, done, err := OpenArchive(snaps[0].ZipPath)
	if err != nil || archive != snaps[0].ZipPath {
		t.Fatalf("open = %q, %v; want the zip", archive, err)
	}
	done()
	env.mgr.CompactAll(context.Background(), 0, 0)
	if !compactedAt(snaps[0].ZipPath) {
		t.Fatal("the next pass did not finish it")
	}
	archive, done, _ = OpenArchive(snaps[0].ZipPath)
	sameEntries(t, "finished", readWhole(t, archive), originals[0])
	done()
}

// An archive from another zip writer — no extended timestamps, a different
// compression — comes back the same.
func TestArchivesFromElsewhereRoundTrip(t *testing.T) {
	env := setup(t)
	snaps, _ := threeSnapshots(t, env)
	// Rewrite the first as a plain zip with MS-DOS times only.
	path := snaps[0].ZipPath
	out, err := os.Create(path + ".new")
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(out)
	big := noise(t, 60<<10)
	for _, e := range []struct {
		name string
		body string
	}{{"dir/", ""}, {"dir/big.sav", big}, {"dir/small.txt", "x"}} {
		fh := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		fh.ModifiedDate, fh.ModifiedTime = 0x5a21, 0x6c3e
		fw, err := w.CreateHeader(fh)
		if err != nil {
			t.Fatal(err)
		}
		io.WriteString(fw, e.body)
	}
	w.Close()
	out.Close()
	if err := os.Rename(path+".new", path); err != nil {
		t.Fatal(err)
	}
	want := readWhole(t, path)

	env.mgr.CompactAll(context.Background(), 0, 0)
	if !compactedAt(path) {
		t.Fatal("not compacted")
	}
	archive, done, err := OpenArchive(path)
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	sameEntries(t, "from elsewhere", readWhole(t, archive), want)
}

func forgetSharedChecks() {
	sharedChecked.Range(func(k, _ any) bool { sharedChecked.Delete(k); return true })
}

// Another process working on the store — the app while `opensave` runs from
// a terminal — keeps this one out of it: no compaction, no clean-up, and
// both happen on a later pass once it is done.
func TestAnotherProcessOnTheStoreIsWaitedFor(t *testing.T) {
	env := setup(t)
	snaps, _ := threeSnapshots(t, env)
	root := storeRoot(snaps[0].ZipPath)
	os.MkdirAll(filepath.Join(root, sharedDirName), 0o777)
	unlock, ok, err := fsx.TryLock(filepath.Join(root, sharedDirName, ".lock"))
	if err != nil || !ok {
		t.Fatalf("lock: %v %v", ok, err)
	}
	res, err := env.mgr.CompactAll(context.Background(), 0, 0)
	if err != nil || res.Compacted != 0 || res.Failed != 0 {
		t.Errorf("compaction with the store taken = %+v, %v; want nothing done and nothing failed", res, err)
	}
	unlock()
	env.mgr.CompactAll(context.Background(), 0, 0)
	if !compactedAt(snaps[0].ZipPath) {
		t.Fatal("not compacted once the store was free")
	}

	unlock, _, _ = fsx.TryLock(filepath.Join(root, sharedDirName, ".lock"))
	env.mgr.DeleteSnapshot("game1", snaps[0].ID)
	if n := len(sharedFiles(t, root)); n != 5 {
		t.Errorf("%d shared files: cleaned up while another process had the store", n)
	}
	unlock()
	env.mgr.CompactAll(context.Background(), 0, 0)
	if n := len(sharedFiles(t, root)); n != 4 {
		t.Errorf("%d shared files after the store was free, want 4", n)
	}
}

// Switching branch puts back the newest snapshot of the branch switched to.
// That one is never compacted while it is the newest — but deleting the
// newest makes an older, compacted one the newest.
func TestSwitchingToABranchWhoseNewestIsCompacted(t *testing.T) {
	env := setup(t)
	snaps, originals := threeSnapshots(t, env)
	// Over to another branch: the switch keeps the save on main first, and
	// that copy is main's newest.
	if _, err := env.mgr.CreateBranch("game1", "other", true); err != nil {
		t.Fatal(err)
	}
	if err := env.mgr.SwitchBranch("game1", "other"); err != nil {
		t.Fatal(err)
	}
	mainSnaps, _ := env.store.ListSnapshots("game1", "main")
	if _, err := env.mgr.CompactAll(context.Background(), 0, 0); err != nil {
		t.Fatal(err)
	}
	// Without that copy, main's newest is the third snapshot, compacted.
	if _, err := env.mgr.DeleteSnapshot("game1", mainSnaps[0].ID); err != nil {
		t.Fatal(err)
	}
	if !compactedAt(snaps[2].ZipPath) {
		t.Fatal("setup: main's newest is not compacted")
	}

	writeSave(t, env.saveDir, "slot4.sav", "played on the other branch")
	if err := env.mgr.SwitchBranch("game1", "main"); err != nil {
		t.Fatal(err)
	}
	for _, e := range originals[2] {
		if e.body == nil {
			continue
		}
		got, err := os.ReadFile(filepath.Join(env.saveDir, filepath.FromSlash(e.name)))
		if err != nil || !bytes.Equal(got, e.body) {
			t.Errorf("after switching back, %s is not the compacted snapshot's", e.name)
		}
	}
}

// The same file stored two ways — uncompressed by a snapshot from before
// compression, deflated by a later one — is two shared files: each archive
// comes back as it was written.
func TestTheSameFileStoredTwoWaysIsKeptBothWays(t *testing.T) {
	env := setup(t)
	snaps, _ := threeSnapshots(t, env)
	// The first snapshot rewritten as an older version wrote it: nothing
	// compressed, and a file that compresses well.
	// Hex: it deflates to about half, still well past the size for sharing.
	body := hex.EncodeToString([]byte(noise(t, 60<<10)))
	writeArchive := func(path string, method uint16) []byte {
		var buf bytes.Buffer
		w := zip.NewWriter(&buf)
		fw, _ := w.CreateHeader(&zip.FileHeader{Name: "big.sav", Method: method, Modified: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)})
		io.WriteString(fw, body)
		w.Close()
		if err := os.WriteFile(path, buf.Bytes(), 0o666); err != nil {
			t.Fatal(err)
		}
		return buf.Bytes()
	}
	stored := writeArchive(snaps[0].ZipPath, zip.Store)
	deflated := writeArchive(snaps[1].ZipPath, zip.Deflate)
	if _, err := env.mgr.CompactAll(context.Background(), 0, 0); err != nil {
		t.Fatal(err)
	}
	for i, want := range [][]byte{stored, deflated} {
		if !compactedAt(snaps[i].ZipPath) {
			t.Fatalf("snapshot %d not compacted", i)
		}
		archive, done, err := OpenArchive(snaps[i].ZipPath)
		if err != nil {
			t.Fatal(err)
		}
		got, _ := os.ReadFile(archive)
		done()
		if !bytes.Equal(got, want) {
			t.Errorf("snapshot %d came back %d bytes, not the %d it was written as", i, len(got), len(want))
		}
	}
}
