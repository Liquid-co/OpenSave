package relay

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// A relay with nothing configured runs fine — it just answers "not configured"
// for the features that need a secret. A missing file must not be an error.
func TestNoSecretsFileIsNotAnError(t *testing.T) {
	s, err := LoadSecrets(filepath.Join(t.TempDir(), "absent.json"))
	if err != nil {
		t.Fatalf("a missing secrets file errored: %v", err)
	}
	if s.GoogleClientSecret != "" || s.SteamGridDBKey != "" {
		t.Errorf("a missing file produced values: %+v", s)
	}
}

func TestSecretsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.json")
	want := SecretsFile{GoogleClientSecret: "goog-abc", SteamGridDBKey: "grid-xyz"}
	if err := SaveSecrets(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSecrets(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("round trip = %+v, want %+v", got, want)
	}
}

// The file holds secrets, so it is written 0600 — and written that way from the
// first byte rather than chmod'ed afterwards, which would leave a window where
// it is readable.
//
// Windows is exempt because Go's Chmod there toggles the read-only bit and
// nothing else. Asserting 0600 on Windows would be asserting something the
// platform does not do.
func TestSecretsFileIsNotWorldReadable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Go cannot set POSIX modes on Windows; the file inherits the folder's ACL")
	}
	path := filepath.Join(t.TempDir(), "secrets.json")
	if err := SaveSecrets(path, SecretsFile{SteamGridDBKey: "grid-xyz"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("mode = %o, want 600 — this file holds a secret", mode)
	}
}

// Overwriting a file that already exists with looser permissions must tighten
// it, not inherit what was there.
func TestSavingOverALooseFileTightensIt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no POSIX modes on Windows")
	}
	path := filepath.Join(t.TempDir(), "secrets.json")
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SaveSecrets(path, SecretsFile{SteamGridDBKey: "x"}); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("mode = %o after overwriting a 0644 file, want 600", mode)
	}
}

// A container or systemd unit that already injects secrets has to keep working
// exactly as it did, and must never be overridden by a file someone left on
// the box.
func TestTheEnvironmentBeatsTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.json")
	if err := SaveSecrets(path, SecretsFile{
		GoogleClientSecret: "from-file", SteamGridDBKey: "from-file",
	}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("STEAMGRIDDB_KEY", "from-env")
	t.Setenv("GOOGLE_DRIVE_CLIENT_SECRET", "")

	got, sources := ResolveSecrets(path)
	if got.SteamGridDBKey != "from-env" {
		t.Errorf("SteamGridDBKey = %q, want the environment to win", got.SteamGridDBKey)
	}
	if !strings.HasPrefix(sources["steamGridDBKey"], "environment") {
		t.Errorf("source = %q, want it reported as the environment", sources["steamGridDBKey"])
	}
	// An empty environment variable is not a value — it must not blank out a
	// configured secret, or exporting an empty var silently disables a feature.
	if got.GoogleClientSecret != "from-file" {
		t.Errorf("an empty env var overrode the file: %q", got.GoogleClientSecret)
	}
}

// Nothing configured anywhere is a normal state and has to be reported as
// such, so an operator can tell "not set" from "set to something wrong".
func TestUnsetIsReportedAsUnset(t *testing.T) {
	t.Setenv("STEAMGRIDDB_KEY", "")
	t.Setenv("GOOGLE_DRIVE_CLIENT_SECRET", "")
	_, sources := ResolveSecrets(filepath.Join(t.TempDir(), "absent.json"))
	for _, name := range []string{"googleClientSecret", "steamGridDBKey"} {
		if sources[name] != "not set" {
			t.Errorf("%s reported as %q, want \"not set\"", name, sources[name])
		}
	}
}

// A corrupt file must be reported rather than read as "nothing configured",
// which would silently disable cloud sync and cover art at once.
func TestACorruptSecretsFileIsReported(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSecrets(path); err == nil {
		t.Error("a corrupt secrets file loaded without complaint")
	}
}

// Masking exists so a value can be confirmed without being revealed. It must
// never print enough to be useful to someone reading over a shoulder or a log.
func TestMaskingRevealsOnlyTheTail(t *testing.T) {
	// Shaped like a real SteamGridDB key — 32 hex characters — because the
	// last-four assertion below is only meaningful against a realistic
	// length. Not a real one, and it must never become one: this file is
	// public, and a key committed here is a key anyone can lift out and get
	// rate-limited for every user. Real keys go in the secrets file that
	// this very code exists to provide. See secrets.go and docs/RELAY.md.
	const secret = "0123456789abcdef0123456789abcdef"
	masked := MaskSecret(secret)
	if strings.Contains(masked, secret[:len(secret)-4]) {
		t.Errorf("MaskSecret leaked the body of the secret: %q", masked)
	}
	if !strings.HasSuffix(masked, secret[len(secret)-4:]) {
		t.Errorf("MaskSecret = %q, want it to end in the last four characters", masked)
	}
	if MaskSecret("") != "(not set)" {
		t.Errorf("an unset secret should say so, got %q", MaskSecret(""))
	}
	// A short value must not be shown whole just because it is short.
	if got := MaskSecret("abcd"); strings.Contains(got, "abcd") {
		t.Errorf("MaskSecret(%q) = %q, which reveals it", "abcd", got)
	}
}
