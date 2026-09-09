package selfupdate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Verifying a downloaded update against the checksums published with it.
//
// Before this, the only check between a download and a rename over the running
// binary was "is it about the right size and does it start with MZ". These
// tests exist to make sure a file that is not the published one cannot get
// past — including the cases where it would be tempting to shrug and continue.

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func sumOf(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// sumsServer serves a release's checksums over TLS.
//
// TLS rather than plain HTTP because SumsURLFor refuses anything but https —
// the release comes from GitHub and a downgrade would be the first thing an
// attacker reached for. Testing against http:// would have meant relaxing that
// check for the tests' convenience, which is how a real one gets lost.
func sumsServer(t *testing.T, body string, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/"+sumsFileName) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
	old := sumsClient
	sumsClient = srv.Client() // trusts this server's throwaway certificate
	t.Cleanup(func() { sumsClient = old; srv.Close() })
	return srv
}

func TestSumsURLFor(t *testing.T) {
	sums, name, err := SumsURLFor("https://github.com/o/r/releases/download/v2.4.0/OpenSave.exe")
	if err != nil {
		t.Fatal(err)
	}
	if name != "OpenSave.exe" {
		t.Errorf("asset name = %q", name)
	}
	if sums != "https://github.com/o/r/releases/download/v2.4.0/SHA256SUMS" {
		t.Errorf("sums URL = %q", sums)
	}
}

func TestSumsURLFor_RejectsNonHTTPS(t *testing.T) {
	for _, bad := range []string{
		"http://github.com/o/r/releases/download/v1/x.exe",
		"file:///tmp/x.exe",
	} {
		if _, _, err := SumsURLFor(bad); err == nil {
			t.Errorf("SumsURLFor(%q) was accepted", bad)
		}
	}
}

func TestExpectedSum_ReadsBothCoreutilsForms(t *testing.T) {
	body := "aaa  plain.exe\nbbb *binary.exe\nccc  nested/dir/deep.exe\n"
	for _, c := range []struct{ name, want string }{
		{"plain.exe", "aaa"},
		{"binary.exe", "bbb"},
		{"deep.exe", "ccc"},
	} {
		if got, ok := ExpectedSum([]byte(body), c.name); !ok || got != c.want {
			t.Errorf("ExpectedSum(%s) = %q,%v; want %q", c.name, got, ok, c.want)
		}
	}
	if _, ok := ExpectedSum([]byte(body), "absent.exe"); ok {
		t.Error("a file not listed was reported as found")
	}
}

func TestVerifyDownload_AcceptsAMatchingFile(t *testing.T) {
	content := "the real release binary"
	file := writeTemp(t, "OpenSave.exe", content)
	srv := sumsServer(t, sumOf(content)+"  OpenSave.exe\n", http.StatusOK)

	if err := VerifyDownload(srv.URL+"/releases/download/v2.4.0/OpenSave.exe", file); err != nil {
		t.Fatalf("a matching download was rejected: %v", err)
	}
}

// The case the whole thing exists for: a file that is not what was published.
func TestVerifyDownload_RejectsATamperedFile(t *testing.T) {
	published := "the real release binary"
	file := writeTemp(t, "OpenSave.exe", "MZ...definitely not that")
	srv := sumsServer(t, sumOf(published)+"  OpenSave.exe\n", http.StatusOK)

	err := VerifyDownload(srv.URL+"/releases/download/v2.4.0/OpenSave.exe", file)
	if err == nil {
		t.Fatal("a tampered download was accepted")
	}
	if !strings.Contains(err.Error(), "does not match the checksum") {
		t.Errorf("unhelpful error for a mismatch: %v", err)
	}
}

// Fail closed: no checksums file must not mean "install it anyway".
func TestVerifyDownload_RefusesWhenNoChecksumsArePublished(t *testing.T) {
	file := writeTemp(t, "OpenSave.exe", "anything")
	srv := sumsServer(t, "not found", http.StatusNotFound)

	if err := VerifyDownload(srv.URL+"/releases/download/v2.4.0/OpenSave.exe", file); err == nil {
		t.Fatal("a release with no SHA256SUMS was accepted")
	}
}

// Nor must an asset simply being absent from the list.
func TestVerifyDownload_RefusesWhenTheAssetIsNotListed(t *testing.T) {
	file := writeTemp(t, "OpenSave.exe", "anything")
	srv := sumsServer(t, sumOf("x")+"  SomethingElse.exe\n", http.StatusOK)

	err := VerifyDownload(srv.URL+"/releases/download/v2.4.0/OpenSave.exe", file)
	if err == nil {
		t.Fatal("an asset missing from SHA256SUMS was accepted")
	}
	if !strings.Contains(err.Error(), "does not list") {
		t.Errorf("unhelpful error: %v", err)
	}
}

// Case differences in the published hash must not read as a mismatch.
func TestVerifyDownload_IsCaseInsensitiveOnTheHash(t *testing.T) {
	content := "release"
	file := writeTemp(t, "OpenSave.exe", content)
	srv := sumsServer(t, strings.ToUpper(sumOf(content))+"  OpenSave.exe\n", http.StatusOK)

	if err := VerifyDownload(srv.URL+"/releases/download/v2.4.0/OpenSave.exe", file); err != nil {
		t.Fatalf("an uppercase hash was treated as a mismatch: %v", err)
	}
}

func TestFileSum(t *testing.T) {
	file := writeTemp(t, "x", "hello")
	got, err := FileSum(file)
	if err != nil {
		t.Fatal(err)
	}
	if got != sumOf("hello") {
		t.Errorf("FileSum = %q", got)
	}
}
