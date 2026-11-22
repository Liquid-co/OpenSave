package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/opensave/opensave/internal/changelog"
)

// What a newer release changes, for the app's "What's new" to show before it
// is installed.
//
// A GitHub release's own description is not it: that is kept to a Discord
// invitation and a link (see .github/workflows/release.yml). The changes are
// told in CHANGELOG.md, so the file is read as it stands at the release's tag
// and the release's section taken from it, parsed the same way as the
// changelog built into the app.

// changelogAtTag is where a release's CHANGELOG.md is read from; a variable so
// a test can point it at a server of its own.
var changelogAtTag = func(repo, tag string) string {
	return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/CHANGELOG.md", repo, tag)
}

// The file is a few hundred kilobytes; anything far past that is not it.
const maxChangelogBytes = 2 << 20

var notesClient = &http.Client{Timeout: 8 * time.Second}

// upcomingRelease returns the section of the changelog at `tag` for version
// `ver`, or nil when it cannot be had: offline, no such file, or no section
// for that version. Best effort, like the update check it serves.
func upcomingRelease(repo, tag, ver string) *changelog.Release {
	req, err := http.NewRequest(http.MethodGet, changelogAtTag(repo, tag), nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "OpenSave/"+AppVersion)
	resp, err := notesClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxChangelogBytes+1))
	if err != nil || len(body) > maxChangelogBytes {
		return nil
	}
	want := strings.TrimPrefix(ver, "v")
	for _, r := range changelog.Parse(string(body)) {
		if strings.TrimPrefix(r.Version, "v") == want {
			r := r
			return &r
		}
	}
	return nil
}
