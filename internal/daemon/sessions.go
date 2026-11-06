package daemon

import (
	"context"
	"fmt"
	"os"
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
	// cpu: each process's processor time at the last poll, and when it last
	// used any (activeProcesses).
	cpu map[int]procSeen
	// endedAt: when each game's last session ended (noteActivity).
	endedAt map[string]time.Time
	// saveChanged: when each playing game's save last changed; quiet: where
	// each one's quiet stretch began; quietEnded: games whose session was
	// ended as quiet, by the save as it was then, while their process is still
	// there (endQuietSessions).
	saveChanged map[string]time.Time
	quiet       map[string]quietWindow
	quietEnded  map[string]string
	// checkpoint is when each playing game last had a snapshot taken while
	// it was played (holdSnapshotWhilePlaying).
	checkpoint map[string]time.Time
	// programs: what programsIn found in each install folder.
	programs map[string]programList
	// active: stretches of changes no session accounts for (noteActivity);
	// warned: games already said to change with no program seen.
	active map[string]*activity
	warned map[string]bool
	// lastChange: when each game's save last changed here.
	lastChange map[string]time.Time
	// startHash is each playing game's save as it was when play began, to
	// tell afterwards whether the session changed it.
	startHash map[string]string
	// seen counts, for each game being played whose program is still to be
	// learned, the polls each program from its folder was running in.
	seen     map[string]map[string]int
	installs struct {
		at       time.Time
		byAppID  map[string]string
		byFolder map[string]string
		steam    bool // a Steam library was found at all
	}
}

func (d *Daemon) initSessions() {
	d.sessions.list = sessions.List
	d.sessions.startHash = map[string]string{}
	d.sessions.seen = map[string]map[string]int{}
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
func (d *Daemon) PollSessions() { d.pollSessionsAt(time.Now()) }

func (d *Daemon) pollSessionsAt(now time.Time) {
	targets, learn, err := d.sessionTargets()
	if err != nil || len(targets) == 0 {
		d.sessions.tracker.Poll(nil, now)
		return
	}
	procs, err := d.sessions.list()
	if err != nil {
		return
	}
	procs = d.activeProcessesAt(procs, now)
	running := sessions.Running(procs, targets)
	d.rememberInstallFolders(procs, targets, running)
	d.notePrograms(procs, targets, learn, running)
	d.endQuietSessions(running, sessions.CPUOf(procs, targets), now)
	d.sessions.tracker.Poll(running, now)
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

// sessionTargets is every tracked game to look for, and the ones whose
// program is to be learned from what runs in their folder (see learnProgram).
func (d *Daemon) sessionTargets() ([]sessions.Target, map[string]bool, error) {
	games, err := d.Store.ListGames()
	if err != nil {
		return nil, nil, err
	}
	byAppID, byFolder, _ := d.installDirs()
	targets := make([]sessions.Target, 0, len(games))
	learn := map[string]bool{}
	for _, g := range games {
		t := sessions.Target{GameID: g.ID, AppID: g.AppID}
		exe := strings.TrimSpace(g.ExePath)
		if exe != "" && !launcherPrograms[programName(exe)] {
			t.Exe = exe
			if dir := programFolder(exe); dir != "" {
				t.Dirs = append(t.Dirs, dir)
			}
		}
		dir, viaSteam := installFolder(g, byAppID, byFolder)
		if dir == "" {
			dir = d.knownInstallFolder(g.ID)
		}
		t.FolderNames = d.installFolderNames(g)
		if dir != "" {
			t.Dirs = append(t.Dirs, dir)
		}
		for _, dir := range t.Dirs {
			t.Programs = append(t.Programs, d.programsIn(dir)...)
		}
		// Steam launches the games it has installed; the rest need a
		// program to start them, and none was given.
		if dir != "" && !viaSteam && exe == "" {
			learn[g.ID] = true
		}
		targets = append(targets, t)
	}
	return targets, learn, nil
}

// installFolder is where a game is installed on this device, if that is
// known, and whether Steam installed it there.
func installFolder(g store.Game, byAppID, byFolder map[string]string) (dir string, viaSteam bool) {
	if dir := byAppID[g.AppID]; g.AppID != "" && dir != "" {
		return dir, true
	}
	if dir := byFolder[strings.ToLower(g.Name)]; dir != "" && specificEnough(dir) {
		return dir, false
	}
	if dir := byFolder[folderish(g.Name)]; dir != "" && specificEnough(dir) {
		return dir, false
	}
	return "", false
}

// Install states, as a game's entry reports them.
const (
	InstallFound    = "found"
	InstallNotFound = "not-found"
)

// InstallState says whether a game is installed on this device: found, not
// found, or "" when there is no telling.
//
// "Not found" is said only when it is certain enough to be worth saying: the
// game has a Steam App ID, Steam is here and does not have it, and it is not
// in any other folder games are kept in. A game with no App ID — an
// emulator's save, one installed somewhere of the user's own choosing — is
// not reported missing merely because this device cannot see where it is.
func (d *Daemon) InstallState(g store.Game) string {
	if exe := strings.TrimSpace(g.ExePath); exe != "" && !launcherPrograms[programName(exe)] {
		if _, err := os.Stat(exe); err == nil {
			return InstallFound
		}
	}
	byAppID, byFolder, steam := d.installDirs()
	if dir, _ := installFolder(g, byAppID, byFolder); dir != "" {
		return InstallFound
	}
	if d.knownInstallFolder(g.ID) != "" {
		return InstallFound
	}
	if g.AppID != "" && steam {
		return InstallNotFound
	}
	return ""
}

// SteamInstall reads afresh whether Steam on this device has a game
// installed, and whether there is a Steam here to ask at all. Fresh, not the
// session poll's copy: it answers a click, and a game installed a minute ago
// must launch. dir is where the game was found otherwise, if it was.
func (d *Daemon) SteamInstall(g store.Game) (installed, steamHere bool, dir string) {
	d.sessions.mu.Lock()
	d.sessions.installs.at = time.Time{} // read again below
	d.sessions.mu.Unlock()
	byAppID, byFolder, steam := d.installDirs()
	dir, viaSteam := installFolder(g, byAppID, byFolder)
	return viaSteam, steam, dir
}

// notePrograms counts, for the games whose program is being learned, which
// programs from their folders are running.
func (d *Daemon) notePrograms(procs []sessions.Proc, targets []sessions.Target, learn map[string]bool, running map[string]int) {
	d.sessions.mu.Lock()
	defer d.sessions.mu.Unlock()
	for _, t := range targets {
		if _, playing := running[t.GameID]; !playing || !learn[t.GameID] {
			continue
		}
		for _, p := range procs {
			if !sessions.InFolder(p, t) {
				continue
			}
			if d.sessions.seen[t.GameID] == nil {
				d.sessions.seen[t.GameID] = map[string]int{}
			}
			d.sessions.seen[t.GameID][p.Exe]++
		}
	}
}

// helperNames mark programs that run from a game's folder beside the game
// without being it: crash reporters, anti-cheat, installers.
var helperNames = []string{
	"crash", "reporter", "werfault", "unins", "redist", "dxsetup",
	"anticheat", "battleye", "beservice", "helper", "updater",
}

// learnProgram gives a game the program it was just played with, when it had
// none and Steam cannot start it: a copy kept in D:\Games, say. Without one,
// Launch had nothing to run, or went to Steam and asked to install the game.
//
// The program is the one that ran for most of the session. A game started
// through a small program of its own shows that one for a poll or two, and
// the game itself for the rest; a crash reporter runs as long as the game,
// but is named as one and skipped. Of two running equally long, the larger
// file is the game.
func (d *Daemon) learnProgram(gameID string, seen map[string]int) {
	best, bestN, bestSize := "", 0, int64(-1)
	for exe, n := range seen {
		name := programName(exe)
		helper := false
		for _, h := range helperNames {
			if strings.Contains(name, h) {
				helper = true
				break
			}
		}
		info, err := os.Stat(exe)
		if helper || err != nil {
			continue
		}
		size := info.Size()
		if n > bestN || (n == bestN && (size > bestSize || (size == bestSize && exe < best))) {
			best, bestN, bestSize = exe, n, size
		}
	}
	if best == "" {
		return
	}
	game, err := d.Store.GetGame(gameID)
	if err != nil || strings.TrimSpace(game.ExePath) != "" {
		return // one was set meanwhile, and that one is the user's
	}
	game.ExePath = best
	if err := d.Store.UpdateGame(game); err != nil {
		d.Log.Log("warn", fmt.Sprintf("could not remember how %q is started: %v", game.Name, err))
		return
	}
	d.Log.Log("info", fmt.Sprintf("%q runs from %s; Launch starts it from there now", game.Name, best))
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

func (d *Daemon) installDirs() (byAppID, byFolder map[string]string, steam bool) {
	d.sessions.mu.Lock()
	defer d.sessions.mu.Unlock()
	if d.Scanner == nil {
		return nil, nil, false
	}
	in := &d.sessions.installs
	if time.Since(in.at) > installDirsFresh || in.byAppID == nil {
		in.byAppID, in.byFolder = d.Scanner.InstallDirs()
		in.steam = d.Scanner.HasSteam()
		in.at = time.Now()
	}
	return in.byAppID, in.byFolder, in.steam
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
	// The session's end keeps the save and syncs it; a stretch of changes
	// it overlapped would otherwise hold the sync back for quietAfter more.
	d.endActivity(gameID)
	d.sessions.mu.Lock()
	startHash := d.sessions.startHash[gameID]
	delete(d.sessions.startHash, gameID)
	delete(d.sessions.checkpoint, gameID)
	delete(d.sessions.saveChanged, gameID)
	if d.sessions.endedAt == nil {
		d.sessions.endedAt = map[string]time.Time{}
	}
	d.sessions.endedAt[gameID] = ended
	seen := d.sessions.seen[gameID]
	delete(d.sessions.seen, gameID)
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
	d.learnProgram(gameID, seen)
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
	// Bound to the P2P engine's lifecycle, as the watcher's syncs are, so
	// shutdown cancels it.
	d.P2P.GoSync(func(ctx context.Context) {
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		_, _ = d.P2P.SyncGame(ctx, gameID)
	})
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
			// A snapshot's time is kept to the millisecond, the session's start
			// to the nanosecond: one taken in the millisecond the session began
			// read as older than it, was not named for it, and a second copy of
			// the same save was taken.
			if at, perr := time.Parse(time.RFC3339Nano, latest.Timestamp); perr == nil && !at.Before(started.Truncate(time.Millisecond)) && latest.IsSystemAuto {
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

// checkpointEvery is how often a game being played gets a snapshot of its
// save: one per autosave pushed the snapshots worth keeping out, and none
// until the session ends leaves nothing behind a crash.
const checkpointEvery = 30 * time.Minute

// holdSnapshotWhilePlaying tells the watcher not to snapshot a change to a
// game being played — in a session, or changing with no session seen
// (noteActivity) — unless checkpointEvery has passed since play began or
// since the last checkpoint. The end of the session, or of the stretch of
// changes, keeps the save as it was left.
func (d *Daemon) holdSnapshotWhilePlaying(gameID string) bool {
	// Every change made here counts, session seen or not (noteActivity).
	start, stretch := d.noteActivity(gameID)
	since := d.PlayingSince(gameID)
	if since.IsZero() {
		if !stretch {
			return false
		}
		since = start
	}
	d.sessions.mu.Lock()
	defer d.sessions.mu.Unlock()
	if d.sessions.checkpoint == nil {
		d.sessions.checkpoint = map[string]time.Time{}
	}
	now := time.Now()
	if d.sessions.saveChanged == nil {
		d.sessions.saveChanged = map[string]time.Time{}
	}
	d.sessions.saveChanged[gameID] = now // the game is saving: being played (endQuietSessions)
	if !checkpointDue(since, d.sessions.checkpoint[gameID], now) {
		return true
	}
	d.sessions.checkpoint[gameID] = now
	return false
}

// checkpointDue says whether checkpointEvery has passed since play began, or
// since the last checkpoint when there was one.
func checkpointDue(since, last, now time.Time) bool {
	if last.Before(since) {
		last = since
	}
	return now.Sub(last) >= checkpointEvery
}

// activeProcesses leaves out the processes doing nothing. A game leaves one
// behind when it closes or is killed — a crash handler, a stub of the same
// name — and it kept a session open for as long as it lingered: fifteen
// minutes after one game had closed. While a session is open the game's save
// is held back from syncing (syncengine/playing.go), so this is what lets it
// go once the game is gone.
func (d *Daemon) activeProcesses(procs []sessions.Proc) []sessions.Proc {
	return d.activeProcessesAt(procs, time.Now())
}

// procSeen is a process's processor time at the last poll, and when it was
// last seen using any.
type procSeen struct {
	cpu  time.Duration
	busy time.Time
}

// idleFor is how long a process may use no processor time at all before it
// is no longer the game, however much or little memory it holds. A game
// being played uses some all the time — a paused one still draws its menu —
// but one minimised in the background can throttle to almost nothing, or be
// suspended, and Windows trims its memory too. Ending its session then would
// sync the save under it, which is what the session is there to stop (asked
// in review on #117). Five minutes still lets go of what a closed game left
// behind well before the quarter of an hour one lingered.
const idleFor = 5 * time.Minute

// busyCPU is the processor time between two polls that counts as using any.
const busyCPU = 20 * time.Millisecond

func (d *Daemon) activeProcessesAt(procs []sessions.Proc, now time.Time) []sessions.Proc {
	d.sessions.mu.Lock()
	defer d.sessions.mu.Unlock()
	prev := d.sessions.cpu
	next := make(map[int]procSeen, len(procs))
	out := procs[:0:0]
	for _, p := range procs {
		before, seen := prev[p.PID]
		if !p.Measured {
			out = append(out, p) // cannot be told: counted
			continue
		}
		s := procSeen{cpu: p.CPU, busy: before.busy}
		if !seen || p.CPU-before.cpu >= busyCPU {
			s.busy = now // new, or working
		}
		next[p.PID] = s
		if now.Sub(s.busy) > idleFor {
			continue // doing nothing for long enough not to be the game
		}
		out = append(out, p)
	}
	d.sessions.cpu = next
	return out
}

// quietSessionLimit is how long a session may go without its save changing,
// and its game's processes all but idle, before it is taken as over.
//
// While a game is played its save is held back from syncing
// (syncengine/playing.go), so a session that never ended would hold the save
// for good. A process left behind that still ticks over now and then — a
// crash reporter, a launcher's helper — counts as running however long it
// lingers: activeProcesses only lets go of one doing nothing at all.
//
// A game being played may well not save for an hour — one saved by hand, at
// save points — but it keeps drawing, and uses far more processor time than
// quietCPUShare. What a game left behind uses less. Ended, the session is kept
// and synced as when the game closes.
//
// Never a session begun from outside (Tracker.Marked: the Deck plugin, `opensave
// wrap`): what began it says when it ends, and knows.
const quietSessionLimit = time.Hour

// quietCPUShare is the share of one processor core, at most, that a game's
// processes may have used over quietSessionLimit for its session to be taken
// as over: 1%, some 36 seconds in an hour.
const quietCPUShare = 100 // as a divisor

// quietWindow is where a game's quiet stretch began: the processor time its
// processes had used by then, and when.
type quietWindow struct {
	cpu time.Duration
	at  time.Time
}

// endQuietSessions ends the sessions whose save has not changed, and whose
// processes have all but idled, for quietSessionLimit; and keeps them from
// starting again at once. running is the games seen running this poll, and
// cpu their processes' processor time (sessions.CPUOf); a game ended this way
// is taken out of running while its save is still as it was then. Called
// before the tracker polls.
//
// Measured in windows of quietSessionLimit from the session's start or its
// save's last change: at the end of one, the session is over if the game was
// all but idle in it; otherwise a new window begins.
func (d *Daemon) endQuietSessions(running map[string]int, cpu map[string]sessions.GameCPU, now time.Time) {
	playing := d.sessions.tracker.Playing()
	lastHash := func(gameID string) string {
		if g, err := d.Store.GetGame(gameID); err == nil {
			return g.LastManifestHash
		}
		return ""
	}

	var end []string
	d.sessions.mu.Lock()
	if d.sessions.quietEnded == nil {
		d.sessions.quietEnded = map[string]string{}
	}
	if d.sessions.quiet == nil {
		d.sessions.quiet = map[string]quietWindow{}
	}
	for gameID, hash := range d.sessions.quietEnded {
		if _, still := running[gameID]; !still {
			delete(d.sessions.quietEnded, gameID) // gone: its next start is a session again
		} else if lastHash(gameID) != hash {
			delete(d.sessions.quietEnded, gameID) // saved since: being played after all
		} else {
			delete(running, gameID)
		}
	}
	for gameID := range d.sessions.quiet {
		if _, ok := playing[gameID]; !ok {
			delete(d.sessions.quiet, gameID)
		}
	}
	for gameID, since := range playing {
		g, seen := cpu[gameID]
		if d.sessions.tracker.Marked(gameID) || !seen || !g.Measured {
			delete(d.sessions.quiet, gameID) // begun from outside, or cannot be told
			continue
		}
		w, has := d.sessions.quiet[gameID]
		if !has {
			d.sessions.quiet[gameID] = quietWindow{cpu: g.CPU, at: since} // the first window: from the start
			continue
		}
		if d.sessions.saveChanged[gameID].After(w.at) || g.CPU < w.cpu {
			d.sessions.quiet[gameID] = quietWindow{cpu: g.CPU, at: now} // saved, or a new process: a new window
			continue
		}
		span := now.Sub(w.at)
		if span < quietSessionLimit {
			continue
		}
		if g.CPU-w.cpu >= span/quietCPUShare {
			d.sessions.quiet[gameID] = quietWindow{cpu: g.CPU, at: now} // busy: being played
			continue
		}
		delete(d.sessions.quiet, gameID)
		d.sessions.quietEnded[gameID] = lastHash(gameID)
		delete(running, gameID)
		end = append(end, gameID)
	}
	d.sessions.mu.Unlock()

	for _, gameID := range end {
		name := gameID
		if g, err := d.Store.GetGame(gameID); err == nil {
			name = g.Name
		}
		d.Log.Log("info", fmt.Sprintf("%q has neither saved nor done anything for %s while seen running: taken as no longer played, so its save is kept and synced",
			name, quietSessionLimit))
		d.sessions.tracker.Finish(gameID, now)
	}
}

// Programs that come with a game without being it: installers, crash
// reporters, redistributables, anti-cheat services. A process known only by
// name is never matched by one of these — every game ships the same few.
var notTheGame = []string{
	"unins", "setup", "install", "crash", "report", "redist", "vcredist", "vc_redist", "dxsetup",
	"directx", "prereq", "launcher", "easyanticheat", "eac", "battleye", "beservice", "update",
	"patch", "config", "settings", "helper", "service", "cef", "webhelper", "dotnet", "physx",
}

// programsIn lists the programs in a game's install folder by file name,
// lower-case, for a process known only by name (sessions.Proc.Name). Read at
// most every installDirsFresh, three folders deep.
func (d *Daemon) programsIn(dir string) []string {
	d.sessions.mu.Lock()
	if c, ok := d.sessions.programs[dir]; ok && time.Since(c.at) < installDirsFresh {
		d.sessions.mu.Unlock()
		return c.names
	}
	d.sessions.mu.Unlock()

	var names []string
	root := filepath.Clean(dir)
	depth := strings.Count(root, string(filepath.Separator))
	_ = filepath.WalkDir(root, func(p string, e os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if e.IsDir() {
			if strings.Count(filepath.Clean(p), string(filepath.Separator))-depth >= 3 {
				return filepath.SkipDir
			}
			return nil
		}
		name := strings.ToLower(e.Name())
		if !strings.HasSuffix(name, ".exe") || len(names) >= 64 {
			return nil
		}
		for _, w := range notTheGame {
			if strings.Contains(name, w) {
				return nil
			}
		}
		names = append(names, name)
		return nil
	})

	d.sessions.mu.Lock()
	if d.sessions.programs == nil {
		d.sessions.programs = map[string]programList{}
	}
	d.sessions.programs[dir] = programList{at: time.Now(), names: names}
	d.sessions.mu.Unlock()
	return names
}

type programList struct {
	at    time.Time
	names []string
}

// quietAfter is how long a game's save must stay unchanged before a stretch
// of changes no session accounts for is over: longer than the gap between
// two autosaves of the games measured (Crimson Desert: up to six minutes).
const quietAfter = 10 * time.Minute

// activity is a stretch of changes to a game's save while no session of it
// was seen: a game whose program cannot be told apart, or is not installed
// where this device looks. It is handled as a session — not synced either
// way, a checkpoint at most every checkpointEvery — and when the save has
// stayed unchanged for quietAfter, it is kept and synced as a session's end
// would be.
type activity struct {
	start, last time.Time
	timer       *time.Timer
	// changes counts the changes in it; a change within quietAfter of the
	// one before, even one already sent, counts as a second.
	changes int
}

// firstChangeWait is how long a change on its own is held before it is kept
// and synced: long enough that the next autosave of a game being played
// arrives first (Crimson Desert: 3.6 minutes between its first two), so a
// session not seen still sends one version, at its end. A game that saves
// once, as it closes, reaches the other devices this much later — when its
// session is not seen; the end of a session seen is sent at once.
const firstChangeWait = 5 * time.Minute

// quietFor is how long the save must stay unchanged to end the stretch.
func (a *activity) quietFor() time.Duration {
	if a.changes <= 1 {
		return firstChangeWait
	}
	return quietAfter
}

// noteActivity records a change to a game's save made here (not by a sync)
// and returns when the stretch it belongs to began, and whether it is held
// as one. Every change is: a change on its own is kept and synced once the
// save has stayed unchanged for firstChangeWait, a second within quietAfter
// of the one before makes it a stretch that ends after quietAfter unchanged.
// Only SyncEveryChange sends each change as it comes.
func (d *Daemon) noteActivity(gameID string) (time.Time, bool) {
	now := time.Now()
	d.sessions.mu.Lock()
	defer d.sessions.mu.Unlock()
	if d.sessions.active == nil {
		d.sessions.active = map[string]*activity{}
	}
	if d.sessions.lastChange == nil {
		d.sessions.lastChange = map[string]time.Time{}
	}
	prev, seen := d.sessions.lastChange[gameID]
	d.sessions.lastChange[gameID] = now
	if d.opts.SyncEveryChange {
		return now, false
	}
	// A game whose sessions are seen here, while none is open: a change is
	// not play, and is sent at once (recognised). So is one just after a
	// session's end: the game writing as it closes.
	if d.sessions.tracker.Playing()[gameID].IsZero() && d.sessions.active[gameID] == nil &&
		(now.Sub(d.sessions.endedAt[gameID]) < sessionEndGrace || d.recognisedLocked(gameID)) {
		return now, false
	}
	a := d.sessions.active[gameID]
	if a == nil {
		a = &activity{start: now, changes: 1}
		if seen && now.Sub(prev) < quietAfter {
			a.start, a.changes = prev, 2
		}
		d.sessions.active[gameID] = a
		a.timer = time.AfterFunc(a.quietFor(), func() { d.activityQuiet(gameID) })
	} else {
		a.changes++
		a.timer.Reset(a.quietFor())
	}
	a.last = now
	if a.changes == 2 && d.PlayingSince(gameID).IsZero() && !d.sessions.warned[gameID] {
		if d.sessions.warned == nil {
			d.sessions.warned = map[string]bool{}
		}
		d.sessions.warned[gameID] = true
		name := gameID
		if g, err := d.Store.GetGame(gameID); err == nil {
			name = g.Name
		}
		go d.Log.Log("info", fmt.Sprintf("%q is changing its save, but its program is not seen running here — "+
			"handled as being played: kept and synced once it has not changed for %s", name, spokenLength(quietAfter)))
	}
	return a.start, true
}

// changingNow reports a game in a stretch of changes (noteActivity).
func (d *Daemon) changingNow(gameID string) bool {
	d.sessions.mu.Lock()
	defer d.sessions.mu.Unlock()
	_, ok := d.sessions.active[gameID]
	return ok
}

// endActivity forgets a stretch of changes; a session's end takes over.
func (d *Daemon) endActivity(gameID string) {
	d.sessions.mu.Lock()
	if a := d.sessions.active[gameID]; a != nil {
		a.timer.Stop()
		delete(d.sessions.active, gameID)
	}
	d.sessions.mu.Unlock()
}

// activityQuiet ends a stretch of changes once the save has stayed unchanged
// for quietAfter: the save as it was left is kept and goes to the other
// devices, as at the end of a session.
func (d *Daemon) activityQuiet(gameID string) {
	d.sessions.mu.Lock()
	a := d.sessions.active[gameID]
	if a == nil {
		d.sessions.mu.Unlock()
		return
	}
	if wait := a.quietFor() - time.Since(a.last); wait > 0 {
		a.timer.Reset(wait)
		d.sessions.mu.Unlock()
		return
	}
	delete(d.sessions.active, gameID)
	delete(d.sessions.checkpoint, gameID)
	start, last, stretch := a.start, a.last, a.changes > 1
	d.sessions.mu.Unlock()

	if !d.PlayingSince(gameID).IsZero() {
		return // a session is open after all; its end keeps the save
	}
	game, err := d.Store.GetGame(gameID)
	if err != nil {
		return
	}
	if hash, err := d.currentContentHash(game); err == nil && hash != game.LastManifestHash {
		comment := "Auto backup"
		if last.Sub(start) >= shortestSession {
			comment = fmt.Sprintf("After playing (%s)", spokenLength(last.Sub(start)))
		}
		if err := d.markSessionEnd(game, hash, start, comment); err != nil {
			d.Log.Log("warn", fmt.Sprintf("could not keep %q as it was left: %v", game.Name, err))
			return
		}
	}
	if stretch {
		d.Log.Log("info", fmt.Sprintf("%q has not changed its save for %s; sending it to your other devices",
			game.Name, spokenLength(quietAfter)))
	}
	if d.OnGameChanged != nil {
		d.OnGameChanged(gameID)
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		_, _ = d.P2P.SyncGame(ctx, gameID)
	}()
}

// Games installed where no library lists them — a folder of the user's own,
// "G:\Games\Crimson Desert" — are found by the name of their install folder:
// the names the game database knows it by, and the game's own name when it
// is specific enough not to be a folder of something else. Once a session
// has been seen that way, the folder is remembered, so the game is known to
// be installed and its programs can be told by name (programsIn).

// genericFolders are names no game's folder is recognised by alone.
var genericFolders = map[string]bool{
	"games": true, "game": true, "steam": true, "steamapps": true, "common": true, "program files": true,
	"program files x86": true, "bin": true, "bin64": true, "win64": true, "binaries": true, "launcher": true,
	"data": true, "x64": true, "app": true, "client": true, "epic games": true, "gog games": true,
}

func (d *Daemon) installFolderNames(g store.Game) []string {
	var names []string
	add := func(n string) {
		n = sessions.FolderName(n)
		if n == "" || genericFolders[n] {
			return
		}
		for _, have := range names {
			if have == n {
				return
			}
		}
		names = append(names, n)
	}
	d.devMu.Lock()
	db := d.devDBLocked()
	d.devMu.Unlock()
	for _, n := range db.InstallNames(g.Name, g.AppID) {
		if len(sessions.FolderName(n)) >= 4 {
			add(n)
		}
	}
	// The game's own name only when it could hardly name anything else:
	// two words, or one long one. "Rust" is a folder in every Rust toolchain.
	if n := sessions.FolderName(g.Name); strings.Contains(n, " ") && len(n) >= 6 || len(n) >= 10 {
		add(g.Name)
	}
	return names
}

func installFolderMark(gameID string) string { return "installdir:" + gameID }

// knownInstallFolder is the install folder a session of the game was seen
// running from, when it is still there.
func (d *Daemon) knownInstallFolder(gameID string) string {
	dir := d.Store.Mark(installFolderMark(gameID))
	if dir == "" {
		return ""
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return ""
	}
	return dir
}

// rememberInstallFolders records, for each game found running by the name of
// its install folder, which folder that was.
func (d *Daemon) rememberInstallFolders(procs []sessions.Proc, targets []sessions.Target, running map[string]int) {
	for _, t := range targets {
		pid, ok := running[t.GameID]
		if !ok || len(t.Dirs) > 0 {
			continue
		}
		for _, p := range procs {
			if p.PID != pid {
				continue
			}
			if dir := sessions.FolderOf(p, t); dir != "" && d.Store.Mark(installFolderMark(t.GameID)) != dir {
				_ = d.Store.SetMark(installFolderMark(t.GameID), dir)
			}
		}
	}
}

// playingHere says whether a game counts as being played on this device, for
// the sync engine (syncengine/playing.go): a session is open, a stretch of
// changes is (noteActivity) — or one is about to be: a change made here waits
// out the watcher's debounce. A sync landing in that gap would otherwise
// send the change on the moment before it is held.
func (d *Daemon) playingHere(gameID string) bool {
	if !d.PlayingSince(gameID).IsZero() || d.changingNow(gameID) {
		return true
	}
	return !d.opts.SyncEveryChange && d.Watcher != nil && d.Watcher.ChangePending(gameID)
}

// sessionEndGrace is how long after a session ends a change to its save is
// still the session's — the game writing as it closes — and sent at once.
const sessionEndGrace = 2 * time.Minute

// recognisedLocked reports a game whose sessions this device sees: one has
// been recorded, or the folder it ran from is known. While no session of such
// a game is open, a change to its save is not play, and is sent at once;
// only a game whose program is not seen is held as a stretch (noteActivity).
// Called with sessions.mu held.
func (d *Daemon) recognisedLocked(gameID string) bool {
	if dir := d.Store.Mark(installFolderMark(gameID)); dir != "" {
		return true
	}
	st, err := d.Store.PlayStatsFor(gameID)
	return err == nil && st.Sessions > 0
}
