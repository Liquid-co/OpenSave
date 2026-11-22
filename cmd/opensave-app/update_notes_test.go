package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testChangelog = `# Changelog

## [2.5.1] — 2026-12-01

A small release.

### Fixed

- **Pausing syncing** from the Library sticks.

## [2.5.0-beta.1] — 2026-11-19

### Fixed

- **Older** things.
`

func withChangelogServer(t *testing.T, status int, body string) *string {
	t.Helper()
	var asked string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = r.URL.Path
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	old := changelogAtTag
	changelogAtTag = func(repo, tag string) string { return srv.URL + "/" + repo + "/" + tag + "/CHANGELOG.md" }
	t.Cleanup(func() { changelogAtTag = old })
	return &asked
}

func TestUpcomingReleaseReadsTheTagsChangelog(t *testing.T) {
	asked := withChangelogServer(t, http.StatusOK, testChangelog)
	r := upcomingRelease("Liquid-co/OpenSave", "v2.5.1", "2.5.1")
	if r == nil {
		t.Fatal("no release found")
	}
	if *asked != "/Liquid-co/OpenSave/v2.5.1/CHANGELOG.md" {
		t.Errorf("read %q", *asked)
	}
	if r.Version != "2.5.1" || r.Date != "2026-12-01" {
		t.Errorf("got %+v", r)
	}
	if len(r.Sections) != 1 || r.Sections[0].Title != "Fixed" || r.Sections[0].Entries[0].Lead != "Pausing syncing" {
		t.Errorf("sections %+v", r.Sections)
	}
	if len(r.Intro) != 1 || r.Intro[0] != "A small release." {
		t.Errorf("intro %+v", r.Intro)
	}
}

func TestUpcomingReleaseIsNilWithoutIt(t *testing.T) {
	withChangelogServer(t, http.StatusOK, testChangelog)
	if r := upcomingRelease("Liquid-co/OpenSave", "v2.6.0", "2.6.0"); r != nil {
		t.Errorf("found %+v for a version the file does not have", r)
	}
	withChangelogServer(t, http.StatusNotFound, "nope")
	if r := upcomingRelease("Liquid-co/OpenSave", "v2.5.1", "2.5.1"); r != nil {
		t.Errorf("found %+v in a 404", r)
	}
	withChangelogServer(t, http.StatusOK, strings.Repeat("x", maxChangelogBytes+10))
	if r := upcomingRelease("Liquid-co/OpenSave", "v2.5.1", "2.5.1"); r != nil {
		t.Errorf("found %+v in an oversized file", r)
	}
}
