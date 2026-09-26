package e2e

import (
	"encoding/json"
	"strings"
	"testing"
)

// The app's Activity timeline, from a terminal: what happened to a save, in
// the words the app uses.
func TestCLI_ActivityShowsWhatHappened(t *testing.T) {
	c := newCLI(t)
	dir := c.saveDir("activitygame", map[string]string{"a.sav": "one"})
	c.mustRun("add", "Activity Game", dir)
	c.mustRun("snapshot", "activity-game", "before the boss")

	out := c.mustRun("activity")
	if !strings.Contains(out, "Snapshot: before the boss") || !strings.Contains(out, "Activity Game") {
		t.Errorf("activity does not show the snapshot:\n%s", out)
	}

	var report struct {
		Items []struct {
			GameID  string `json:"gameId"`
			Kind    string `json:"kind"`
			Comment string `json:"comment"`
		} `json:"items"`
	}
	c.mustJSON(&report, "activity", "activity-game", "--json")
	found := false
	for _, it := range report.Items {
		if it.GameID == "activity-game" && it.Kind == "snapshot" && it.Comment == "before the boss" {
			found = true
		}
	}
	if !found {
		t.Errorf("activity --json has no such snapshot: %+v", report.Items)
	}
	c.mustFail("activity", "no-such-game")
}

// With nothing to sync with, `sync` says so and exits non-zero — it used to
// print "Sync started" and exit 0 whatever happened.
func TestCLI_SyncWithNobodyOnlineSaysSo(t *testing.T) {
	c := newCLI(t)
	c.startDaemon()
	dir := c.saveDir("lonelygame", map[string]string{"a.sav": "one"})
	c.mustRun("add", "Lonely Game", dir)

	out := c.mustFail("sync", "--all")
	if !strings.Contains(out, "No other device is online") || strings.Contains(out, "Sync started") {
		t.Errorf("sync --all with nobody online said:\n%s", out)
	}
	out = c.mustFail("sync", "lonely-game")
	if !strings.Contains(out, "No other device is online") {
		t.Errorf("sync <game> with nobody online said:\n%s", out)
	}

	c.mustRun("pause", "30m")
	out = c.mustFail("sync", "--all")
	if !strings.Contains(out, "paused") {
		t.Errorf("sync --all while paused said:\n%s", out)
	}
	c.mustRun("resume")
}

// Offers: nothing waiting, and a clear refusal for one that is not offered.
func TestCLI_OffersWithNothingOffered(t *testing.T) {
	c := newCLI(t)
	c.startDaemon()
	var offers []json.RawMessage
	c.mustJSON(&offers, "offers", "--json")
	if len(offers) != 0 {
		t.Errorf("offers on a fresh device: %d", len(offers))
	}
	out := c.mustRun("offers")
	if !strings.Contains(out, "Nothing waiting") {
		t.Errorf("offers said:\n%s", out)
	}
	c.mustFail("offers", "place", "not-offered", c.home)
	c.mustFail("offers", "frobnicate")
}
