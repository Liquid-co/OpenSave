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

// A game whose program is not seen: a change is held, not synced, and kept
// once the save has stayed unchanged for firstChangeWait; a second soon after
// makes it a stretch, which ends after quietAfter unchanged.
func TestActivityWithoutASessionIsHandledAsPlay(t *testing.T) {
	d := newTestDaemon(t)
	g, dir := sessionGame(t, d, "unseen")
	if !d.holdSnapshotWhilePlaying(g.ID) {
		t.Fatal("a single change was not held")
	}
	d.sessions.mu.Lock()
	if q := d.sessions.active[g.ID].quietFor(); q != firstChangeWait {
		t.Errorf("a single change waits %v, want %v", q, firstChangeWait)
	}
	d.sessions.mu.Unlock()
	if !d.holdSnapshotWhilePlaying(g.ID) {
		t.Fatal("a second change soon after was not held")
	}
	d.sessions.mu.Lock()
	if q := d.sessions.active[g.ID].quietFor(); q != quietAfter {
		t.Errorf("a stretch waits %v, want %v", q, quietAfter)
	}
	d.sessions.mu.Unlock()
	if !d.changingNow(g.ID) || !d.P2P.Sync.PlayingHere(g.ID) {
		t.Fatal("a game changing its save is not handled as being played")
	}
	before := len(snapComments(t, d, g.ID))
	if err := os.WriteFile(filepath.Join(dir, "slot1.sav"), []byte("left like this"), 0o666); err != nil {
		t.Fatal(err)
	}

	// Quiet for longer than quietAfter.
	d.sessions.mu.Lock()
	d.sessions.active[g.ID].last = time.Now().Add(-quietAfter - time.Second)
	d.sessions.mu.Unlock()
	d.activityQuiet(g.ID)
	if d.changingNow(g.ID) || d.P2P.Sync.PlayingHere(g.ID) {
		t.Error("still handled as being played after the save went quiet")
	}
	if got := snapComments(t, d, g.ID); len(got) != before+1 {
		t.Errorf("snapshots %q: want one more, keeping the save as it was left", got)
	}
}

// The names a game's install folder is recognised by: never a generic folder,
// and the game's own name only when it could hardly be another folder's.
func TestInstallFolderNames(t *testing.T) {
	d := newTestDaemon(t)
	for name, want := range map[string]bool{"Crimson Desert": true, "Rust": false, "Games": false, "Satisfactory": true} {
		got := d.installFolderNames(store.Game{ID: name, Name: name})
		if (len(got) > 0) != want {
			t.Errorf("%q: folder names %v, want some = %v", name, got, want)
		}
	}
}

// A process doing nothing does not keep a session open; once a game's
// sessions are seen here, a change while it is closed is sent at once.
func TestSessionEndsWhenOnlyAStubIsLeft(t *testing.T) {
	d := newTestDaemon(t)
	g, _ := sessionGame(t, d, "stub")
	running := sessions.Proc{PID: 9, Exe: g.ExePath, Measured: true, Memory: 2 << 30, CPU: time.Second}
	d.sessions.list = func() ([]sessions.Proc, error) { return []sessions.Proc{running}, nil }
	d.PollSessions()
	if d.PlayingSince(g.ID).IsZero() {
		t.Fatal("setup: the game was not seen running")
	}
	// The game closed; a stub of the same program lingers, doing nothing.
	stub := sessions.Proc{PID: 9, Exe: g.ExePath, Measured: true, Memory: 800 << 10, CPU: time.Second}
	d.sessions.list = func() ([]sessions.Proc, error) { return []sessions.Proc{stub}, nil }
	d.PollSessions()
	since := d.PlayingSince(g.ID)
	d.sessions.tracker.Finish(g.ID, since.Add(time.Hour)) // past the grace the tracker allows
	d.PollSessions()
	if !d.PlayingSince(g.ID).IsZero() {
		t.Fatal("a lingering stub keeps the session open")
	}
	if d.holdSnapshotWhilePlaying(g.ID) {
		t.Error("a change right after the session ended was held")
	}
	if !d.recognised(g.ID) {
		t.Error("a game with a recorded session is not recognised")
	}
}

// A session's checkpoints are gone once the save it left is kept: one
// snapshot per session, not one per half hour.
func TestSessionEndDropsItsCheckpoints(t *testing.T) {
	d := newTestDaemon(t)
	g, dir := sessionGame(t, d, "checkpointed")
	d.sessions.list = func() ([]sessions.Proc, error) { return []sessions.Proc{{PID: 3, Exe: g.ExePath}}, nil }
	d.PollSessions()
	since := d.PlayingSince(g.ID)
	if since.IsZero() {
		t.Fatal("setup: not playing")
	}
	for i, data := range []string{"half an hour in", "an hour in"} {
		if err := os.WriteFile(filepath.Join(dir, "slot1.sav"), []byte(data), 0o666); err != nil {
			t.Fatal(err)
		}
		if _, err := d.Snapshots.Create(g.ID, checkpointComment, true); err != nil {
			t.Fatalf("checkpoint %d: %v", i, err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "slot1.sav"), []byte("as it was left"), 0o666); err != nil {
		t.Fatal(err)
	}
	d.sessions.tracker.Finish(g.ID, since.Add(90*time.Minute))

	got := snapComments(t, d, g.ID)
	if len(got) != 1 || got[0] != "After playing (1 h 30 min)" {
		t.Errorf("snapshots = %q, want only the session's", got)
	}
}

// In a session, a change is the session's: no stretch is started alongside
// it, whose timer once cleared the session's last checkpoint and let nearly
// every autosave after the first half hour through as a new one.
func TestSessionChangesStartNoStretch(t *testing.T) {
	d := newTestDaemon(t)
	g, _ := sessionGame(t, d, "in-session")
	d.sessions.list = func() ([]sessions.Proc, error) { return []sessions.Proc{{PID: 4, Exe: g.ExePath}}, nil }
	d.PollSessions()
	if d.PlayingSince(g.ID).IsZero() {
		t.Fatal("setup: not playing")
	}
	for i := 0; i < 3; i++ {
		if !d.holdSnapshotWhilePlaying(g.ID) {
			t.Fatalf("autosave %d in a fresh session was snapshotted", i)
		}
	}
	if d.changingNow(g.ID) {
		t.Error("a stretch runs alongside the session")
	}
}

// A game that autosaves every ten minutes keeps its stretch open: the wait
// is twice the longest gap seen, within bounds.
func TestStretchWaitAdaptsToAutosaves(t *testing.T) {
	for _, c := range []struct {
		a    activity
		want time.Duration
	}{
		{activity{changes: 1}, firstChangeWait},
		{activity{changes: 2, maxGap: time.Minute}, quietAfter},
		{activity{changes: 3, maxGap: 10 * time.Minute}, 20 * time.Minute},
		{activity{changes: 1, maxGap: 10 * time.Minute}, 20 * time.Minute},
		{activity{changes: 4, maxGap: time.Hour}, maxQuiet},
	} {
		if got := c.a.quietFor(); got != c.want {
			t.Errorf("%d changes, gap %v: wait %v, want %v", c.a.changes, c.a.maxGap, got, c.want)
		}
	}
}

// What a killed game leaves behind can keep its memory; it is no longer the
// game once it has used no processor time for a minute. One still working —
// a paused game draws its menu — stays.
func TestActiveProcesses_HoldingMemoryDoingNothing(t *testing.T) {
	d := newTestDaemon(t)
	t0 := time.Now()
	left := func(cpu time.Duration) sessions.Proc {
		return sessions.Proc{PID: 1, Exe: `G:\Games\Crimson Desert\bin64\CrimsonDesert.exe`, Measured: true, Memory: 2 << 30, CPU: cpu}
	}
	playing := func(cpu time.Duration) sessions.Proc {
		return sessions.Proc{PID: 2, Exe: `G:\Games\Other\game.exe`, Measured: true, Memory: 2 << 30, CPU: cpu}
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
	got = count(d.activeProcessesAt([]sessions.Proc{left(5 * time.Second), playing(3 * time.Second)}, t0.Add(70*time.Second)))
	if got[1] {
		t.Error("a process doing nothing for over a minute still counts as the game")
	}
	if !got[2] {
		t.Error("a game using the processor was dropped")
	}
}
