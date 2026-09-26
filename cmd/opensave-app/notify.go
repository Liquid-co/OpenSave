package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Desktop notifications: what the in-app messages say, shown by the system
// when OpenSave is not in front — hidden in the tray, minimised, or behind
// another window. The UI decides when, from the same preferences as its own
// messages (lib/notify.js); this shows them, and a click brings OpenSave up
// where the notification is about.
//
// Windows gets a toast from "OpenSave", with its icon, the game's cover as
// the picture across the top, the game's name and what happened
// (notify_windows.go). Elsewhere the system's own notifications carry the
// title and the text (notify_other.go).

// DesktopNote is one notification, as the UI sends it.
type DesktopNote struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	// Image is a cover on the local daemon, shown with the notification
	// where the system can; "" for none.
	Image string `json:"image"`
	// Open is where clicking it goes, as the UI's router has it: JSON
	// {"view": …, "params": {…}}.
	Open string `json:"open"`
}

// DesktopNotify shows a notification on the desktop. Returns "" once shown,
// or why it could not be.
func (a *App) DesktopNotify(n DesktopNote) string {
	if a.ctx == nil || a.addr == "" {
		return "not started"
	}
	if err := a.showNote(n); err != nil {
		return err.Error()
	}
	return ""
}

// openFromNotification brings the window up at what a clicked notification
// was about. open is the note's Open, encoded or not.
func (a *App) openFromNotification(open string) {
	a.ShowWindow()
	if target := decodeOpen(open); target != "" && a.ctx != nil {
		runtime.EventsEmit(a.ctx, "notification-open", target)
	}
}

// encodeOpen makes a note's target safe to carry through the system, which
// on Windows is an XML attribute nothing escapes; decodeOpen reverses it,
// and passes anything else through.
func encodeOpen(open string) string {
	if open == "" {
		return ""
	}
	return "open:" + base64.RawURLEncoding.EncodeToString([]byte(open))
}

func decodeOpen(s string) string {
	rest, ok := strings.CutPrefix(s, "open:")
	if !ok {
		return ""
	}
	raw, err := base64.RawURLEncoding.DecodeString(rest)
	if err != nil {
		return ""
	}
	return string(raw)
}

// plainText keeps a title or a body from closing the CDATA section Windows'
// template puts it in.
func plainText(s string) string {
	return strings.ReplaceAll(s, "]]>", "]] >")
}

var noteImageClient = &http.Client{Timeout: 4 * time.Second}

// noteImage fetches a cover from the daemon into a file the system can
// show, reusing one fetched lately. Only the daemon's own covers: the URL
// comes from the page.
func (a *App) noteImage(url string) string {
	if url == "" || !strings.HasPrefix(url, "http://"+a.addr+"/api/cover?") {
		return ""
	}
	sum := sha256.Sum256([]byte(url))
	dir := filepath.Join(os.TempDir(), "OpenSave", "notifications")
	path := filepath.Join(dir, hex.EncodeToString(sum[:8])+".jpg")
	if info, err := os.Stat(path); err == nil && time.Since(info.ModTime()) < 24*time.Hour {
		return path
	}
	resp, err := noteImageClient.Get(url)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil || len(data) == 0 {
		return ""
	}
	if os.MkdirAll(dir, 0o755) != nil || os.WriteFile(path, data, 0o644) != nil {
		return ""
	}
	return path
}
