package cloud

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/opensave/opensave/internal/store"
)

// Deleting a cloud snapshot is what retention pruning and the cloud screen's
// Delete button do, and no test sent one to any remote provider: each
// provider's delete was reached only by hand. These pin down the one request
// each makes, and what a refusal from the server comes back as.

type seenRequest struct {
	Method, Path, Auth, Custom, Body string
}

// recorder answers every request with status and remembers the last one.
func recorder(t *testing.T, status int) (*httptest.Server, *seenRequest) {
	t.Helper()
	seen := &seenRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*seen = seenRequest{
			Method: r.Method,
			Path:   r.URL.EscapedPath(),
			Auth:   r.Header.Get("Authorization"),
			Custom: r.Header.Get("X-Custom"),
			Body:   string(body),
		}
		w.WriteHeader(status)
		if status >= 400 {
			_, _ = io.WriteString(w, `{"error":"refused"}`)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, seen
}

func signedIn(provider string) func(*store.CloudConfig) {
	return func(c *store.CloudConfig) {
		c.Enabled = true
		c.Provider = provider
		c.AccessToken = "at-" + provider
		c.ExpiryTimeMs = time.Now().UnixMilli() + 3600_000
	}
}

func TestDeleteSendsEachProvidersRequest(t *testing.T) {
	const name = "game__main__snap 1.zip" // a space, to see it escaped

	t.Run("google drive deletes by file id", func(t *testing.T) {
		srv, seen := recorder(t, http.StatusNoContent)
		svc, s := newTestService(t)
		setCloudConfig(t, s, signedIn("google_drive"))
		svc.Endpoints.GoogleAPI = srv.URL
		if err := svc.Delete(CloudFile{ID: "drive-id-7", Name: name}); err != nil {
			t.Fatal(err)
		}
		want := seenRequest{Method: http.MethodDelete, Path: "/drive/v3/files/drive-id-7", Auth: "Bearer at-google_drive"}
		if *seen != want {
			t.Errorf("request = %+v, want %+v", *seen, want)
		}
	})

	t.Run("google drive refuses without an id, since names are not unique there", func(t *testing.T) {
		srv, seen := recorder(t, http.StatusNoContent)
		svc, s := newTestService(t)
		setCloudConfig(t, s, signedIn("google_drive"))
		svc.Endpoints.GoogleAPI = srv.URL
		err := svc.Delete(CloudFile{Name: name})
		if err == nil || !strings.Contains(err.Error(), "missing Drive file id") {
			t.Fatalf("err = %v, want a missing-id refusal", err)
		}
		if seen.Method != "" {
			t.Errorf("a request was sent anyway: %+v", *seen)
		}
	})

	t.Run("dropbox", func(t *testing.T) {
		srv, seen := recorder(t, http.StatusOK)
		svc, s := newTestService(t)
		setCloudConfig(t, s, signedIn("dropbox"))
		svc.Endpoints.DropboxAPI = srv.URL
		if err := svc.Delete(CloudFile{Name: name}); err != nil {
			t.Fatal(err)
		}
		var body map[string]string
		_ = json.Unmarshal([]byte(seen.Body), &body)
		if seen.Method != http.MethodPost || seen.Path != "/2/files/delete_v2" || seen.Auth != "Bearer at-dropbox" ||
			body["path"] != "/OpenSave/"+name {
			t.Errorf("request = %+v", *seen)
		}
	})

	t.Run("onedrive", func(t *testing.T) {
		srv, seen := recorder(t, http.StatusNoContent)
		svc, s := newTestService(t)
		setCloudConfig(t, s, signedIn("onedrive"))
		svc.Endpoints.Graph = srv.URL
		if err := svc.Delete(CloudFile{Name: name}); err != nil {
			t.Fatal(err)
		}
		want := seenRequest{
			Method: http.MethodDelete,
			Path:   "/v1.0/me/drive/special/approot:/game__main__snap%201.zip",
			Auth:   "Bearer at-onedrive",
		}
		if *seen != want {
			t.Errorf("request = %+v, want %+v", *seen, want)
		}
	})

	t.Run("webdav, with its credentials and custom headers", func(t *testing.T) {
		srv, seen := recorder(t, http.StatusNoContent)
		svc, s := newTestService(t)
		setCloudConfig(t, s, func(c *store.CloudConfig) {
			c.Enabled = true
			c.Provider = "webdav"
			c.URL = srv.URL + "/dav"
			c.Username, c.Password = "user", "pass"
			c.HeadersJSON = `{"X-Custom":"h"}`
		})
		if err := svc.Delete(CloudFile{Name: name}); err != nil {
			t.Fatal(err)
		}
		want := seenRequest{
			Method: http.MethodDelete,
			Path:   "/dav/game__main__snap%201.zip",
			Auth:   "Basic " + base64.StdEncoding.EncodeToString([]byte("user:pass")),
			Custom: "h",
		}
		if *seen != want {
			t.Errorf("request = %+v, want %+v", *seen, want)
		}
	})
}

// A delete the server refuses must come back as an error naming the
// provider: retention pruning logs it and moves on, and a refusal reported
// as success would leave files the person thinks are gone.
func TestDeleteReportsARefusal(t *testing.T) {
	cases := []struct {
		provider, prefix string
		point            func(svc *Service, url string)
	}{
		{"google_drive", "Google Drive: ", func(svc *Service, u string) { svc.Endpoints.GoogleAPI = u }},
		{"dropbox", "Dropbox: ", func(svc *Service, u string) { svc.Endpoints.DropboxAPI = u }},
		{"onedrive", "OneDrive: ", func(svc *Service, u string) { svc.Endpoints.Graph = u }},
	}
	for _, c := range cases {
		t.Run(c.provider, func(t *testing.T) {
			srv, _ := recorder(t, http.StatusInternalServerError)
			svc, s := newTestService(t)
			setCloudConfig(t, s, signedIn(c.provider))
			c.point(svc, srv.URL)
			err := svc.Delete(CloudFile{ID: "id", Name: "a.zip"})
			if err == nil || !strings.HasPrefix(err.Error(), c.prefix) || !strings.Contains(err.Error(), "HTTP 500") {
				t.Errorf("err = %v, want it prefixed %q and carrying the status", err, c.prefix)
			}
		})
	}

	t.Run("webdav", func(t *testing.T) {
		srv, _ := recorder(t, http.StatusForbidden)
		svc, s := newTestService(t)
		setCloudConfig(t, s, func(c *store.CloudConfig) {
			c.Enabled, c.Provider, c.URL = true, "webdav", srv.URL
		})
		if err := svc.Delete(CloudFile{Name: "a.zip"}); err == nil || !strings.Contains(err.Error(), "HTTP 403") {
			t.Errorf("err = %v, want the 403", err)
		}
	})
}

// Drive's one refusal with a known cause gets the fix spelled out.
func TestDriveMissingPermissionSaysHowToFixIt(t *testing.T) {
	err := googleDriveErr(errors.New(`HTTP 403 - {"error":{"message":"Request had insufficient authentication scopes."}}`))
	if !strings.Contains(err.Error(), "TICK THE CHECKBOX") {
		t.Errorf("err = %v", err)
	}
	if got := googleDriveErr(errors.New("HTTP 500 - boom")).Error(); got != "Google Drive: HTTP 500 - boom" {
		t.Errorf("other errors = %q", got)
	}
}

func TestWebhookOnlySends(t *testing.T) {
	svc, s := newTestService(t)
	setCloudConfig(t, s, func(c *store.CloudConfig) {
		c.Enabled, c.Provider, c.URL = true, "webhook", "http://127.0.0.1:1/hook"
	})
	files, err := svc.List()
	if err != nil || len(files) != 0 {
		t.Errorf("List = %v, %v; want empty", files, err)
	}
	if err := svc.Download("a.zip", t.TempDir()+"/a.zip"); err == nil || err.Error() != "downloading is not supported for provider: webhook" {
		t.Errorf("Download err = %v", err)
	}
	if err := svc.Delete(CloudFile{Name: "a.zip"}); err == nil || err.Error() != "deletion is not supported for provider: webhook" {
		t.Errorf("Delete err = %v", err)
	}
}

func TestAnUnknownProviderIsRefusedPlainly(t *testing.T) {
	svc, s := newTestService(t)
	setCloudConfig(t, s, func(c *store.CloudConfig) {
		c.Enabled, c.Provider = true, "carrier-pigeon"
	})
	if err := svc.Upload(writeTempZip(t, "x"), "a.zip"); err == nil || err.Error() != "unsupported cloud sync provider: carrier-pigeon" {
		t.Errorf("Upload err = %v", err)
	}
	if files, err := svc.List(); err != nil || len(files) != 0 {
		t.Errorf("List = %v, %v; want empty", files, err)
	}
	if err := svc.Download("a.zip", t.TempDir()+"/a.zip"); err == nil || err.Error() != "downloading is not supported for provider: carrier-pigeon" {
		t.Errorf("Download err = %v", err)
	}
	if err := svc.Delete(CloudFile{Name: "a.zip"}); err == nil || err.Error() != "deletion is not supported for provider: carrier-pigeon" {
		t.Errorf("Delete err = %v", err)
	}
}
