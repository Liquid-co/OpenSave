package cloud

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/opensave/opensave/internal/store"
)

// A listing is every page, not the first.
//
// Each provider answers a folder listing a page at a time — Drive a hundred
// files unless asked for more, Dropbox and OneDrive at their own sizes — and
// List read one page. Past that the cloud screens showed an arbitrary subset,
// a restore could not find the rest, and `cloud push` re-uploaded what it
// could not see. These serve three pages and expect all of them.

func pagedNames(page int) []string {
	return []string{
		fmt.Sprintf("game__main__snap_%d1.zip", page),
		fmt.Sprintf("game__main__snap_%d2.zip", page),
	}
}

func assertAllPages(t *testing.T, files []CloudFile, pages int) {
	t.Helper()
	got := map[string]bool{}
	for _, f := range files {
		got[f.Name] = true
	}
	for p := 0; p < pages; p++ {
		for _, name := range pagedNames(p) {
			if !got[name] {
				t.Errorf("%s (page %d) is missing from the listing of %d files", name, p+1, len(files))
			}
		}
	}
}

func TestGoogleDriveListReadsEveryPage(t *testing.T) {
	const pages = 3
	drive := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/drive/v3/files" || !strings.Contains(r.URL.Query().Get("q"), "application/zip") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		page := 0
		if tok := r.URL.Query().Get("pageToken"); tok != "" {
			fmt.Sscanf(tok, "page-%d", &page)
		}
		var files []map[string]any
		for i, name := range pagedNames(page) {
			files = append(files, map[string]any{"id": fmt.Sprintf("f%d%d", page, i), "name": name, "size": "10", "createdTime": "2026-09-01T00:00:00Z"})
		}
		out := map[string]any{"files": files}
		if page+1 < pages {
			out["nextPageToken"] = fmt.Sprintf("page-%d", page+1)
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
	defer drive.Close()

	svc, s := newTestService(t)
	setCloudConfig(t, s, func(c *store.CloudConfig) {
		c.Enabled = true
		c.Provider = "google_drive"
		c.AccessToken = "at"
		c.ExpiryTimeMs = time.Now().UnixMilli() + 3600_000
		c.FolderID = "folder1"
	})
	svc.Endpoints.GoogleAPI = drive.URL

	files, err := svc.List()
	if err != nil {
		t.Fatal(err)
	}
	assertAllPages(t, files, pages)
}

func TestDropboxListReadsEveryPage(t *testing.T) {
	const pages = 3
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := 0
		if r.URL.Path == "/2/files/list_folder/continue" {
			var body struct {
				Cursor string `json:"cursor"`
			}
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &body)
			fmt.Sscanf(body.Cursor, "cursor-%d", &page)
		} else if r.URL.Path != "/2/files/list_folder" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var entries []map[string]any
		for _, name := range pagedNames(page) {
			entries = append(entries, map[string]any{".tag": "file", "name": name, "size": 10, "client_modified": "2026-09-01T00:00:00Z"})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"entries":  entries,
			"cursor":   fmt.Sprintf("cursor-%d", page+1),
			"has_more": page+1 < pages,
		})
	}))
	defer api.Close()

	svc, s := newTestService(t)
	setCloudConfig(t, s, func(c *store.CloudConfig) {
		c.Enabled = true
		c.Provider = "dropbox"
		c.AccessToken = "at"
		c.ExpiryTimeMs = time.Now().UnixMilli() + 3600_000
	})
	svc.Endpoints.DropboxAPI = api.URL

	files, err := svc.List()
	if err != nil {
		t.Fatal(err)
	}
	assertAllPages(t, files, pages)
}

func TestOneDriveListReadsEveryPage(t *testing.T) {
	const pages = 3
	var base string
	graph := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := 0
		if p := r.URL.Query().Get("page"); p != "" {
			fmt.Sscanf(p, "%d", &page)
		}
		var value []map[string]any
		for _, name := range pagedNames(page) {
			value = append(value, map[string]any{"name": name, "size": 10, "createdDateTime": "2026-09-01T00:00:00Z", "file": map[string]any{}})
		}
		out := map[string]any{"value": value}
		if page+1 < pages {
			out["@odata.nextLink"] = fmt.Sprintf("%s/v1.0/me/drive/special/approot/children?page=%d", base, page+1)
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
	defer graph.Close()
	base = graph.URL

	svc, s := newTestService(t)
	setCloudConfig(t, s, func(c *store.CloudConfig) {
		c.Enabled = true
		c.Provider = "onedrive"
		c.AccessToken = "at"
		c.ExpiryTimeMs = time.Now().UnixMilli() + 3600_000
	})
	svc.Endpoints.Graph = graph.URL

	files, err := svc.List()
	if err != nil {
		t.Fatal(err)
	}
	assertAllPages(t, files, pages)
}

// A provider that keeps handing back the same page must end in an error, not
// a loop and not a listing that looks complete.
func TestDriveListThatNeverEndsIsAnError(t *testing.T) {
	drive := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"files":         []map[string]any{{"id": "x", "name": "game__main__snap_1.zip", "size": "1"}},
			"nextPageToken": "same-token-forever",
		})
	}))
	defer drive.Close()

	svc, s := newTestService(t)
	setCloudConfig(t, s, func(c *store.CloudConfig) {
		c.Enabled = true
		c.Provider = "google_drive"
		c.AccessToken = "at"
		c.ExpiryTimeMs = time.Now().UnixMilli() + 3600_000
		c.FolderID = "folder1"
	})
	svc.Endpoints.GoogleAPI = drive.URL

	if files, err := svc.List(); err == nil {
		t.Fatalf("a listing that never ended came back as %d files and no error", len(files))
	}
}
