// Package sessions notices when a tracked game is being played: its process
// found running, and later gone.
//
// A play session is what the other tools in this space back up around —
// Game Backup Monitor snapshots when you stop playing, Hoard after each
// session — and what a person means by "my save from last night". OpenSave's
// watcher snapshots every change to a save; a session gives those a shape:
// the snapshot at the end of one is the save as it was left, and the sessions
// themselves say when a game was last played and for how long in all.
//
// Which process is which game is decided three ways, most certain first:
//
//   - Steam's own tag. On Linux — the Steam Deck included — Steam starts a
//     game, native or through Proton, with SteamAppId in its environment and
//     AppId=… on its launcher's command line.
//   - The launch program a game was given (its exe path), exactly.
//   - The game's install folder: Steam's, from its app manifest, or another
//     launcher's, by the folder's name. Anything running from inside it is
//     that game.
package sessions

import (
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode"
)

// Proc is a running process, as much of it as matching needs.
type Proc struct {
	PID int
	// Exe is the program's full path, where the system says.
	Exe string
	// Name is the program's file name, where the system gives no path: a
	// game protected against tampering lets nobody read where it runs from,
	// but the process list still says what it is called.
	Name string
	// Args is the command line, where the system says (Linux).
	Args []string
	// Measured says Memory and CPU were read: the program's working set in
	// bytes, and the processor time it has used so far. A game being played
	// holds hundreds of megabytes and keeps the processor busy; what a game
	// leaves behind when it closes — a crash handler, a stub of the same name
	// — holds next to nothing and does nothing (Idle).
	Measured bool
	Memory   uint64
	CPU      time.Duration
	// SteamAppID is the Steam app the process was started for, where Steam
	// tagged it (Linux: SteamAppId in the environment, or AppId=… on the
	// command line of Steam's launcher).
	SteamAppID string
}

// Target is a game to look for.
type Target struct {
	GameID string
	AppID  string
	// Exe is the program the game is launched with, if one was given.
	Exe string
	// Dirs are the game's install folders: anything running from inside one
	// is the game.
	Dirs []string
	// FolderNames are names the game's install folder goes by, for a game
	// installed where no library lists it ("G:GamesCrimson Desert"):
	// a program running from inside a folder of one of these names is the
	// game. Normalised with FolderName.
	FolderNames []string
	// Programs are the file names of the programs in the install folders, for
	// a process known only by name (Proc.Name). Lower-case.
	Programs []string
}

// Running reports which targets have a process running, from a list of
// processes: game id to the lowest matching pid.
func Running(procs []Proc, targets []Target) map[string]int {
	out := map[string]int{}
	for _, t := range targets {
		for _, p := range procs {
			if matches(p, t) {
				if pid, seen := out[t.GameID]; !seen || p.PID < pid {
					out[t.GameID] = p.PID
				}
			}
		}
	}
	return out
}

func matches(p Proc, t Target) bool {
	if t.AppID != "" && p.SteamAppID == t.AppID {
		return true
	}
	if p.Exe != "" && t.Exe != "" && samePath(p.Exe, t.Exe) {
		return true
	}
	if p.Exe == "" && p.Name != "" {
		name := strings.ToLower(p.Name)
		if t.Exe != "" && strings.EqualFold(filepath.Base(t.Exe), name) {
			return true
		}
		for _, prog := range t.Programs {
			if prog == name {
				return true
			}
		}
	}
	if p.Exe != "" && FolderOf(p, t) != "" {
		return true
	}
	for _, dir := range t.Dirs {
		if dir == "" {
			continue
		}
		if p.Exe != "" && inside(p.Exe, dir) {
			return true
		}
		// Through Proton the program is Wine; the game is on its command
		// line, as a path inside the install folder.
		for _, arg := range p.Args {
			if inside(arg, dir) {
				return true
			}
		}
	}
	return false
}

// InFolder reports whether a process is a program from inside one of a
// target's folders — by its own path, not its command line. Through Proton
// the program is Wine, and Wine is not a game anyone could launch.
func InFolder(p Proc, t Target) bool {
	if p.Exe == "" {
		return false
	}
	for _, dir := range t.Dirs {
		if dir != "" && inside(p.Exe, dir) {
			return true
		}
	}
	return false
}

func norm(p string) string {
	p = filepath.Clean(strings.TrimSpace(p))
	if runtime.GOOS == "windows" {
		p = strings.ToLower(p)
	}
	return p
}

func samePath(a, b string) bool { return norm(a) == norm(b) }

// inside reports whether path is within dir, not merely beginning with its
// name: "D:\Games\Hades II\x.exe" is not inside "D:\Games\Hades".
func inside(path, dir string) bool {
	p, d := norm(path), norm(dir)
	if p == d || d == "." || d == string(filepath.Separator) {
		return false
	}
	return strings.HasPrefix(p, d+string(filepath.Separator))
}

// Tracker turns what is running, polled, into sessions: a start when a game
// is first seen, an end when it has been gone for more than one poll.
//
// One poll's absence is not an end. A game restarting itself — to apply a
// setting, or a launcher handing over to the game — shows a moment with
// nothing running, and splitting one evening into two sessions there would
// take a snapshot mid-play.
type Tracker struct {
	// Grace is how long a game may be gone before its session ends.
	Grace time.Duration
	// OnStart and OnEnd report sessions. They are called from Poll, in
	// order; OnEnd is given the session's start.
	OnStart func(gameID string, at time.Time)
	OnEnd   func(gameID string, started, ended time.Time)

	mu      sync.Mutex
	playing map[string]*session
}

type session struct {
	started  time.Time
	lastSeen time.Time
	// marked is a session begun from outside (Begin). Only Finish ends it:
	// the game it was begun for is often one no process of which is ever
	// recognised — launched through Steam, say — and the poll not seeing it
	// says nothing about whether it is still being played.
	marked bool
}

// Poll takes what is running now.
func (t *Tracker) Poll(running map[string]int, now time.Time) {
	t.mu.Lock()
	if t.playing == nil {
		t.playing = map[string]*session{}
	}
	var started []string
	var ended []struct {
		id    string
		s     session
		ended time.Time
	}
	for id := range running {
		if s, ok := t.playing[id]; ok {
			s.lastSeen = now
			continue
		}
		t.playing[id] = &session{started: now, lastSeen: now}
		started = append(started, id)
	}
	for id, s := range t.playing {
		if _, still := running[id]; still || s.marked {
			continue
		}
		if now.Sub(s.lastSeen) > t.Grace {
			ended = append(ended, struct {
				id    string
				s     session
				ended time.Time
			}{id, *s, s.lastSeen})
			delete(t.playing, id)
		}
	}
	t.mu.Unlock()

	for _, id := range started {
		if t.OnStart != nil {
			t.OnStart(id, now)
		}
	}
	for _, e := range ended {
		if t.OnEnd != nil {
			// The session ended when the game was last seen, not when its
			// absence was confirmed.
			t.OnEnd(e.id, e.s.started, e.ended)
		}
	}
}

// Playing returns the games being played and since when.
func (t *Tracker) Playing() map[string]time.Time {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make(map[string]time.Time, len(t.playing))
	for id, s := range t.playing {
		out[id] = s.started
	}
	return out
}

// Begin and Finish mark a session from outside — `opensave wrap` launching a
// game knows exactly when it starts and stops. Finish ends it at once, grace
// or not.
func (t *Tracker) Begin(gameID string, at time.Time) {
	t.mu.Lock()
	if t.playing == nil {
		t.playing = map[string]*session{}
	}
	if s, ok := t.playing[gameID]; ok {
		// Already seen running: the same session, now one only Finish ends.
		s.marked = true
		t.mu.Unlock()
		return
	}
	t.playing[gameID] = &session{started: at, lastSeen: at, marked: true}
	t.mu.Unlock()
	if t.OnStart != nil {
		t.OnStart(gameID, at)
	}
}

func (t *Tracker) Finish(gameID string, at time.Time) {
	t.mu.Lock()
	s, ok := t.playing[gameID]
	if ok {
		delete(t.playing, gameID)
	}
	t.mu.Unlock()
	if ok && t.OnEnd != nil {
		t.OnEnd(gameID, s.started, at)
	}
}

// List returns the processes running now. Where the system cannot say, it
// returns none and an error, and nothing is ever seen playing.
func List() ([]Proc, error) { return listProcesses() }

// FolderName normalises a folder or game name for comparing the two: lower
// case, letters, digits and single spaces.
func FolderName(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		default:
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// FolderOf returns the folder, among those above a process's program, that
// is named as one of a target's install folders (Target.FolderNames), or "".
func FolderOf(p Proc, t Target) string {
	if p.Exe == "" || len(t.FolderNames) == 0 {
		return ""
	}
	dir := filepath.Dir(p.Exe)
	for dir != "" && dir != filepath.Dir(dir) {
		// Without spaces: the game database writes "ProjectZomboid" for a
		// folder anyone else calls "Project Zomboid".
		name := strings.ReplaceAll(FolderName(filepath.Base(dir)), " ", "")
		for _, want := range t.FolderNames {
			if name == strings.ReplaceAll(want, " ", "") {
				return dir
			}
		}
		dir = filepath.Dir(dir)
	}
	return ""
}

// Idle reports a process that holds next to no memory and has used no
// processor time since it was last seen (prevCPU, when it was): not a game
// being played, whatever it is called. A process that could not be measured
// is never called idle.
func Idle(p Proc, prevCPU time.Duration, seenBefore bool) bool {
	if !p.Measured || p.Memory >= idleMemory {
		return false
	}
	return !seenBefore || p.CPU-prevCPU < idleCPU
}

const (
	idleMemory = 32 << 20               // a game holds far more
	idleCPU    = 100 * time.Millisecond // per poll: 1% of ten seconds
)
