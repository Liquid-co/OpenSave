package sessions

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestRunningMatchesEachWay(t *testing.T) {
	sep := string(filepath.Separator)
	games := filepath.Join(sep+"games", "steamapps", "common")
	targets := []Target{
		{GameID: "hades", AppID: "1145360", Dirs: []string{filepath.Join(games, "Hades")}},
		{GameID: "portable", Exe: filepath.Join(sep+"apps", "Portable Game", "game.exe")},
		{GameID: "proton", Dirs: []string{filepath.Join(games, "Elden Ring")}},
		{GameID: "hades2", Dirs: []string{filepath.Join(games, "Hades II")}},
	}
	procs := []Proc{
		{PID: 40, SteamAppID: "1145360", Exe: "/usr/bin/reaper"},
		{PID: 50, Exe: filepath.Join(sep+"apps", "Portable Game", "game.exe")},
		{PID: 60, Exe: "/usr/bin/wine64-preloader", Args: []string{"wine", filepath.Join(games, "Elden Ring", "Game", "eldenring.exe")}},
		// Beside, not inside: "Hades" must not claim "Hades II".
		{PID: 70, Exe: filepath.Join(games, "Hades II", "Hades2.exe")},
		{PID: 80, Exe: filepath.Join(sep+"windows", "explorer.exe")},
	}
	got := Running(procs, targets)
	want := map[string]int{"hades": 40, "portable": 50, "proton": 60, "hades2": 70}
	if len(got) != len(want) {
		t.Fatalf("running = %v, want %v", got, want)
	}
	for id, pid := range want {
		if got[id] != pid {
			t.Errorf("%s: pid %d, want %d (all: %v)", id, got[id], pid, got)
		}
	}
}

func TestTrackerSessions(t *testing.T) {
	var starts, ends []string
	var lastEnd [2]time.Time
	tr := &Tracker{
		Grace:   15 * time.Second,
		OnStart: func(id string, _ time.Time) { starts = append(starts, id) },
		OnEnd: func(id string, s, e time.Time) {
			ends = append(ends, id)
			lastEnd = [2]time.Time{s, e}
		},
	}
	t0 := time.Date(2026, 9, 25, 20, 0, 0, 0, time.UTC)
	at := func(sec int) time.Time { return t0.Add(time.Duration(sec) * time.Second) }

	tr.Poll(map[string]int{"hades": 1}, at(0))
	tr.Poll(map[string]int{"hades": 1}, at(10))
	// Gone for one poll: a restart, not an end.
	tr.Poll(map[string]int{}, at(20))
	tr.Poll(map[string]int{"hades": 2}, at(30))
	if len(starts) != 1 || len(ends) != 0 {
		t.Fatalf("a moment's absence split the session: starts %v, ends %v", starts, ends)
	}
	if since := tr.Playing()["hades"]; !since.Equal(at(0)) {
		t.Errorf("playing since %v, want %v", since, at(0))
	}
	// Gone for longer than the grace: it ended when last seen.
	tr.Poll(map[string]int{}, at(40))
	tr.Poll(map[string]int{}, at(60))
	if len(ends) != 1 || !lastEnd[0].Equal(at(0)) || !lastEnd[1].Equal(at(30)) {
		t.Fatalf("ends %v, last %v; want one session from 0s to 30s", ends, lastEnd)
	}

	// Marked from outside: polls that never see it do not end it — only
	// Finish does, and at once.
	tr.Begin("celeste", at(100))
	tr.Poll(map[string]int{}, at(130))
	tr.Poll(map[string]int{}, at(150))
	if _, playing := tr.Playing()["celeste"]; !playing || len(ends) != 1 {
		t.Fatalf("a marked session was ended by polls that could not see it: ends %v", ends)
	}
	tr.Finish("celeste", at(160))
	if len(ends) != 2 || !lastEnd[1].Equal(at(160)) {
		t.Errorf("a marked session: ends %v, last %v", ends, lastEnd)
	}
	tr.Finish("never-started", at(170))
	if len(ends) != 2 {
		t.Errorf("finishing a session never begun reported one")
	}
}

// The real process list finds this test itself, by its program.
func TestListFindsThisProcess(t *testing.T) {
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" {
		t.Skip("not supported here")
	}
	procs, err := List()
	if err != nil {
		t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	running := Running(procs, []Target{{GameID: "me", Exe: self}})
	if running["me"] != os.Getpid() {
		t.Errorf("this process (%d, %s) was not found among %d: %v", os.Getpid(), self, len(procs), running)
	}
}
