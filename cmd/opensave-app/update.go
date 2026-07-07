package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// updateRepo is the GitHub "owner/repo" whose releases are checked for a
// newer version. Change this one line if the project moves.
const updateRepo = "sivadaboi/OpenSave"

// CheckForUpdate best-effort asks GitHub for the latest published release
// and reports whether it is newer than the running build. Any failure
// (offline, rate-limited, no releases) resolves to "no update" so the UI
// never blocks or errors on it.
func (a *App) CheckForUpdate() map[string]any {
	none := map[string]any{"available": false, "current": AppVersion}

	req, err := http.NewRequest(http.MethodGet,
		"https://api.github.com/repos/"+updateRepo+"/releases/latest", nil)
	if err != nil {
		return none
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "OpenSave/"+AppVersion)

	resp, err := (&http.Client{Timeout: 6 * time.Second}).Do(req)
	if err != nil {
		return none
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return none
	}

	var rel struct {
		TagName    string `json:"tag_name"`
		HTMLURL    string `json:"html_url"`
		Prerelease bool   `json:"prerelease"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return none
	}
	if rel.Prerelease || rel.TagName == "" {
		return none
	}

	latest := strings.TrimPrefix(rel.TagName, "v")
	if compareVersions(latest, AppVersion) <= 0 {
		return none
	}
	url := rel.HTMLURL
	if url == "" {
		url = "https://github.com/" + updateRepo + "/releases/latest"
	}
	return map[string]any{
		"available": true,
		"current":   AppVersion,
		"latest":    latest,
		"url":       url,
	}
}

// compareVersions returns -1, 0, or 1 comparing dotted numeric versions
// (e.g. "2.1.0" vs "2.0.3"). Non-numeric or missing parts count as 0.
func compareVersions(a, b string) int {
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	n := len(as)
	if len(bs) > n {
		n = len(bs)
	}
	for i := 0; i < n; i++ {
		ai, bi := 0, 0
		if i < len(as) {
			ai, _ = strconv.Atoi(strings.TrimSpace(as[i]))
		}
		if i < len(bs) {
			bi, _ = strconv.Atoi(strings.TrimSpace(bs[i]))
		}
		if ai != bi {
			if ai < bi {
				return -1
			}
			return 1
		}
	}
	return 0
}
