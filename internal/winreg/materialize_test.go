package winreg

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Two devices must capture the same save in the same shape, or the file's hash
// disagrees and the two look permanently in conflict over nothing.
func TestKeyOrderAndSpellingAreCanonical(t *testing.T) {
	a := dedupeKeys([]string{
		`HKCU\Software\B`,
		"HKEY_CURRENT_USER/Software/A",
		"hkcu/software/a", // the same key, spelled differently
		"   ",
	})
	if len(a) != 2 {
		t.Fatalf("dedupeKeys = %v, want two distinct keys", a)
	}
	if a[0] > a[1] {
		t.Errorf("dedupeKeys returned an unsorted result: %v", a)
	}
}

// Nothing to capture writes nothing — a game with no registry keys must not
// gain an empty location that then syncs back and forth.
func TestNoKeysWritesNoFile(t *testing.T) {
	dir := t.TempDir()
	warns, err := CaptureToDir(nil, dir)
	if err != nil || len(warns) != 0 {
		t.Fatalf("CaptureToDir(nil) = (%v, %v)", warns, err)
	}
	if _, err := os.Stat(filepath.Join(dir, FileName)); !os.IsNotExist(err) {
		t.Error("a capture file was written for a game with no registry keys")
	}
}

// A key that does not exist is recorded as missing rather than dropped: "the
// game has never written this" is different from "nothing was asked for", and
// only the first tells a reader the capture actually looked.
func TestAMissingKeyIsRecordedNotDropped(t *testing.T) {
	if !Available() {
		t.Skip("no registry on this platform")
	}
	dir := t.TempDir()
	key := `HKEY_CURRENT_USER\Software\OpenSaveTest\definitely-absent`
	warns, err := CaptureToDir([]string{key}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) != 0 {
		t.Errorf("a key that has never been written is not a warning: %v", warns)
	}
	raw, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatal(err)
	}
	var snap Snapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		t.Fatal(err)
	}
	if len(snap.Missing) != 1 || len(snap.Keys) != 0 {
		t.Errorf("snapshot = %+v, want the key recorded as missing", snap)
	}
	if len(snap.Requested) != 1 {
		t.Errorf("the capture must record what it was asked for, got %v", snap.Requested)
	}
}

// A snapshot from before this feature, or of a game with no keys, has nothing
// to put back — and must not be treated as a failure.
func TestRestoringADirectoryWithNoCaptureIsSilent(t *testing.T) {
	warns, err := RestoreFromDir(t.TempDir())
	if err != nil || len(warns) != 0 {
		t.Errorf("RestoreFromDir(empty) = (%v, %v), want silence", warns, err)
	}
}

// A corrupt capture is reported rather than ignored: silently restoring
// nothing would look like a successful restore of a save that is now gone.
func TestACorruptCaptureIsReported(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := RestoreFromDir(dir); err == nil {
		t.Error("a corrupt capture restored without complaint")
	}
}

// The capture file must not be dot-prefixed. A dot-named entry is excluded
// from every manifest, which is what makes .opensave-locations/ safe for older
// builds — and is exactly what would stop a registry capture from syncing.
func TestTheCaptureFileIsVisibleToManifests(t *testing.T) {
	if strings.HasPrefix(FileName, ".") {
		t.Errorf("FileName = %q — a dot-prefixed file is skipped by delta.isDotEntry, "+
			"so the capture would never be hashed and registry changes would never sync", FileName)
	}
}
