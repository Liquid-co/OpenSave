package api

import (
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/go-chi/chi/v5"
)

// handleLaunchGame starts a tracked game: the program set for it if there is
// one, otherwise through Steam by its App ID.
//
// The program comes first. Someone who set one chose how this game is
// started — a copy outside Steam, a mod launcher, a build Steam does not know
// about — and a Steam App ID is often only there for the cover art and the
// name, so launching through Steam instead started a different copy of the
// game, or asked to install one.
func (s *Server) handleLaunchGame(w http.ResponseWriter, r *http.Request) {
	gameID := chi.URLParam(r, "gameId")
	game, err := s.Daemon.Store.GetGame(gameID)
	if err != nil {
		writeError(w, notFoundToStatus(err), err.Error())
		return
	}

	switch exe := strings.TrimSpace(game.ExePath); {
	case exe != "":
		if err := runExecutable(exe); err != nil {
			writeError(w, http.StatusInternalServerError, "could not launch executable: "+err.Error())
			return
		}
	case game.AppID != "":
		if err := openURL("steam://run/" + game.AppID); err != nil {
			writeError(w, http.StatusInternalServerError, "could not launch via Steam: "+err.Error())
			return
		}
	default:
		writeError(w, http.StatusBadRequest, "no Steam App ID or executable configured for this game")
		return
	}

	s.Daemon.Log.Log("info", fmt.Sprintf("launched %q", game.Name))
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// How a launch is carried out; tests put their own in place, since the real
// ones start Steam or a game.
var (
	openURL       = openWithSystem
	runExecutable = startProgram
)

// openWithSystem opens a URL/protocol handler with the OS default handler.
func openWithSystem(url string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}

// launchCommand is how a game's program is started.
//
// In its own folder: games find their data relative to where they were
// started, and one started from wherever this process happens to be cannot
// find it and quits at once. And what cannot be run directly — a shortcut, a
// batch file, a macOS app bundle — is opened the way double-clicking it would.
func launchCommand(path string) *exec.Cmd {
	ext := strings.ToLower(filepath.Ext(path))
	switch {
	case runtime.GOOS == "windows" && ext != ".exe":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", path)
	case runtime.GOOS == "darwin" && ext == ".app":
		return exec.Command("open", path)
	}
	cmd := exec.Command(path)
	cmd.Dir = filepath.Dir(path)
	return cmd
}

// startProgram launches a program by path.
func startProgram(path string) error {
	cmd := launchCommand(path)
	if err := cmd.Start(); err != nil {
		return err
	}
	// Waited on so that, where it applies, the finished process does not
	// linger as a zombie for as long as this one runs.
	go func() { _ = cmd.Wait() }()
	return nil
}
