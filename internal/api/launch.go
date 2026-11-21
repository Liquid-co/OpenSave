package api

import (
	"fmt"
	"net/http"
	"os"
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
		if _, err := os.Stat(exe); err != nil {
			writeJSON(w, http.StatusConflict, map[string]string{
				"error": fmt.Sprintf("the program set for %s is not there any more (%s) — set it again under Configuration",
					game.Name, exe),
				"reason": "program-missing",
			})
			return
		}
		if err := runExecutable(exe); err != nil {
			writeError(w, http.StatusInternalServerError, "could not launch executable: "+err.Error())
			return
		}
	case game.AppID != "":
		// Steam asked to run a game it does not have offers to install it,
		// which is not what the button promised — and for a copy kept
		// outside Steam it is a different copy altogether. Said here instead,
		// with the way to launch the one that is here. Only when there is a
		// Steam to have asked: without one there is no knowing.
		if installed, steamHere, dir := s.Daemon.SteamInstall(game); steamHere && !installed {
			msg := fmt.Sprintf("%s is not installed in Steam on this device — install it there, "+
				"or set the program it starts with under Configuration", game.Name)
			if dir != "" {
				msg = fmt.Sprintf("%s is not installed in Steam on this device, but it is in %s. "+
					"Play it once from there and Launch will start it, or set its program under Configuration",
					game.Name, dir)
			}
			writeJSON(w, http.StatusConflict, map[string]string{"error": msg, "reason": "not-installed"})
			return
		}
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

// handleInstallFolder answers with the folder a game is installed in on this
// device, or "" when that is not known: where choosing its program starts.
func (s *Server) handleInstallFolder(w http.ResponseWriter, r *http.Request) {
	game, err := s.Daemon.Store.GetGame(chi.URLParam(r, "gameId"))
	if err != nil {
		writeError(w, notFoundToStatus(err), err.Error())
		return
	}
	_, _, dir := s.Daemon.SteamInstall(game)
	if info, err := os.Stat(dir); dir == "" || err != nil || !info.IsDir() {
		dir = ""
	}
	writeJSON(w, http.StatusOK, map[string]string{"dir": dir})
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
