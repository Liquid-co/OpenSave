package api

import (
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/opensave/opensave/internal/presets"
)

// folderEntry is one folder the navigator can open.
type folderEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// folderListing is one folder's subfolders. Parent is "" at a drive's root.
type folderListing struct {
	Path    string        `json:"path"`
	Parent  string        `json:"parent"`
	Folders []folderEntry `json:"folders"`
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
// With no path it answers with the places to start from.
//
// Folders only, never files, and names only, never contents. It sits behind
// localhostOnly with every other dashboard route, and that surface already
// takes any path to track.
func (s *Server) handleListFolders(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimSpace(r.URL.Query().Get("path"))
	if p == "" {
		writeJSON(w, http.StatusOK, startingPlaces())
		return
	}
	if !filepath.IsAbs(p) {
		writeError(w, http.StatusBadRequest, "path must be absolute")
		return
	}
	p = filepath.Clean(p)

	type result struct {
		list folderListing
		err  error
	}
	done := make(chan result, 1)
	go func() {
		list, err := listFolders(p)
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

func listFolders(p string) (folderListing, error) {
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
		if !isDir {
			continue
		}
		if len(out.Folders) == maxListedFolders {
			out.More = true
			break
		}
		out.Folders = append(out.Folders, folderEntry{Name: e.Name(), Path: full})
	}
	sort.Slice(out.Folders, func(i, j int) bool {
		return strings.ToLower(out.Folders[i].Name) < strings.ToLower(out.Folders[j].Name)
	})
	return out, nil
}

func startingPlaces() folderPlaces {
	out := folderPlaces{Places: []folderEntry{}, Drives: []folderEntry{}}
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
	for _, root := range presets.FixedDriveRoots() {
		out.Drives = append(out.Drives, folderEntry{Name: strings.TrimRight(root, `\/`), Path: root})
	}
	if len(out.Drives) == 0 && filepath.Separator == '/' {
		out.Drives = append(out.Drives, folderEntry{Name: "/", Path: "/"})
	}
	return out
}
