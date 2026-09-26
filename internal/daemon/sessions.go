package daemon

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/opensave/opensave/internal/sessions"
	"github.com/opensave/opensave/internal/store"
)

// Play sessions: which tracked game is being played, and what happens when it
// stops. See package sessions for how a game is recognised.
//
// When a session ends, the save as it was left gets a snapshot of its own —
// "After playing (1 h 12 min)" — and is synced to the other devices, so it is
// there when you sit down at one of them. Nothing is taken when the save did
// not change while the game ran.

const (
	// sessionPoll is how often running programs are looked at.
	sessionPoll = 10 * time.Second
	// sessionGrace is how long a game may be gone before its session ends.
	sessionGrace = 25 * time.Second
	// shortestSession is the least that counts as having played: less is a
	// launcher opening and closing, or a crash at start.
	shortestSession = 30 * time.Second
	// installDirsFresh is how long what was read about install folders is
	// trusted before it is read again.
	installDirsFresh = 10 * time.Minute
)

// sessionState is the daemon's side of play sessions.
type sessionState struct {
	tracker *sessions.Tracker
	// list finds running programs; a test gives its own.
	list func() ([]sessions.Proc, error)

	mu sync.Mutex
	// startHash is each playing game's save as it was when play began, to
	// tell afterwards whether the session changed it.
	startHash map[string]string
	installs  struct {
		at       time.Time
		byAppID  map[string]string
		byFolder map[string]string
	}
}

func (d *Daemon) initSessions() {
	d.sessions.list = sessions.List
	d.sessions.startHash = map[string]string{}
	d.sessions.tracker = &sessions.Tracker{
		Grace:   sessionGrace,
		OnStart: d.sessionStarted,
		OnEnd:   d.sessionEnded,
	}
}

// SessionTracker is the tracker, for the API and the CLI's `wrap`.
func (d *Daemon) SessionTracker() *sessions.Tracker { return d.sessions.tracker }

// PlayingSince says when a game's current session began, or zero.
func (d *Daemon) PlayingSince(gameID string) time.Time {
	if d.sessions.tracker == nil {
		return time.Time{}
	}
	return d.sessions.tracker.Playing()[gameID]
}

// PollSessions looks at what is running once.
func (d *Daemon) PollSessions() {
	targets, err := d.sessionTargets()
	if err != nil || len(targets) == 0 {
		d.sessions.tracker.Poll(nil, time.Now())
		return
	}
	procs, err := d.sessions.list()
	if err != nil {
		return
	}
	d.sessions.tracker.Poll(sessions.Running(procs, targets), time.Now())
}

func (d *Daemon) runSessions(ctx context.Context) {
	ticker := time.NewTicker(sessionPoll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.PollSessions()
		}
	}
}

// launcherPrograms are launchers, not games: a game whose launch program is
// one of these is not recognised by it, or every time the launcher ran would
// count as playing.
var launcherPrograms = map[string]bool{
	"steam.exe": true, "steam": true, "epicgameslauncher.exe": true, "galaxyclient.exe": true,
	"heroic.exe": true, "heroic": true, "lutris": true, "playnite.desktopapp.exe": true,
	"playnite.fullscreenapp.exe": true, "eadesktop.exe": true, "ubisoftconnect.exe": true,
	"battle.net.exe": true, "xboxpcapp.exe": true,
}

// programName is a launch program's file name, lower-cased, whichever
// separator its path was written with: a path from Windows names Steam as
// steam.exe on any system.
func programName(exe string) string {
	exe = strings.ReplaceAll(exe, `\`, "/")
	return strings.ToLower(exe[strings.LastIndex(exe, "/")+1:])
}

func (d *Daemon) sessionTargets() ([]sessions.Target, error) {
	games, err := d.Store.ListGames()
	if err != nil {
		return nil, err
	}
	byAppID, byFolder := d.installDirs()
	targets := make([]sessions.Target, 0, len(games))
	for _, g := range games {
		t := sessions.Target{GameID: g.ID, AppID: g.AppID}
		if exe := strings.TrimSpace(g.ExePath); exe != "" && !launcherPrograms[programName(exe)] {
			t.Exe = exe
			if dir := programFolder(exe); dir != "" {
				t.Dirs = append(t.Dirs, dir)
			}
		}
		if dir := byAppID[g.AppID]; g.AppID != "" && dir != "" {
			t.Dirs = append(t.Dirs, dir)
		} else if dir := byFolder[strings.ToLower(g.Name)]; dir != "" && specificEnough(dir) {
			t.Dirs = append(t.Dirs, dir)
		} else if dir := byFolder[folderish(g.Name)]; dir != "" && specificEnough(dir) {
			t.Dirs = append(t.Dirs, dir)
		}
		targets = append(targets, t)
	}
	return targets, nil
}

// programFolder is the folder a game's launch program is in, whose programs
// all count as the game running.
//
// The program chosen is often not the one that stays running. Many games
// start through a small program of their own that hands over and exits —
// Elden Ring's start_protected_game.exe starts eldenring.exe beside it — so
// matching the chosen program alone saw the game for a few seconds, or not
// at all. Whichever of them was picked, the game runs from that folder.
//
// Not for a shortcut, which is kept anywhere and points somewhere else, and
// not for a folder near a drive root, which holds more than one game.
func programFolder(exe string) string {
	switch strings.ToLower(filepath.Ext(exe)) {
	case ".lnk", ".url":
		return ""
	}
	dir := filepath.Dir(exe)
	if !specificEnough(dir) {
		return ""
	}
	return dir
}

// folderish is a game name as a folder is usually named: "Hollow Knight: Silksong"
// is in "hollow knight silksong".
func folderish(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' {
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// specificEnough keeps a folder from claiming too much: a drive root, or a
// folder right under one, holds more than one game.
func specificEnough(dir string) bool {
	clean := filepath.Clean(dir)
	parts := strings.FieldsFunc(clean, func(r rune) bool { return r == '/' || r == '\\' })
	return len(parts) >= 3
}

func (d *Daemon) installDirs() (map[string]string, map[string]string) {
	d.sessions.mu.Lock()
	defer d.sessions.mu.Unlock()
	if d.Scanner == nil {
		return nil, nil
	}
	if time.Since(d.sessions.installs.at) > installDirsFresh || d.sessions.installs.byAppID == nil {
		d.sessions.installs.byAppID, d.sessions.installs.byFolder = d.Scanner.InstallDirs()
		d.sessions.installs.at = time.Now()
	}
	return d.sessions.installs.byAppID, d.sessions.installs.byFolder
}

func (d *Daemon) sessionStarted(gameID string, at time.Time) {
	game, err := d.Store.GetGame(gameID)
	if err != nil {
		return
	}
	hash, _ := d.currentContentHash(game)
	d.sessions.mu.Lock()
	d.sessions.startHash[gameID] = hash
	d.sessions.mu.Unlock()
	d.Log.Log("info", fmt.Sprintf("playing %q", game.Name))
	if d.OnGameChanged != nil {
		d.OnGameChanged(gameID)
	}
}

func (d *Daemon) sessionEnded(gameID string, started, ended time.Time) {
	d.sessions.mu.Lock()
	startHash := d.sessions.startHash[gameID]
	delete(d.sessions.startHash, gameID)
	d.sessions.mu.Unlock()

	game, err := d.Store.GetGame(gameID)
	if err != nil {
		return
	}
	defer func() {
		if d.OnGameChanged != nil {
			d.OnGameChanged(gameID)
		}
	}()
	length := ended.Sub(started)
	if length < shortestSession {
		return
	}
	if err := d.Store.AddPlaySession(gameID, started.UnixMilli(), ended.UnixMilli()); err != nil {
		d.Log.Log("warn", err.Error())
	}
	d.Log.Log("info", fmt.Sprintf("stopped playing %q after %s", game.Name, spokenLength(length)))

	hash, err := d.currentContentHash(game)
	if err != nil || hash == startHash {
		return // the save did not change, or cannot be read: nothing to keep
	}
	comment := fmt.Sprintf("After playing (%s)", spokenLength(length))
	if err := d.markSessionEnd(game, hash, started, comment); err != nil {
		d.Log.Log("warn", fmt.Sprintf("could not keep %q as it was left: %v", game.Name, err))
		return
	}
	// Onward to the other devices, so it is there on the next one picked up.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		_, _ = d.P2P.SyncGame(ctx, gameID)
	}()
}

// markSessionEnd keeps the save as a session left it. The watcher has usually
// snapshotted it already, moments before the game closed; that snapshot is
// then named for the session rather than a copy of it taken. Otherwise one is
// taken.
func (d *Daemon) markSessionEnd(game store.Game, hash string, started time.Time, comment string) error {
	if game.LastManifestHash == hash {
		snaps, err := d.Store.ListSnapshots(game.ID, game.ActiveBranch)
		if err == nil && len(snaps) > 0 {
			latest := snaps[0] // newest first
			if at, perr := time.Parse(time.RFC3339Nano, latest.Timestamp); perr == nil && !at.Before(started) && latest.IsSystemAuto {
				return d.Store.SetSnapshotComment(latest.ID, comment)
			}
		}
	}
	if _, err := d.Snapshots.Create(game.ID, comment, true); err != nil {
		return err
	}
	return d.Store.SetLastManifestHash(game.ID, hash)
}

// spokenLength says how long a session was: "45 min", "1 h 12 min", "3 h".
func spokenLength(dur time.Duration) string {
	dur = dur.Round(time.Minute)
	h, m := int(dur/time.Hour), int(dur%time.Hour/time.Minute)
	switch {
	case h == 0:
		if m < 1 {
			m = 1
		}
		return fmt.Sprintf("%d min", m)
	case m == 0:
		return fmt.Sprintf("%d h", h)
	default:
		return fmt.Sprintf("%d h %d min", h, m)
	}
}

// endOpenSessions records the sessions still open when the daemon stops, as
// ending now: the game may go on, but nothing will be watching it.
func (d *Daemon) endOpenSessions() {
	if d.sessions.tracker == nil {
		return
	}
	now := time.Now()
	for id, since := range d.sessions.tracker.Playing() {
		if now.Sub(since) >= shortestSession {
			_ = d.Store.AddPlaySession(id, since.UnixMilli(), now.UnixMilli())
		}
	}
}
