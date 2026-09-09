package fsx

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// A game must be able to delete its own save while OpenSave is reading it.
//
// On Windows that is not the default: os.Open omits FILE_SHARE_DELETE, so for
// as long as OpenSave holds a save open — and a manifest build walks every
// tracked save on a timer — the game cannot delete it. The test is written
// from the game's side: hold the file the way OpenSave does, then do what a
// game does, and require it to work.
func TestOpenShared_DoesNotBlockTheGameFromDeleting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "player.sav")
	if err := os.WriteFile(path, []byte("original"), 0o666); err != nil {
		t.Fatal(err)
	}

	f, err := OpenShared(path)
	if err != nil {
		t.Fatalf("OpenShared: %v", err)
	}
	defer f.Close()

	if err := os.Remove(path); err != nil {
		t.Errorf("a game could not delete its own save while OpenSave had it open: %v", err)
	}
}

// The limit of what the share mode can do, recorded as a test so the next
// person does not spend the afternoon rediscovering it.
//
// Windows refuses to replace a file that has any open handle, whatever share
// mode it was opened with, and granting the handle DELETE access does not
// change that. Since writing a temp file and renaming it over the original is
// how many games save safely, OpenSave reading at the wrong moment can still
// make a game's save fail. The mitigation is holding files open as briefly as
// possible and not opening unchanged ones at all — the hash cache — not the
// share mode.
//
// Asserted in whichever direction the platform actually takes, so this stays a
// statement of fact rather than a wish.
func TestOpenShared_RenameOverAnOpenFileIsAnOsLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "player.sav")
	if err := os.WriteFile(path, []byte("original"), 0o666); err != nil {
		t.Fatal(err)
	}
	f, err := OpenShared(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	tmp := filepath.Join(dir, "player.sav.tmp")
	if err := os.WriteFile(tmp, []byte("new progress"), 0o666); err != nil {
		t.Fatal(err)
	}
	err = os.Rename(tmp, path)

	if runtime.GOOS == "windows" {
		if err == nil {
			t.Log("NOTE: renaming over an open file now succeeds on this Windows build. " +
				"If that holds generally, the hash cache is no longer the only thing " +
				"protecting a game's save from a badly timed read.")
		}
		return
	}
	if err != nil {
		t.Errorf("renaming over an open file failed on %s, where it should always work: %v",
			runtime.GOOS, err)
	}
}

// The reading has to still work, obviously.
func TestOpenShared_ReadsTheContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "player.sav")
	want := "the actual bytes"
	if err := os.WriteFile(path, []byte(want), 0o666); err != nil {
		t.Fatal(err)
	}
	f, err := OpenShared(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	got := make([]byte, len(want))
	if _, err := f.Read(got); err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Errorf("read %q, want %q", got, want)
	}
	// Stat through the handle, which the hashing path relies on.
	if st, err := f.Stat(); err != nil {
		t.Errorf("Stat on a shared handle: %v", err)
	} else if st.Size() != int64(len(want)) {
		t.Errorf("size %d, want %d", st.Size(), len(want))
	}
}

// Errors have to keep the shape callers already test for, or every
// os.IsNotExist check upstream silently stops working.
func TestOpenShared_MissingFileLooksMissing(t *testing.T) {
	_, err := OpenShared(filepath.Join(t.TempDir(), "nope.sav"))
	if err == nil {
		t.Fatal("opening a missing file returned no error")
	}
	if !os.IsNotExist(err) {
		t.Errorf("os.IsNotExist said no for a missing file: %v (%T)", err, err)
	}
	var pathErr *os.PathError
	if !asPathError(err, &pathErr) {
		t.Errorf("error is not an *os.PathError: %T", err)
	}
}

func asPathError(err error, target **os.PathError) bool {
	pe, ok := err.(*os.PathError)
	if ok {
		*target = pe
	}
	return ok
}

// Deep save folders are common — a Windows profile path is long before a game
// adds anything — and a hand-rolled CreateFileW loses the long-path handling
// os.Open does for free. Rather than reimplement it, openShared falls back to
// os.Open whenever CreateFileW will not take the path, so this must pass on
// any path the standard library can open.
func TestOpenShared_HandlesAPathBeyondMaxPath(t *testing.T) {
	root := t.TempDir()
	deep := root
	for len(deep) < 300 {
		deep = filepath.Join(deep, strings.Repeat("s", 40))
	}
	if err := os.MkdirAll(deep, 0o777); err != nil {
		t.Skipf("could not create a long path on this system: %v", err)
	}
	path := filepath.Join(deep, "player.sav")
	if err := os.WriteFile(path, []byte("deep"), 0o666); err != nil {
		t.Skipf("could not write at a long path on this system: %v", err)
	}
	// os.Open is the benchmark: whatever it manages, OpenShared must manage.
	if ref, refErr := os.Open(path); refErr != nil {
		t.Skipf("os.Open cannot open this path either, so there is nothing to compare: %v", refErr)
	} else {
		ref.Close()
	}

	f, err := OpenShared(path)
	if err != nil {
		t.Fatalf("OpenShared failed on a %d-character path that os.Open opens fine: %v",
			len(path), err)
	}
	f.Close()
}
