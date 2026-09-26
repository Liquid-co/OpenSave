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
