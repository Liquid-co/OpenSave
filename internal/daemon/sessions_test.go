package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/opensave/opensave/internal/sessions"
	"github.com/opensave/opensave/internal/store"
)

func sessionGame(t *testing.T, d *Daemon, id string) (store.Game, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "slot1.sav"), []byte("start"), 0o666); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(t.TempDir(), "Game", "game.exe")
	g := store.Game{ID: id, Name: "Session Game " + id, SavePath: dir, ActiveBranch: "main", MaxSnapshots: 20, ExePath: exe}
	if err := d.Store.CreateGame(g); err != nil {
		t.Fatal(err)
	}
	if err := d.Store.CreateBranch(id, "main"); err != nil && !strings.Contains(err.Error(), "UNIQUE") {
		t.Fatal(err)
	}
	return g, dir
}

func snapComments(t *testing.T, d *Daemon, id string) []string {
	t.Helper()
	snaps, err := d.Store.ListSnapshots(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, s := range snaps {
		out = append(out, s.Comment)
	}
	return out
}

// A game found running starts a session; one that changed its save while it
// ran leaves a snapshot named for the session, and the session is recorded.
func TestSessionLeavesASnapshotWhenTheSaveChanged(t *testing.T) {
	d := newTestDaemon(t)
	g, dir := sessionGame(t, d, "played")

	// Found running, by its launch program.
	d.sessions.list = func() ([]sessions.Proc, error) { return []sessions.Proc{{PID: 42, Exe: g.ExePath}}, nil }
	d.PollSessions()
	since := d.PlayingSince(g.ID)
	if since.IsZero() {
		t.Fatal("the running game was not seen as playing")
	}

	if err := os.WriteFile(filepath.Join(dir, "slot1.sav"), []byte("an evening later"), 0o666); err != nil {
		t.Fatal(err)
	}
	// It ended 72 minutes after it began.
	d.sessions.tracker.Finish(g.ID, since.Add(72*time.Minute))

	if got := snapComments(t, d, g.ID); len(got) != 1 || got[0] != "After playing (1 h 12 min)" {
		t.Errorf("snapshots = %q, want one named for the session", got)
	}
	st, err := d.Store.PlayStatsFor(g.ID)
	if err != nil || st.Sessions != 1 || st.PlaytimeMs != int64(72*time.Minute/time.Millisecond) {
		t.Errorf("play stats = %+v (%v), want one session of 72 minutes", st, err)
	}
	if !d.PlayingSince(g.ID).IsZero() {
		t.Errorf("still playing after the session ended")
	}
}

// When the watcher already kept the save as it was left, that snapshot is
// named for the session instead of a copy being taken.
func TestSessionNamesTheSnapshotAlreadyTaken(t *testing.T) {
	d := newTestDaemon(t)
	g, dir := sessionGame(t, d, "watched")
	start := time.Now()
	d.sessions.tracker.Begin(g.ID, start)

	if err := os.WriteFile(filepath.Join(dir, "slot1.sav"), []byte("saved at the checkpoint"), 0o666); err != nil {
		t.Fatal(err)
	}
	// What the watcher does on the change: an automatic snapshot, and the
	// content it holds recorded.
	if _, err := d.Snapshots.Create(g.ID, "", true); err != nil {
		t.Fatal(err)
	}
	game, _ := d.Store.GetGame(g.ID)
	hash, _ := d.currentContentHash(game)
	if err := d.Store.SetLastManifestHash(g.ID, hash); err != nil {
		t.Fatal(err)
	}

	d.sessions.tracker.Finish(g.ID, start.Add(45*time.Minute))
	if got := snapComments(t, d, g.ID); len(got) != 1 || got[0] != "After playing (45 min)" {
		t.Errorf("snapshots = %q, want the watcher's one, named for the session", got)
	}
}

// A session that changed nothing takes nothing; one too short to be play is
// not recorded at all.
func TestSessionsThatLeaveNothing(t *testing.T) {
	d := newTestDaemon(t)
	g, _ := sessionGame(t, d, "idle")
	start := time.Now()
	d.sessions.tracker.Begin(g.ID, start)
	d.sessions.tracker.Finish(g.ID, start.Add(20*time.Minute))
	if got := snapComments(t, d, g.ID); len(got) != 0 {
		t.Errorf("an unchanged save was snapshotted: %q", got)
	}
	if st, _ := d.Store.PlayStatsFor(g.ID); st.Sessions != 1 {
		t.Errorf("the session was not recorded: %+v", st)
	}

	d.sessions.tracker.Begin(g.ID, start)
	d.sessions.tracker.Finish(g.ID, start.Add(10*time.Second))
	if st, _ := d.Store.PlayStatsFor(g.ID); st.Sessions != 1 {
		t.Errorf("a ten-second blip counted as a session: %+v", st)
	}
}

// A launcher is not the game: a game launched through Steam is not "played"
// whenever Steam runs.
func TestSessionIgnoresALauncherAsTheLaunchProgram(t *testing.T) {
	d := newTestDaemon(t)
	g, _ := sessionGame(t, d, "viasteam")
	g.ExePath = `C:\Program Files (x86)\Steam\steam.exe`
	if err := d.Store.UpdateGame(g); err != nil {
		t.Fatal(err)
	}
	d.sessions.list = func() ([]sessions.Proc, error) { return []sessions.Proc{{PID: 7, Exe: g.ExePath}}, nil }
	d.PollSessions()
	if !d.PlayingSince(g.ID).IsZero() {
		t.Errorf("Steam running counted as playing the game")
	}
}

// A game started through a small program that hands over to the real one and
// exits is still being played: whichever was chosen as the launch program,
// the game runs from that folder.
func TestSessionRecognisesTheGameBesideItsLaunchProgram(t *testing.T) {
	d := newTestDaemon(t)
	g, _ := sessionGame(t, d, "handover")
	gameDir := filepath.Join(t.TempDir(), "ELDEN RING", "Game")
	g.ExePath = filepath.Join(gameDir, "start_protected_game.exe")
	if err := d.Store.UpdateGame(g); err != nil {
		t.Fatal(err)
	}
	// The program chosen has exited; the one it started is running.
	d.sessions.list = func() ([]sessions.Proc, error) {
		return []sessions.Proc{{PID: 9, Exe: filepath.Join(gameDir, "eldenring.exe")}}, nil
	}
	d.PollSessions()
	if d.PlayingSince(g.ID).IsZero() {
		t.Error("the game its launch program started was not seen as playing")
	}
}

// That folder and nothing more: a program beside it is not the game, and the
// folder a shortcut is kept in says nothing about where the game is.
func TestSessionLaunchFolderClaimsNothingElse(t *testing.T) {
	root := t.TempDir()
	for label, c := range map[string]struct{ exe, running string }{
		"a neighbouring folder": {filepath.Join(root, "Game", "game.exe"), filepath.Join(root, "Tools", "editor.exe")},
		"a shortcut's folder":   {filepath.Join(root, "Desktop", "Game.lnk"), filepath.Join(root, "Desktop", "portable.exe")},
	} {
		d := newTestDaemon(t)
		g, _ := sessionGame(t, d, "claims")
		g.ExePath = c.exe
		if err := d.Store.UpdateGame(g); err != nil {
			t.Fatal(err)
		}
		d.sessions.list = func() ([]sessions.Proc, error) {
			return []sessions.Proc{{PID: 11, Exe: c.running}}, nil
		}
		d.PollSessions()
		if !d.PlayingSince(g.ID).IsZero() {
			t.Errorf("%s: %s running counted as playing a game launched by %s", label, c.running, c.exe)
		}
	}
}

func TestSpokenLength(t *testing.T) {
	for dur, want := range map[time.Duration]string{
		40 * time.Second: "1 min", 45 * time.Minute: "45 min", time.Hour: "1 h", 72 * time.Minute: "1 h 12 min",
	} {
		if got := spokenLength(dur); got != want {
			t.Errorf("spokenLength(%s) = %q, want %q", dur, got, want)
		}
	}
}

// While a game is played, a change to its save is snapshotted once per
// checkpointEvery, not at every autosave; and it is not synced either way.
func TestSessionHoldsSnapshotsAndSyncWhilePlaying(t *testing.T) {
	d := newTestDaemon(t)
	g, _ := sessionGame(t, d, "autosaving")
	if d.holdSnapshotWhilePlaying(g.ID) {
		t.Fatal("held while nothing is being played")
	}
	d.sessions.list = func() ([]sessions.Proc, error) { return []sessions.Proc{{PID: 7, Exe: g.ExePath}}, nil }
	d.PollSessions()
	if d.PlayingSince(g.ID).IsZero() {
		t.Fatal("setup: not playing")
	}
	if !d.holdSnapshotWhilePlaying(g.ID) {
		t.Error("an autosave right after the game started was snapshotted")
	}
	if !d.P2P.Sync.PlayingHere(g.ID) {
		t.Error("the sync engine does not know the game is being played")
	}
	// Checkpoints: half an hour after play began, then half an hour after
	// the last one.
	start := time.Now()
	for _, c := range []struct {
		last  time.Time
		after time.Duration
		want  bool
	}{
		{time.Time{}, 10 * time.Minute, false},
		{time.Time{}, checkpointEvery, true},
		{start.Add(40 * time.Minute), 50 * time.Minute, false},
		{start.Add(40 * time.Minute), 70 * time.Minute, true},
	} {
		if got := checkpointDue(start, c.last, start.Add(c.after)); got != c.want {
			t.Errorf("checkpoint due %v after start (last %v) = %v, want %v", c.after, c.last, got, c.want)
		}
	}
}

// A process doing nothing does not keep a session open, and with it the
// game's save held back from syncing.
func TestSessionEndsWhenOnlyAStubIsLeft(t *testing.T) {
	d := newTestDaemon(t)
	g, _ := sessionGame(t, d, "stub")
	running := sessions.Proc{PID: 9, Exe: g.ExePath, Measured: true, CPU: time.Second}
	d.sessions.list = func() ([]sessions.Proc, error) { return []sessions.Proc{running}, nil }
	d.PollSessions()
	if d.PlayingSince(g.ID).IsZero() {
		t.Fatal("setup: the game was not seen running")
	}
	// The game closed; a stub of the same program lingers, doing nothing, and
	// has for longer than idleFor.
	stub := sessions.Proc{PID: 9, Exe: g.ExePath, Measured: true, CPU: time.Second}
	d.sessions.mu.Lock()
	d.sessions.cpu[9] = procSeen{cpu: time.Second, busy: time.Now().Add(-idleFor - time.Minute)}
	d.sessions.mu.Unlock()
	d.sessions.list = func() ([]sessions.Proc, error) { return []sessions.Proc{stub}, nil }
	d.PollSessions()
	since := d.PlayingSince(g.ID)
	d.sessions.tracker.Finish(g.ID, since.Add(time.Hour)) // past the grace the tracker allows
	d.PollSessions()
	if !d.PlayingSince(g.ID).IsZero() {
		t.Fatal("a lingering stub keeps the session open")
	}
	if d.holdSnapshotWhilePlaying(g.ID) {
		t.Error("a change after the session ended was held")
	}
}

// A process that has used no processor time for idleFor is no longer the game,
// whatever it holds: what a killed game leaves behind. One quiet for less —
// a game minimised in the background, throttled or suspended — still is, so
// its session and the hold on its save go on. One working stays.
func TestActiveProcesses_QuietForLongIsNotTheGame(t *testing.T) {
	d := newTestDaemon(t)
	t0 := time.Now()
	left := func(cpu time.Duration) sessions.Proc {
		return sessions.Proc{PID: 1, Exe: `G:\Games\Crimson Desert\bin64\CrimsonDesert.exe`, Measured: true, CPU: cpu}
	}
	playing := func(cpu time.Duration) sessions.Proc {
		return sessions.Proc{PID: 2, Exe: `G:\Games\Other\game.exe`, Measured: true, CPU: cpu}
	}
	count := func(ps []sessions.Proc) map[int]bool {
		m := map[int]bool{}
		for _, p := range ps {
			m[p.PID] = true
		}
		return m
	}
	got := count(d.activeProcessesAt([]sessions.Proc{left(5 * time.Second), playing(time.Second)}, t0))
	if !got[1] || !got[2] {
		t.Fatalf("first seen: %v, want both", got)
	}
	got = count(d.activeProcessesAt([]sessions.Proc{left(5 * time.Second), playing(2 * time.Second)}, t0.Add(30*time.Second)))
	if !got[1] || !got[2] {
		t.Errorf("half a minute in: %v, want both", got)
	}
	// Minimised and throttled: quiet for minutes, and still the game.
	for _, at := range []time.Duration{70 * time.Second, 4 * time.Minute} {
		got = count(d.activeProcessesAt([]sessions.Proc{left(5 * time.Second), playing(3 * time.Second)}, t0.Add(at)))
		if !got[1] {
			t.Errorf("quiet for %s: no longer counted, so a game in the background would lose its session", at)
		}
	}
	got = count(d.activeProcessesAt([]sessions.Proc{left(5 * time.Second), playing(4 * time.Second)}, t0.Add(idleFor+10*time.Second)))
	if got[1] {
		t.Error("a process doing nothing for longer than idleFor still counts as the game")
	}
	if !got[2] {
		t.Error("a game using the processor was dropped")
	}
}

// quietGame runs one game process whose processor time grows by perPoll at
// every poll.
func quietGame(t *testing.T, d *Daemon, id string, perPoll time.Duration) store.Game {
	t.Helper()
	g, _ := sessionGame(t, d, id)
	cpu := time.Duration(0)
	d.sessions.list = func() ([]sessions.Proc, error) {
		cpu += perPoll
		return []sessions.Proc{{PID: 7, Exe: g.ExePath, Measured: true, CPU: cpu}}, nil
	}
	return g
}

// What a game left behind, still ticking over now and then, holds a session
// open, and with it the hold on the save. An hour without a save, all but
// idle, and the session is taken as over; the process still there does not
// start it again, and the save changing again does.
func TestQuietSession_ALeftoverThatTicksOverIsLetGo(t *testing.T) {
	d := newTestDaemon(t)
	g := quietGame(t, d, "leftover", 30*time.Millisecond) // busy enough to count, every poll
	t0 := time.Now()
	for i := 0; i <= 5; i++ {
		d.pollSessionsAt(t0.Add(time.Duration(i) * 10 * time.Minute))
	}
	if d.PlayingSince(g.ID).IsZero() {
		t.Fatal("setup: the session ended before the hour")
	}
	d.pollSessionsAt(t0.Add(61 * time.Minute))
	if !d.PlayingSince(g.ID).IsZero() {
		t.Fatal("an hour without a save, all but idle, still holds the save")
	}
	d.pollSessionsAt(t0.Add(62 * time.Minute))
	if !d.PlayingSince(g.ID).IsZero() {
		t.Error("the process still running started the session again at once")
	}
	if err := d.Store.SetLastManifestHash(g.ID, "a save made since"); err != nil {
		t.Fatal(err)
	}
	d.pollSessionsAt(t0.Add(63 * time.Minute))
	if d.PlayingSince(g.ID).IsZero() {
		t.Error("the game saved again but no session started")
	}
}

// A game saved only by hand can go hours without a save while it is played.
// It keeps drawing, so its session goes on.
func TestQuietSession_AGamePlayedWithoutSavingKeepsItsSession(t *testing.T) {
	d := newTestDaemon(t)
	g := quietGame(t, d, "manual-saves", time.Minute) // a minute of processor time per ten
	t0 := time.Now()
	for i := 0; i <= 18; i++ { // three hours
		d.pollSessionsAt(t0.Add(time.Duration(i) * 10 * time.Minute))
	}
	if d.PlayingSince(g.ID).IsZero() {
		t.Error("a game being played without saving lost its session")
	}
}

// A session begun from outside — the Deck plugin, from Steam's launch — ends
// when that says so, however quiet the game is.
func TestQuietSession_ASessionBegunFromOutsideIsNotEnded(t *testing.T) {
	d := newTestDaemon(t)
	g := quietGame(t, d, "deck", 30*time.Millisecond)
	t0 := time.Now()
	d.sessions.tracker.Begin(g.ID, t0)
	for i := 0; i <= 12; i++ {
		d.pollSessionsAt(t0.Add(time.Duration(i) * 10 * time.Minute))
	}
	if d.PlayingSince(g.ID).IsZero() {
		t.Error("a session begun by the Deck plugin was ended as quiet")
	}
}
