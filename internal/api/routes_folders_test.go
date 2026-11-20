package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func getFolders(t *testing.T, path string) (*httptest.ResponseRecorder, folderListing) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/folders?path="+url.QueryEscape(path), nil)
	rec := httptest.NewRecorder()
	(&Server{}).handleListFolders(rec, req)
	var out folderListing
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

func TestListFolders_FoldersOnlySortedWithTheirParent(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"saves", "Config", "app data"} {
		if err := os.Mkdir(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "profile.sav"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	rec, out := getFolders(t, root)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var names []string
	for _, f := range out.Folders {
		names = append(names, f.Name)
		if f.Path != filepath.Join(root, f.Name) {
			t.Errorf("%s has path %q", f.Name, f.Path)
		}
	}
	if want := []string{"app data", "Config", "saves"}; len(names) != 3 || names[0] != want[0] || names[1] != want[1] || names[2] != want[2] {
		t.Fatalf("folders %v, want %v (no files, case-insensitive order)", names, want)
	}
	if out.Parent != filepath.Dir(root) {
		t.Errorf("parent %q, want %q", out.Parent, filepath.Dir(root))
	}
}

func TestListFolders_RefusesARelativePathAndSaysWhenMissing(t *testing.T) {
	if rec, _ := getFolders(t, filepath.Join("some", "relative")); rec.Code != http.StatusBadRequest {
		t.Errorf("relative path: status %d, want 400", rec.Code)
	}
	if rec, _ := getFolders(t, filepath.Join(t.TempDir(), "gone")); rec.Code != http.StatusNotFound {
		t.Errorf("missing folder: status %d, want 404", rec.Code)
	}
}

func TestListFolders_StartsFromHomeAndTheDrives(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/folders", nil)
	rec := httptest.NewRecorder()
	(&Server{}).handleListFolders(rec, req)
	var out folderPlaces
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	if len(out.Places) == 0 || out.Places[0].Name != "home" || out.Places[0].Path != home {
		t.Errorf("places %+v, want home first", out.Places)
	}
	if len(out.Drives) == 0 {
		t.Error("no drives to start from")
	}
}

func TestListFolders_IsOnlyForThisComputer(t *testing.T) {
	s := &Server{}
	h := localhostOnly(http.HandlerFunc(s.handleListFolders))
	req := httptest.NewRequest(http.MethodGet, "/api/folders", nil)
	req.RemoteAddr = "192.168.1.66:50000"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatal("a device on the network was shown this computer's folders")
	}
}
