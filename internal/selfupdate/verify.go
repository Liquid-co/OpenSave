package selfupdate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
)

// Checking a downloaded update against the checksums published with it.
//
// Until this existed, ValidateExecutable was the only thing between a
// downloaded file and a rename over the running binary — and it checks the
// size and the first two bytes. Anything beginning "MZ" passed. The transport
// was TLS to GitHub, which protects the connection and says nothing about the
// artifact: a swapped release asset, or a compromised account, would have been
// installed and run without a murmur.
//
// The relay installer has verified against SHA256SUMS since it shipped. The
// in-app updater, which is how nearly everyone actually updates, did not. This
// closes that gap using the same file.
//
// It does NOT make updates signed. A SHA256SUMS fetched from the same release
// as the asset is only as trustworthy as the release itself — an attacker who
// can replace one can replace the other. What it does defeat is the narrower
// and likelier case: an asset altered or truncated in transit or at rest while
// the checksums file was not. Real signing needs a key that never touches CI,
// and is the next step rather than this one.

// sumsFileName is the checksums file published beside every release asset.
const sumsFileName = "SHA256SUMS"

// sumsClient is separate from the download client: fetching a few hundred
// bytes of checksums must not inherit a timeout sized for a 25 MB binary.
var sumsClient = &http.Client{Timeout: 30 * time.Second}

// SumsURLFor derives the checksums URL that sits beside a release asset, and
// the asset's own file name.
//
// GitHub serves every asset of a release from one directory, so replacing the
// last path segment finds the checksums — the same derivation install-relay.sh
// makes.
func SumsURLFor(assetURL string) (sumsURL, assetName string, err error) {
	u, err := url.Parse(assetURL)
	if err != nil {
		return "", "", fmt.Errorf("update URL is not a URL: %w", err)
	}
	if u.Scheme != "https" {
		return "", "", fmt.Errorf("update URL must be https, got %q", u.Scheme)
	}
	assetName = path.Base(u.Path)
	if assetName == "" || assetName == "." || assetName == "/" {
		return "", "", fmt.Errorf("update URL names no file: %s", assetURL)
	}
	sums := *u
	sums.Path = path.Join(path.Dir(u.Path), sumsFileName)
	sums.RawQuery = ""
	sums.Fragment = ""
	return sums.String(), assetName, nil
}

// ExpectedSum finds one file's checksum in the body of a SHA256SUMS file.
//
// Accepts both the plain and the "binary mode" spellings coreutils produces
// ("<hash>  name" and "<hash> *name"), and ignores any leading directory on
// the recorded name, because the generator strips paths and a future one might
// not.
func ExpectedSum(sums []byte, assetName string) (string, bool) {
	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimPrefix(fields[1], "*")
		if path.Base(name) == assetName {
			return strings.ToLower(fields[0]), true
		}
	}
	return "", false
}

// FileSum is the SHA-256 of a file, lowercase hex.
func FileSum(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// VerifyDownload checks a downloaded asset against the checksums published
// with its release.
//
// Fails closed on every uncertainty — no checksums file, the asset absent from
// it, an unreadable download. Refusing to update is a recoverable annoyance;
// running an unverified binary is not, and "the checksums were missing so we
// installed it anyway" is precisely the reasoning an attacker would want.
func VerifyDownload(assetURL, downloadedPath string) error {
	sumsURL, assetName, err := SumsURLFor(assetURL)
	if err != nil {
		return err
	}

	resp, err := sumsClient.Get(sumsURL)
	if err != nil {
		return fmt.Errorf("could not fetch %s to verify the download: %w", sumsFileName, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("this release publishes no %s (HTTP %d), so the download cannot be verified",
			sumsFileName, resp.StatusCode)
	}
	// Bounded: a checksums file is a few hundred bytes, and this must not be
	// a way to make the app read an unbounded response into memory.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("could not read %s: %w", sumsFileName, err)
	}

	want, ok := ExpectedSum(body, assetName)
	if !ok {
		return fmt.Errorf("%s does not list %s, so the download cannot be verified", sumsFileName, assetName)
	}
	got, err := FileSum(downloadedPath)
	if err != nil {
		return fmt.Errorf("could not hash the download: %w", err)
	}
	if !strings.EqualFold(want, got) {
		return fmt.Errorf(
			"the downloaded %s does not match the checksum published with the release.\n"+
				"  expected %s\n  actually %s\n"+
				"This update has not been installed. It may have been corrupted in transit — "+
				"try again, and if it keeps happening, report it before installing anything.",
			assetName, want, got)
	}
	return nil
}
