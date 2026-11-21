package api

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func getFolders(t *testing.T, path string, ext ...string) (*httptest.ResponseRecorder, folderListing) {
	t.Helper()
	q := "/api/folders?path=" + url.QueryEscape(path)
	if len(ext) > 0 {
		q += "&ext=" + url.QueryEscape(ext[0])
	}
	req := httptest.NewRequest(http.MethodGet, q, nil)
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

func TestListFolders_ListsFilesOfTheTypeAskedForOnly(t *testing.T) {
	root := t.TempDir()
	for _, f := range []string{"saves.SSCB", "older.sscb", "notes.txt", "saves.sscb.bak"} {
		if err := os.WriteFile(filepath.Join(root, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, out := getFolders(t, root); len(out.Files) != 0 {
		t.Fatalf("files listed without asking: %+v", out.Files)
	}
	rec, out := getFolders(t, root, ".sscb")
	if rec.Code != http.StatusOK || len(out.Files) != 2 || out.Files[0].Name != "older.sscb" || out.Files[1].Name != "saves.SSCB" {
		t.Fatalf("status %d, files %+v, want the two .sscb files by name", rec.Code, out.Files)
	}
	if rec, _ := getFolders(t, root, "*"); rec.Code != http.StatusBadRequest {
		t.Errorf("ext * answered %d, want 400", rec.Code)
	}
}

// A game's program is known as each system knows one: by extension on
// Windows — including what launching opens with a double click, a shortcut
// or a batch file — and by the executable bit elsewhere, where programs have
// no extension, leaving out shared libraries, which carry the bit as often as
// not. A macOS app bundle is a folder that is chosen, not opened.
func TestIsProgram_ByEachSystemsOwnRules(t *testing.T) {
	cases := []struct {
		name string
		mode fs.FileMode
		goos string
		want bool
	}{
		{"eldenring.exe", 0, "windows", true},
		{"Launch Game.LNK", 0, "windows", true},
		{"start.bat", 0, "windows", true},
		{"start.cmd", 0, "windows", true},
		{"Hades.url", 0, "windows", true},
		{"steam_api64.dll", 0, "windows", false},
		{"readme.txt", 0, "windows", false},
		{"Celeste", 0o755, "linux", true},
		{"start.sh", 0o744, "linux", true},
		{"start.sh", 0o644, "linux", false},
		{"libfmod.so", 0o755, "linux", false},
		{"libSDL2-2.0.so.0", 0o755, "linux", false},
		{"bin", fs.ModeDir | 0o755, "linux", false},
		{"Celeste", 0o755, "darwin", true},
		{"libsteam_api.dylib", 0o755, "darwin", false},
	}
	for _, c := range cases {
		if got := isProgram(c.name, c.mode, c.goos); got != c.want {
			t.Errorf("isProgram(%q, %v, %s) = %v, want %v", c.name, c.mode, c.goos, got, c.want)
		}
	}
	if !isProgramFolder("Celeste.app", "darwin") || isProgramFolder("Celeste.app", "linux") || isProgramFolder("Saves", "darwin") {
		t.Error("only a .app folder on macOS is a program")
	}
}

// Asked for programs, a folder lists this system's programs beside its
// folders, and nothing else.
func TestListFolders_ListsProgramsWhenAskedFor(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "Data"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name string, mode os.FileMode) {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x"), mode); err != nil {
			t.Fatal(err)
		}
	}
	var want []string
	if runtime.GOOS == "windows" {
		write("Game.exe", 0o644)
		write("Play.lnk", 0o644)
		write("steam_api64.dll", 0o644)
		want = []string{"Game.exe", "Play.lnk"}
	} else {
		write("Game", 0o755)
		write("libgame.so", 0o755)
		want = []string{"Game"}
	}
	write("readme.txt", 0o644)

	req := httptest.NewRequest(http.MethodGet, "/api/folders?programs=1&path="+url.QueryEscape(root), nil)
	rec := httptest.NewRecorder()
	(&Server{}).handleListFolders(rec, req)
	var out folderListing
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	var got []string
	for _, f := range out.Files {
		got = append(got, f.Name)
	}
	if rec.Code != http.StatusOK || strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("status %d, programs %v, want %v", rec.Code, got, want)
	}
	if len(out.Folders) != 1 || out.Folders[0].Name != "Data" {
		t.Errorf("folders %+v, want Data still there to open", out.Folders)
	}
	if _, plain := getFolders(t, root); len(plain.Files) != 0 {
		t.Errorf("programs listed without asking: %+v", plain.Files)
	}
}

// Choosing a program starts from home and where programs are installed, not
// from where saves are.
func TestListFolders_ProgramPlaces(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/folders?programs=1", nil)
	rec := httptest.NewRecorder()
	(&Server{}).handleListFolders(rec, req)
	var out folderPlaces
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	names := map[string]bool{}
	for _, p := range out.Places {
		names[p.Name] = true
	}
	if rec.Code != http.StatusOK || !names["home"] || names["savedgames"] || names["appdata"] || len(out.Drives) == 0 {
		t.Fatalf("status %d, places %+v, drives %+v", rec.Code, out.Places, out.Drives)
	}
	if runtime.GOOS == "windows" && os.Getenv("ProgramFiles") != "" && !names["programfiles"] {
		t.Errorf("no Program Files among %+v", out.Places)
	}
}

func makeFolder(t *testing.T, parent, name string) (*httptest.ResponseRecorder, folderEntry) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"parent": parent, "name": name})
	req := httptest.NewRequest(http.MethodPost, "/api/folders", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	(&Server{}).handleMakeFolder(rec, req)
	var out folderEntry
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

// New folder makes one folder where it is asked, and says where it is.
func TestMakeFolder_MakesOneFolderInsideAnother(t *testing.T) {
	root := t.TempDir()
	rec, out := makeFolder(t, root, "  Exports  ")
	want := filepath.Join(root, "Exports")
	if rec.Code != http.StatusOK || out.Path != want || out.Name != "Exports" {
		t.Fatalf("status %d, %+v; want %s", rec.Code, out, want)
	}
	if info, err := os.Stat(want); err != nil || !info.IsDir() {
		t.Fatalf("no folder at %s: %v", want, err)
	}
}

// Never over something already there, and never anywhere but one level
// down from a folder that exists.
func TestMakeFolder_RefusesWhatItShouldNot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "taken"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// "a" is there, so "a/b" would make a folder inside it if the name were
	// let through: only the check on the name stops it.
	if err := os.Mkdir(filepath.Join(root, "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		parent, name string
		code         int
	}{
		{root, "taken", http.StatusConflict},
		{root, "", http.StatusBadRequest},
		{root, "..", http.StatusBadRequest},
		{root, "a/b", http.StatusBadRequest},
		{root, `a\b`, http.StatusBadRequest},
		{filepath.Join("some", "relative"), "x", http.StatusBadRequest},
		{filepath.Join(root, "gone"), "x", http.StatusNotFound},
	} {
		if rec, _ := makeFolder(t, c.parent, c.name); rec.Code != c.code {
			t.Errorf("parent %q name %q: status %d, want %d (%s)", c.parent, c.name, rec.Code, c.code, rec.Body.String())
		}
	}
	if _, err := os.Stat(filepath.Join(root, "a", "b")); err == nil {
		t.Error(`"a/b" made a folder inside "a"`)
	}
}

// What a name may be is the system's rule: Windows has more of them.
func TestFolderName_ByEachSystemsRules(t *testing.T) {
	for _, c := range []struct {
		raw, goos, want string
		ok              bool
	}{
		{"Saves", "linux", "Saves", true},
		{"what?", "linux", "what?", true},
		{"what?", "windows", "", false},
		{"a:b", "windows", "", false},
		{"CON", "windows", "", false},
		{"nul.txt", "windows", "", false},
		{"console", "windows", "console", true},
		{"Backups. ", "windows", "Backups", true},
		{"...", "windows", "", false},
		{"a/b", "darwin", "", false},
	} {
		got, problem := folderName(c.raw, c.goos)
		if (problem == "") != c.ok || got != c.want {
			t.Errorf("folderName(%q, %s) = %q, %q; want %q ok=%v", c.raw, c.goos, got, problem, c.want, c.ok)
		}
	}
}

func TestMakeFolder_IsOnlyForThisComputer(t *testing.T) {
	h := localhostOnly(http.HandlerFunc((&Server{}).handleMakeFolder))
	req := httptest.NewRequest(http.MethodPost, "/api/folders", strings.NewReader(`{"parent":"/","name":"x"}`))
	req.RemoteAddr = "192.168.1.66:50000"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("a device on the network got %d", rec.Code)
	}
}
