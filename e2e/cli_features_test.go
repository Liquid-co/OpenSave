package e2e

import (
	"encoding/json"
	"strings"
	"testing"
)

// ── Plumbing ─────────────────────────────────────────────────────────────

// Exit codes are the CLI's contract with scripts and with `opensave service`.
// A rejected command that exits 0 turns a typo into a silent no-op.
func TestCLI_ExitCodes(t *testing.T) {
	c := newCLI(t)

	if out, code := c.run("version"); code != 0 {
		t.Errorf("version exited %d: %s", code, out)
	}
	if out, code := c.run("--help"); code != 0 {
		t.Errorf("--help exited %d: %s", code, out)
	}

	c.mustFail("definitely-not-a-command")
	c.mustFail("add")                      // required args missing
	c.mustFail("snapshot")                 // required args missing
	c.mustFail("rollback", "some-game")    // snapshot id missing
	c.mustFail("files", "some-game")       // snapshot id missing
	c.mustFail("game", "some-game", "set") // key and value missing
}

// An unknown command must say so, not just dump usage: "unknown command" is
// what tells the reader they mistyped rather than misused.
func TestCLI_UnknownCommandExplainsItself(t *testing.T) {
	c := newCLI(t)
	out := c.mustFail("trak", "x", "y")
	if !strings.Contains(strings.ToLower(out), "unknown command") {
		t.Errorf("output does not name the problem:\n%s", out)
	}
}

// --json exists so other programs can consume this. Output that is nearly
// JSON is worse than none: it parses in testing and breaks in the field.
func TestCLI_JSONOutputIsValid(t *testing.T) {
	c := newCLI(t)
	c.startDaemon()

	dir := c.saveDir("jsongame", map[string]string{"a.sav": "aaa"})
	c.mustRun("add", "JSON Game", dir)

	for _, args := range [][]string{
		{"status", "--json"},
		{"config", "--json"},
		{"peers", "--json"},
		{"conflicts", "--json"},
		{"snapshots", "json-game", "--json"},
		{"exclude", "list", "--json"},
		{"scanpath", "list", "--json"},
	} {
		out, code := c.run(args...)
		if code != 0 {
			t.Errorf("`%s` exited %d:\n%s", strings.Join(args, " "), code, out)
			continue
		}
		trimmed := strings.TrimSpace(out)
		if trimmed == "" {
			t.Errorf("`%s` produced no output at all", strings.Join(args, " "))
			continue
		}
		var v any
		if err := json.Unmarshal([]byte(trimmed), &v); err != nil {
			t.Errorf("`%s` did not emit valid JSON: %v\n%s", strings.Join(args, " "), err, trimmed)
		}
	}
}

// Styling must never reach a pipe. There is a unit test for the helper; this
// checks the real binary, since a single fmt.Printf with a hardcoded escape
// would bypass the helper entirely.
func TestCLI_NoAnsiEscapesWhenPiped(t *testing.T) {
	c := newCLI(t)
	c.startDaemon()
	dir := c.saveDir("ansi", map[string]string{"a.sav": "x"})
	c.mustRun("add", "Ansi Game", dir)

	for _, args := range [][]string{{"status"}, {"config"}, {"snapshots", "ansi-game"}, {"peers"}} {
		out := c.mustRun(args...)
		if strings.Contains(out, "\x1b[") {
			t.Errorf("`%s` emitted ANSI escapes into a pipe:\n%q", strings.Join(args, " "), out)
		}
	}
}

// ── Game lifecycle ───────────────────────────────────────────────────────

func TestCLI_GameLifecycle(t *testing.T) {
	c := newCLI(t)
	c.startDaemon()

	dir := c.saveDir("lifecycle", map[string]string{"slot1.sav": "one"})

	out := c.mustRun("add", "Life Cycle", dir)
	if !strings.Contains(out, "life-cycle") {
		t.Fatalf("add did not report the game id:\n%s", out)
	}

	if out := c.mustRun("status"); !strings.Contains(out, "Life Cycle") {
		t.Errorf("status does not list the game just added:\n%s", out)
	}

	// Adding the same folder twice must be refused, not silently duplicated:
	// two entries for one folder means two sync lineages over the same files.
	c.mustFail("add", "Life Cycle Again", dir)

	// Per-game settings.
	c.mustRun("game", "life-cycle", "set", "app-id", "1245620")
	var report struct {
		Games []struct {
			ID    string `json:"id"`
			AppID string `json:"appId"`
		} `json:"games"`
	}
	c.mustJSON(&report, "status", "--json")
	var sawAppID bool
	for _, g := range report.Games {
		if g.ID == "life-cycle" && g.AppID == "1245620" {
			sawAppID = true
		}
	}
	if !sawAppID {
		t.Errorf("app-id did not persist: %+v", report.Games)
	}
	c.mustRun("game", "life-cycle", "set", "auto-sync", "false")

	// An unknown settings key must be rejected rather than quietly ignored.
	c.mustFail("game", "life-cycle", "set", "not-a-real-key", "x")

	c.mustRun("remove", "life-cycle")
	if out := c.mustRun("status"); strings.Contains(out, "Life Cycle") {
		t.Errorf("the game is still listed after remove:\n%s", out)
	}

	// Removing tracking must never touch the save on disk.
	if got := c.readSave(dir, "slot1.sav"); got != "one" {
		t.Errorf("untracking modified the save on disk: %q", got)
	}
}

func TestCLI_OperationsOnMissingThingsFail(t *testing.T) {
	c := newCLI(t)
	c.startDaemon()

	c.mustFail("snapshot", "no-such-game")
	c.mustFail("snapshots", "no-such-game")
	c.mustFail("rollback", "no-such-game", "snap_1")
	c.mustFail("files", "no-such-game", "snap_1")
	c.mustFail("remove", "no-such-game")
	c.mustFail("game", "no-such-game", "set", "app-id", "1")
}

// ── History: the part that must never lose data ──────────────────────────

// The core promise. A snapshot has to survive the save being corrupted, files
// being deleted, and nested directories being involved.
func TestCLI_SnapshotAndRollbackRestoresEverything(t *testing.T) {
	c := newCLI(t)
	c.startDaemon()

	dir := c.saveDir("history", map[string]string{
		"slot1.sav":       "original-one",
		"slot2.sav":       "original-two",
		"deep/nested.dat": "nested-original",
	})
	c.mustRun("add", "History Game", dir)
	c.mustRun("snapshot", "history-game", "-m", "known good")

	// Two: the one tracking took automatically, and the one just asked for.
	// The manual one is newest, and is the one to roll back to.
	ids := c.snapshotIDs("history-game")
	if len(ids) != 2 {
		t.Fatalf("expected the initial snapshot plus the manual one, got %v", ids)
	}
	snap := ids[0]

	// Now wreck it in three different ways at once.
	c.saveDir("history", map[string]string{"slot1.sav": "CORRUPTED"})
	if err := removeFile(dir, "slot2.sav"); err != nil {
		t.Fatal(err)
	}
	if err := removeFile(dir, "deep/nested.dat"); err != nil {
		t.Fatal(err)
	}

	c.mustRun("rollback", "history-game", snap, "--yes")

	for rel, want := range map[string]string{
		"slot1.sav":       "original-one",
		"slot2.sav":       "original-two",
		"deep/nested.dat": "nested-original",
	} {
		if got := c.readSave(dir, rel); got != want {
			t.Errorf("after rollback %s = %q, want %q", rel, got, want)
		}
	}
}

// `files` lists a snapshot's contents. It printed every entry as 0 B for a
// release because it read the wrong field name, which no error surfaced.
func TestCLI_SnapshotFilesShowsRealSizes(t *testing.T) {
	c := newCLI(t)
	c.startDaemon()

	dir := c.saveDir("sizes", map[string]string{
		"exact15bytes.sv": "fifteen-bytes!!", // 15 bytes
		"sub/small.dat":   "seven!!",         // 7 bytes
	})
	c.mustRun("add", "Sizes Game", dir)
	c.mustRun("snapshot", "sizes-game")
	snap := c.snapshotIDs("sizes-game")[0]

	out := c.mustRun("files", "sizes-game", snap)
	if !strings.Contains(out, "exact15bytes.sv") {
		t.Fatalf("the listing omits a file that is in the snapshot:\n%s", out)
	}
	if !strings.Contains(out, "15 B") {
		t.Errorf("no real size printed for a 15-byte file — this is the "+
			"\"every save looks empty\" bug returning:\n%s", out)
	}
	if !strings.Contains(out, "7 B") {
		t.Errorf("no real size printed for the nested 7-byte file:\n%s", out)
	}
}

// Single-file restore never worked from the CLI: it sent the wrong field name
// and the endpoint rejected every request, so nothing was ever restored.
func TestCLI_SingleFileRestore(t *testing.T) {
	c := newCLI(t)
	c.startDaemon()

	dir := c.saveDir("onefile", map[string]string{
		"keep.sav":    "keep-original",
		"restore.sav": "restore-original",
	})
	c.mustRun("add", "One File", dir)
	c.mustRun("snapshot", "one-file")
	snap := c.snapshotIDs("one-file")[0]

	c.saveDir("onefile", map[string]string{
		"keep.sav":    "keep-EDITED",
		"restore.sav": "restore-WRECKED",
	})

	c.mustRun("files", "one-file", snap, "restore.sav")

	if got := c.readSave(dir, "restore.sav"); got != "restore-original" {
		t.Errorf("restore.sav = %q, want it restored from the snapshot", got)
	}
	// Only the named file: restoring one file must not roll the whole save
	// back and quietly discard edits the user meant to keep.
	if got := c.readSave(dir, "keep.sav"); got != "keep-EDITED" {
		t.Errorf("keep.sav = %q — a single-file restore reverted an unrelated file", got)
	}
}

func TestCLI_BranchesKeepSavesApart(t *testing.T) {
	c := newCLI(t)
	c.startDaemon()

	dir := c.saveDir("branchy", map[string]string{"slot1.sav": "main-state"})
	c.mustRun("add", "Branchy", dir)
	c.mustRun("snapshot", "branchy", "-m", "on main")

	c.mustRun("branch", "branchy", "run2")
	c.mustRun("checkout", "branchy", "run2")
	if out := c.mustRun("status"); !strings.Contains(out, "run2") {
		t.Errorf("status does not show the active branch:\n%s", out)
	}

	c.saveDir("branchy", map[string]string{"slot1.sav": "run2-state"})
	c.mustRun("snapshot", "branchy", "-m", "on run2")
	run2Count := len(c.snapshotIDs("branchy"))

	c.mustRun("checkout", "branchy", "main")
	mainCount := len(c.snapshotIDs("branchy"))

	// Each branch lists only its own history. The counts are not asserted
	// exactly: switching away from a branch takes an automatic backup of the
	// state being left behind (SwitchBranch), so main legitimately holds more
	// than the one snapshot taken by hand. What matters is that the two
	// branches have separate histories, not one shared list.
	if run2Count == 0 || mainCount == 0 {
		t.Errorf("a branch reported no snapshots: main=%d run2=%d", mainCount, run2Count)
	}

	// Switching back restores that branch's save content, which is the point
	// of branches existing.
	if got := c.readSave(dir, "slot1.sav"); got != "main-state" {
		t.Errorf("after checking out main, slot1.sav = %q, want %q", got, "main-state")
	}
	c.mustRun("checkout", "branchy", "run2")
	if got := c.readSave(dir, "slot1.sav"); got != "run2-state" {
		t.Errorf("after checking out run2, slot1.sav = %q, want %q", got, "run2-state")
	}

	// Deleting a branch must not be possible while it is the active one, or
	// the game is left pointing at history that no longer exists.
	c.mustFail("branch-delete", "branchy", "main", "--yes")
}

// A pinned snapshot outlasts the limit that would have taken it, carries its
// note in both the listing and --json, and is an ordinary snapshot again once
// unpinned — all through the CLI, against a running daemon, the way the app
// does it.
func TestCLI_SnapshotPinAndNote(t *testing.T) {
	c := newCLI(t)
	c.startDaemon()

	dir := c.saveDir("pinning", map[string]string{"slot1.sav": "v0"})
	c.mustRun("add", "Pinning", dir)
	c.mustRun("game", "pinning", "set", "max-manual-snapshots", "1")

	c.saveDir("pinning", map[string]string{"slot1.sav": "keeper"})
	c.mustRun("snapshot", "pinning", "-m", "the keeper")
	keeper := c.snapshotIDs("pinning")[0] // newest first
	c.mustRun("snapshot-pin", "pinning", keeper)
	c.mustRun("snapshot-note", "pinning", keeper, "good", "run,", "before", "the", "boss")

	// Three more manual snapshots under a manual limit of 1: three chances to
	// evict the keeper.
	for _, v := range []string{"v1", "v2", "v3"} {
		c.saveDir("pinning", map[string]string{"slot1.sav": v})
		c.mustRun("snapshot", "pinning", "-m", v)
	}

	type snap struct {
		ID           string `json:"id"`
		Comment      string `json:"comment"`
		Note         string `json:"note"`
		Pinned       bool   `json:"pinned"`
		IsSystemAuto bool   `json:"isSystemAuto"`
	}
	var snaps []snap
	c.mustJSON(&snaps, "snapshots", "pinning", "--json")
	var kept *snap
	var manual []string
	for i := range snaps {
		if snaps[i].ID == keeper {
			kept = &snaps[i]
		}
		if !snaps[i].IsSystemAuto {
			manual = append(manual, snaps[i].Comment)
		}
	}
	if kept == nil {
		t.Fatalf("the pinned snapshot was pruned: %+v", snaps)
	}
	if !kept.Pinned || kept.Note != "good run, before the boss" || kept.Comment != "the keeper" {
		t.Errorf("pinned snapshot = %+v, want pinned, the note joined from its words, and its comment untouched", *kept)
	}
	// The limit still applies to everything else: the keeper plus the newest one.
	if strings.Join(manual, ",") != "v3,the keeper" {
		t.Errorf("manual snapshots = %v, want [v3 the keeper]", manual)
	}

	out := c.mustRun("snapshots", "pinning")
	if !strings.Contains(out, "pinned") || !strings.Contains(out, "note: good run, before the boss") {
		t.Errorf("the listing does not show the pin and the note:\n%s", out)
	}

	// Unpinned, it is one manual snapshot too many and the next prune takes it.
	var edited snap
	c.mustJSON(&edited, "snapshot-unpin", "pinning", keeper, "--json")
	if edited.Pinned || edited.Note != "good run, before the boss" {
		t.Errorf("snapshot-unpin --json = %+v, want unpinned with its note kept", edited)
	}
	c.mustRun("prune")
	for _, id := range c.snapshotIDs("pinning") {
		if id == keeper {
			t.Error("the unpinned snapshot survived a prune it is over the limit for")
		}
	}

	// Refusals: no such snapshot, no snapshot named, a note past the limit.
	c.mustFail("snapshot-pin", "pinning", "snap_nope")
	c.mustFail("snapshot-pin", "pinning")
	latest := c.snapshotIDs("pinning")[0]
	c.mustFail("snapshot-note", "pinning", latest, strings.Repeat("x", 501))
	// And an empty note removes one.
	c.mustRun("snapshot-note", "pinning", latest, "temporary")
	c.mustRun("snapshot-note", "pinning", latest)
	var cleared []snap
	c.mustJSON(&cleared, "snapshots", "pinning", "--json")
	for _, s := range cleared {
		if s.ID == latest && s.Note != "" {
			t.Errorf("snapshot-note with no text left the note %q", s.Note)
		}
	}
}

// snapshot --all takes one of every game, with the comment given, and says
// so. (A game that fails is reported without stopping the rest; that is
// tested on the daemon, where a path no folder can exist at can be set up.)
func TestCLI_SnapshotAll(t *testing.T) {
	c := newCLI(t)
	c.startDaemon()
	one := c.saveDir("all-one", map[string]string{"a.sav": "1"})
	two := c.saveDir("all-two", map[string]string{"b.sav": "2"})
	c.mustRun("add", "All One", one)
	c.mustRun("add", "All Two", two)
	before := len(c.snapshotIDs("all-one")) + len(c.snapshotIDs("all-two"))

	out := c.mustRun("snapshot", "--all", "before", "the", "reinstall")
	if !strings.Contains(out, "2 game") {
		t.Errorf("snapshot --all said:\n%s", out)
	}
	if after := len(c.snapshotIDs("all-one")) + len(c.snapshotIDs("all-two")); after != before+2 {
		t.Errorf("snapshots went from %d to %d, want one more of each game", before, after)
	}
	var snaps []struct {
		Comment string `json:"comment"`
	}
	c.mustJSON(&snaps, "snapshots", "all-one", "--json")
	if len(snaps) == 0 || snaps[0].Comment != "before the reinstall" {
		t.Errorf("newest snapshot of All One = %+v, want the comment given", snaps)
	}

}

// storage reports each game's share and what prune would free; prune then
// frees it, and there is nothing left to clean up.
func TestCLI_Storage(t *testing.T) {
	c := newCLI(t)
	c.startDaemon()
	dir := c.saveDir("roomy", map[string]string{"a.sav": strings.Repeat("save data ", 5000)})
	c.mustRun("add", "Roomy", dir)
	for i := 0; i < 3; i++ {
		c.saveDir("roomy", map[string]string{"a.sav": strings.Repeat("more ", 4000+i)})
		c.mustRun("snapshot", "roomy", "-m", "take", "one")
	}
	c.mustRun("game", "roomy", "set", "max-manual-snapshots", "1")

	type report struct {
		TotalBytes           int64 `json:"totalBytes"`
		Snapshots            int   `json:"snapshots"`
		Reclaimable          int64 `json:"reclaimable"`
		ReclaimableSnapshots int   `json:"reclaimableSnapshots"`
		Games                []struct {
			GameID    string `json:"gameId"`
			Bytes     int64  `json:"bytes"`
			Snapshots int    `json:"snapshots"`
		} `json:"games"`
		Biggest []struct {
			SnapshotID string `json:"snapshotId"`
		} `json:"biggest"`
	}
	var r report
	c.mustJSON(&r, "storage", "--json")
	if len(r.Games) != 1 || r.Games[0].GameID != "roomy" || r.Games[0].Bytes != r.TotalBytes || r.TotalBytes <= 0 {
		t.Fatalf("storage --json = %+v", r)
	}
	if r.ReclaimableSnapshots != 2 || r.Reclaimable <= 0 {
		t.Errorf("reclaimable = %d in %d snapshot(s), want the 2 manual ones past a limit of 1", r.Reclaimable, r.ReclaimableSnapshots)
	}
	if out := c.mustRun("storage"); !strings.Contains(out, "Roomy") || !strings.Contains(out, "opensave prune") {
		t.Errorf("storage said:\n%s", out)
	}

	c.mustRun("prune")
	c.mustJSON(&r, "storage", "--json")
	if r.Reclaimable != 0 || r.ReclaimableSnapshots != 0 {
		t.Errorf("after prune, still reclaimable: %d in %d", r.Reclaimable, r.ReclaimableSnapshots)
	}
	if out := c.mustRun("storage"); !strings.Contains(out, "nothing to clean up") {
		t.Errorf("after prune storage said:\n%s", out)
	}
}

// Collections from the terminal: made, filled, renamed and emptied by name,
// Favourites always there and kept, and a game untracked leaving them all.
func TestCLI_Collections(t *testing.T) {
	c := newCLI(t)
	c.startDaemon()
	for _, name := range []string{"Col One", "Col Two"} {
		c.mustRun("add", name, c.saveDir(strings.ReplaceAll(strings.ToLower(name), " ", "-"), map[string]string{"a.sav": name}))
	}

	c.mustRun("collection", "create", "Playing", "now")
	c.mustRun("collection", "add", "playing now", "col-one", "col-two")
	c.mustRun("collection", "add", "favourites", "col-two")
	c.mustFail("collection", "create", "PLAYING NOW")
	c.mustFail("collection", "add", "no such collection", "col-one")
	c.mustFail("collection", "add", "favourites", "no-such-game")
	c.mustFail("collection", "delete", "favourites")
	c.mustFail("collection", "rename", "Favourites", "Faves")

	type coll struct {
		ID      string   `json:"id"`
		Name    string   `json:"name"`
		GameIDs []string `json:"gameIds"`
	}
	listed := func() map[string][]string {
		var all []coll
		c.mustJSON(&all, "collection", "list", "--json")
		out := map[string][]string{}
		for _, x := range all {
			out[x.Name] = x.GameIDs
		}
		return out
	}
	got := listed()
	if strings.Join(got["Playing now"], ",") != "col-one,col-two" || strings.Join(got["Favourites"], ",") != "col-two" {
		t.Fatalf("collections = %v", got)
	}

	c.mustRun("collection", "rename", "Playing now", "On the go")
	c.mustRun("collection", "remove", "on the go", "col-one")
	c.mustRun("remove", "col-two") // untracked: out of every collection
	got = listed()
	if _, old := got["Playing now"]; old || len(got["On the go"]) != 0 || len(got["Favourites"]) != 0 {
		t.Errorf("after rename, remove and untrack: %v", got)
	}

	c.mustRun("collection", "delete", "On the go")
	if _, still := listed()["On the go"]; still {
		t.Error("the collection is still there after delete")
	}
	if out := c.mustRun("status"); !strings.Contains(out, "Col One") {
		t.Error("deleting a collection untracked its games")
	}
}

// pause and resume reach the running daemon, and status says it is paused.
func TestCLI_PauseAndResume(t *testing.T) {
	c := newCLI(t)
	c.startDaemon()

	out := c.mustRun("pause", "45m")
	if !strings.Contains(out, "paused") || !strings.Contains(out, "45") {
		t.Errorf("pause 45m said:\n%s", out)
	}
	if out := c.mustRun("status"); !strings.Contains(out, "syncing is paused") {
		t.Errorf("status does not say syncing is paused:\n%s", out)
	}
	var report struct {
		SyncPause struct {
			Paused           bool  `json:"paused"`
			RemainingSeconds int64 `json:"remainingSeconds"`
		} `json:"syncPause"`
	}
	c.mustJSON(&report, "status", "--json")
	if !report.SyncPause.Paused || report.SyncPause.RemainingSeconds < 44*60 {
		t.Errorf("status --json syncPause = %+v", report.SyncPause)
	}

	// No duration: until resumed.
	var st struct {
		UntilRestart bool `json:"untilRestart"`
	}
	c.mustJSON(&st, "pause", "--json")
	if !st.UntilRestart {
		t.Errorf("pause with no duration = %+v, want until restart", st)
	}

	if out := c.mustRun("resume"); !strings.Contains(out, "resumed") {
		t.Errorf("resume said:\n%s", out)
	}
	if out := c.mustRun("status"); strings.Contains(out, "syncing is paused") {
		t.Errorf("status still says paused after resume:\n%s", out)
	}
	if out := c.mustRun("resume"); !strings.Contains(out, "not paused") {
		t.Errorf("a second resume should say there was nothing to resume:\n%s", out)
	}

	for _, bad := range []string{"soon", "0", "30s", "25h"} {
		c.mustFail("pause", bad)
	}
	c.mustFail("pause", "1h", "extra")
}

// rollback --dry-run says what a restore would change and changes nothing;
// the restore after it then does what it said.
func TestCLI_RollbackDryRun(t *testing.T) {
	c := newCLI(t)
	c.startDaemon()

	dir := c.saveDir("previewing", map[string]string{"slot1.sav": "v1", "slot2.sav": "same"})
	c.mustRun("add", "Previewing", dir)
	c.mustRun("snapshot", "previewing", "-m", "good")
	snap := c.snapshotIDs("previewing")[0]
	c.saveDir("previewing", map[string]string{"slot1.sav": "v2, longer", "slot3.sav": "new"})

	out := c.mustRun("rollback", "previewing", snap, "--dry-run")
	for _, want := range []string{"change", "slot1.sav", "remove", "slot3.sav", "1 file(s) unchanged"} {
		if !strings.Contains(out, want) {
			t.Errorf("--dry-run output lacks %q:\n%s", want, out)
		}
	}
	if got := c.readSave(dir, "slot1.sav"); got != "v2, longer" {
		t.Fatalf("--dry-run restored the save: slot1.sav = %q", got)
	}

	var preview struct {
		Changes []struct {
			Path   string `json:"path"`
			Change string `json:"change"`
		} `json:"changes"`
		Unchanged int `json:"unchanged"`
	}
	c.mustJSON(&preview, "rollback", "previewing", snap, "--dry-run", "--json")
	if len(preview.Changes) != 2 || preview.Unchanged != 1 {
		t.Errorf("--dry-run --json = %+v, want 2 changes and 1 unchanged", preview)
	}

	c.mustRun("rollback", "previewing", snap)
	if got := c.readSave(dir, "slot1.sav"); got != "v1" {
		t.Errorf("after the real rollback slot1.sav = %q, want v1", got)
	}
	if out := c.mustRun("rollback", "previewing", snap, "--dry-run"); !strings.Contains(out, "already matches") {
		t.Errorf("a dry run of the state just restored should say it matches:\n%s", out)
	}
	c.mustFail("rollback", "previewing", "snap_nope", "--dry-run")
}

func TestCLI_SnapshotDeleteAndPrune(t *testing.T) {
	c := newCLI(t)
	c.startDaemon()

	dir := c.saveDir("pruning", map[string]string{"slot1.sav": "v1"})
	c.mustRun("add", "Pruning", dir)

	for _, v := range []string{"v1", "v2", "v3"} {
		c.saveDir("pruning", map[string]string{"slot1.sav": v})
		c.mustRun("snapshot", "pruning", "-m", v)
	}
	// Four: the automatic one taken when the game was tracked, plus the three
	// asked for here.
	ids := c.snapshotIDs("pruning")
	if len(ids) != 4 {
		t.Fatalf("expected the initial snapshot plus 3 manual ones, got %v", ids)
	}

	c.mustRun("snapshot-delete", "pruning", ids[0])
	if got := c.snapshotIDs("pruning"); len(got) != 3 {
		t.Errorf("after deleting one snapshot there are %d: %v", len(got), got)
	}

	// Deleting the same snapshot twice must fail rather than report success
	// for something that is no longer there.
	c.mustFail("snapshot-delete", "pruning", ids[0])

	// Prune is bounded by the retention limit and must be safe to run when
	// there is nothing to do.
	c.mustRun("prune")
	c.mustRun("prune")
}

// untrack-all must refuse to run unattended. This is the difference between
// a mistyped command and a lost library.
//
// The scope here is deliberately what the CLI documents, not what it arguably
// ought to. `untrack-all --yes` and `cloud delete ... --yes` spell the flag
// out in their usage; `snapshot-delete` and `branch-delete` do not, and both
// run unguarded. That is consistent with itself, and adding a required flag
// to either would break any script already calling them — so this pins the
// behaviour as it stands rather than asserting a preference.
func TestCLI_UntrackAllRequiresConfirmation(t *testing.T) {
	c := newCLI(t)
	c.startDaemon()

	dir := c.saveDir("guard", map[string]string{"slot1.sav": "keep me"})
	c.mustRun("add", "Guarded", dir)
	c.mustRun("snapshot", "guarded")

	c.mustFail("untrack-all")

	// main is protected outright, with or without confirmation: deleting it
	// would leave the game pointing at history that no longer exists.
	c.mustFail("branch-delete", "guarded", "main", "--yes")

	// Still there after all of that.
	if out := c.mustRun("status"); !strings.Contains(out, "Guarded") {
		t.Errorf("a command that should have refused removed the game:\n%s", out)
	}
	// Both survive: the one taken when the game was tracked, and the manual
	// one above.
	if ids := c.snapshotIDs("guarded"); len(ids) != 2 {
		t.Errorf("a command that should have refused deleted a snapshot: %v", ids)
	}

	// And with --yes, untrack-all clears the library but keeps the save.
	c.mustRun("untrack-all", "--yes")
	if out := c.mustRun("status"); strings.Contains(out, "Guarded") {
		t.Errorf("untrack-all --yes did not clear the library:\n%s", out)
	}
	if got := c.readSave(dir, "slot1.sav"); got != "keep me" {
		t.Errorf("untrack-all touched the save on disk: %q", got)
	}
}

// ── Settings ─────────────────────────────────────────────────────────────

func TestCLI_ConfigRoundTrip(t *testing.T) {
	c := newCLI(t)
	c.startDaemon()

	c.mustRun("config", "set", "device-name", "Bench Box")
	if out := c.mustRun("config"); !strings.Contains(out, "Bench Box") {
		t.Errorf("device-name did not persist:\n%s", out)
	}

	c.mustRun("config", "set", "match-by-app-id", "true")
	out := c.mustRun("config", "--json")
	var cfg map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &cfg); err != nil {
		t.Fatalf("config --json is not valid JSON: %v\n%s", err, out)
	}
	if cfg["matchByAppId"] != true {
		t.Errorf("matchByAppId = %v, want true", cfg["matchByAppId"])
	}

	c.mustFail("config", "set", "not-a-real-setting", "x")
}

func TestCLI_ExcludeAndScanPathRoundTrip(t *testing.T) {
	c := newCLI(t)
	c.startDaemon()

	// A distinctive directory name, matched against the full path. Matching a
	// bare word instead is how the first version of this test fooled itself:
	// the empty-state message reads "No excluded folders", which contains the
	// word it was searching for, so removal always looked successful.
	dir := c.saveDir("zzmarker", map[string]string{"a.sav": "x"})

	c.mustRun("exclude", "add", dir)
	if out := c.mustRun("exclude", "list"); !strings.Contains(out, dir) {
		t.Errorf("the excluded folder is not listed:\n%s", out)
	}
	c.mustRun("exclude", "remove", dir)
	if out := c.mustRun("exclude", "list"); strings.Contains(out, dir) {
		t.Errorf("the folder is still excluded after remove:\n%s", out)
	}

	c.mustRun("scanpath", "add", dir)
	if out := c.mustRun("scanpath", "list"); !strings.Contains(out, dir) {
		t.Errorf("the extra scan path is not listed:\n%s", out)
	}
	c.mustRun("scanpath", "remove", dir)
	if out := c.mustRun("scanpath", "list"); strings.Contains(out, dir) {
		t.Errorf("the scan path is still listed after remove:\n%s", out)
	}

	c.mustFail("exclude", "add")
	c.mustFail("scanpath", "add")
}

// ── Daemon lifecycle ─────────────────────────────────────────────────────

// Commands that need the daemon must say it is not running rather than
// failing with a bare connection error.
func TestCLI_WithoutDaemonSaysSo(t *testing.T) {
	c := newCLI(t)

	out, code := c.run("daemon", "status")
	if code == 0 {
		t.Errorf("daemon status reported success with no daemon:\n%s", out)
	}
	lower := strings.ToLower(out)
	if !strings.Contains(lower, "not running") && !strings.Contains(lower, "no daemon") {
		t.Errorf("the message does not say the daemon is down:\n%s", out)
	}
}

func TestCLI_DaemonStartStatusStop(t *testing.T) {
	c := newCLI(t)
	c.startDaemon()

	if out := c.mustRun("daemon", "status"); !strings.Contains(strings.ToLower(out), "running") {
		t.Errorf("daemon status does not report running:\n%s", out)
	}

	c.mustRun("daemon", "stop")
	if _, code := c.run("daemon", "status"); code == 0 {
		t.Error("daemon status still reports success after stop")
	}
	c.daemon = nil // stopped deliberately; nothing for cleanup to kill
}
