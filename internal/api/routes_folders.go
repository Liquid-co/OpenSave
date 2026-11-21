package api

import (
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/opensave/opensave/internal/presets"
)

// folderEntry is one folder the navigator can open.
type folderEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	// Detail tells apart places with the same name: the drive of each Steam
	// library.
	Detail string `json:"detail,omitempty"`
}

// folderListing is one folder's subfolders, and its files of one type when
// asked for. Parent is "" at a drive's root.
type folderListing struct {
	Path    string        `json:"path"`
	Parent  string        `json:"parent"`
	Folders []folderEntry `json:"folders"`
	Files   []folderEntry `json:"files,omitempty"`
	// More is set when the folder holds more than maxListedFolders.
	More bool `json:"more,omitempty"`
}

// folderPlaces is where the navigator starts: the home folder, the folder
// most Windows saves are under, and each drive.
type folderPlaces struct {
	Places []folderEntry `json:"places"`
	Drives []folderEntry `json:"drives"`
}

const (
	maxListedFolders = 2000
	// A folder on a network drive that has gone away can block for a long
	// time; the navigator says so instead of hanging.
	listFoldersTimeout = 5 * time.Second
)

// handleListFolders lists the folders in one folder, for the folder navigator
// the browser build shows where the desktop app opens the system's picker.
// With no path it answers with the places to start from. With ext (".sscb")
// it lists the files of that one type too, for choosing a backup file; with
// programs=1, the programs a game can be started from (isProgram), and the
// places to start from are where games are installed.
//
// Names only, never contents, and files only of the type asked for. It sits behind
// localhostOnly with every other dashboard route, and that surface already
// takes any path to track.
func (s *Server) handleListFolders(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimSpace(r.URL.Query().Get("path"))
	programs := r.URL.Query().Get("programs") == "1"
	if p == "" {
		if programs {
			writeJSON(w, http.StatusOK, programPlaces())
		} else {
			writeJSON(w, http.StatusOK, startingPlaces())
		}
		return
	}
	if !filepath.IsAbs(p) {
		writeError(w, http.StatusBadRequest, "path must be absolute")
		return
	}
	p = filepath.Clean(p)
	ext := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("ext")))
	if ext != "" && (!strings.HasPrefix(ext, ".") || strings.ContainsAny(ext, `/\*?`)) {
		writeError(w, http.StatusBadRequest, "ext must look like .sscb")
		return
	}
	want := fileKind{ext: ext, programs: programs, goos: runtime.GOOS}

	type result struct {
		list folderListing
		err  error
	}
	done := make(chan result, 1)
	go func() {
		list, err := listFolders(p, want)
		done <- result{list, err}
	}()
	select {
	case res := <-done:
		switch {
		case errors.Is(res.err, fs.ErrNotExist):
			writeError(w, http.StatusNotFound, "That folder doesn't exist.")
		case errors.Is(res.err, fs.ErrPermission):
			writeError(w, http.StatusForbidden, "OpenSave isn't allowed to open that folder.")
		case res.err != nil:
			writeError(w, http.StatusBadRequest, res.err.Error())
		default:
			writeJSON(w, http.StatusOK, res.list)
		}
	case <-time.After(listFoldersTimeout):
		writeError(w, http.StatusGatewayTimeout, "That folder took too long to open.")
	case <-r.Context().Done():
	}
}

// fileKind is which files a listing shows beside its folders: none, those of
// one extension, or programs.
type fileKind struct {
	ext      string
	programs bool
	goos     string
}

// soName is a shared library on Linux (libfoo.so, libfoo.so.1.2): marked
// executable as often as not, and never what starts a game.
var soName = regexp.MustCompile(`\.so(\.\d+)*$`)

// isProgram says whether a file is one a game can be started from, as
// launchCommand (launch.go) starts it. On Windows that is by extension: a
// program, a batch file, or a shortcut, which is opened the way a double
// click would. Elsewhere programs carry no extension to know them by, so it
// is any file marked executable — but not a shared library.
func isProgram(name string, mode fs.FileMode, goos string) bool {
	lower := strings.ToLower(name)
	if goos == "windows" {
		switch filepath.Ext(lower) {
		case ".exe", ".bat", ".cmd", ".lnk", ".url":
			return true
		}
		return false
	}
	return mode.IsRegular() && mode.Perm()&0o111 != 0 && !soName.MatchString(lower) && !strings.HasSuffix(lower, ".dylib")
}

// isProgramFolder is a folder that is a program: a macOS app bundle, which
// is chosen whole, not opened.
func isProgramFolder(name, goos string) bool {
	return goos == "darwin" && strings.HasSuffix(strings.ToLower(name), ".app")
}

func (k fileKind) matches(e fs.DirEntry, full string) bool {
	switch {
	case k.programs:
		mode := e.Type()
		if k.goos != "windows" {
			// The permission bits are only in the full mode; and a link is
			// judged by what it points at.
			info, err := os.Stat(full)
			if err != nil {
				return false
			}
			mode = info.Mode()
		}
		return isProgram(e.Name(), mode, k.goos)
	case k.ext != "":
		return strings.HasSuffix(strings.ToLower(e.Name()), k.ext)
	}
	return false
}

func listFolders(p string, want fileKind) (folderListing, error) {
	entries, err := os.ReadDir(p)
	if err != nil {
		return folderListing{}, err
	}
	out := folderListing{Path: p, Folders: []folderEntry{}}
	if parent := filepath.Dir(p); parent != p {
		out.Parent = parent
	}
	for _, e := range entries {
		full := filepath.Join(p, e.Name())
		isDir := e.IsDir()
		if !isDir && e.Type()&fs.ModeSymlink != 0 {
			// A link or junction to a folder opens like one.
			if info, err := os.Stat(full); err == nil && info.IsDir() {
				isDir = true
			}
		}
		if isDir && want.programs && isProgramFolder(e.Name(), want.goos) {
			isDir = false
		} else if !isDir && !want.matches(e, full) {
			continue
		}
		if !isDir {
			if len(out.Files) < maxListedFolders {
				out.Files = append(out.Files, folderEntry{Name: e.Name(), Path: full})
			}
			continue
		}
		if len(out.Folders) == maxListedFolders {
			out.More = true
			break
		}
		out.Folders = append(out.Folders, folderEntry{Name: e.Name(), Path: full})
	}
	byName := func(list []folderEntry) {
		sort.Slice(list, func(i, j int) bool { return strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name) })
	}
	byName(out.Folders)
	byName(out.Files)
	return out, nil
}

// programPlaces is where choosing a game's program starts: home, where
// programs are installed, each Steam library's games, and each drive.
func programPlaces() folderPlaces {
	out := folderPlaces{Places: []folderEntry{}, Drives: drives()}
	add := func(name, p, detail string) {
		if p == "" {
			return
		}
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			out.Places = append(out.Places, folderEntry{Name: name, Path: p, Detail: detail})
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		add("home", home, "")
	}
	add("programfiles", os.Getenv("ProgramFiles"), "")
	add("programfilesx86", os.Getenv("ProgramFiles(x86)"), "")
	if runtime.GOOS == "darwin" {
		add("applications", "/Applications", "")
	}
	games := presets.SteamGameFolders()
	for _, dir := range games {
		detail := ""
		if len(games) > 1 {
			detail = strings.ToUpper(filepath.VolumeName(dir))
			if detail == "" {
				detail = dir
			}
		}
		add("steam", dir, detail)
	}
	return out
}

func drives() []folderEntry {
	out := []folderEntry{}
	for _, root := range presets.FixedDriveRoots() {
		out = append(out, folderEntry{Name: strings.TrimRight(root, `\/`), Path: root})
	}
	if len(out) == 0 && filepath.Separator == '/' {
		out = append(out, folderEntry{Name: "/", Path: "/"})
	}
	return out
}

func startingPlaces() folderPlaces {
	out := folderPlaces{Places: []folderEntry{}, Drives: drives()}
	add := func(name, p string) {
		if p == "" {
			return
		}
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			out.Places = append(out.Places, folderEntry{Name: name, Path: p})
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		add("home", home)
		add("appdata", os.Getenv("APPDATA"))
		add("localappdata", os.Getenv("LOCALAPPDATA"))
		add("savedgames", filepath.Join(home, "Saved Games"))
		add("documents", filepath.Join(home, "Documents"))
		add("share", filepath.Join(home, ".local", "share"))
	}
	return out
}
