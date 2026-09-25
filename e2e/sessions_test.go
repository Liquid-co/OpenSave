package e2e

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/opensave/opensave/testutil"
)

// A play session, marked the way `opensave wrap` marks one: when it ends,
// the save as it was left is kept — a snapshot named for the session — and
// sent on to the other device without anyone pressing sync.
func TestSession_EndingOneKeepsTheSaveAndSyncsIt(t *testing.T) {
	a, b, gameID := pairAndTrack(t, "Session Game", map[string]string{"slot1.sav": "before"})
	testutil.SettleSync(t, gameID, a, b)

	type gameView struct {
		PlayingSince string `json:"playingSince"`
		PlaySessions int    `json:"playSessions"`
		PlaytimeMs   int64  `json:"playtimeMs"`
		Branches     map[string]struct {
			Snapshots []struct {
				Comment string `json:"comment"`
			} `json:"snapshots"`
		} `json:"branches"`
	}
	var games map[string]gameView

	a.API(http.MethodPost, "/api/games/"+gameID+"/session", map[string]string{"state": "start"}, nil)
	a.API(http.MethodGet, "/api/games", nil, &games)
	if games[gameID].PlayingSince == "" {
		t.Fatalf("after the session was started the game is not shown as being played")
	}

	// A session shorter than half a minute is not counted; wait it out.
	time.Sleep(31 * time.Second)
	a.WriteSave("slot1.sav", "after an evening")
	a.API(http.MethodPost, "/api/games/"+gameID+"/session", map[string]string{"state": "end"}, nil)

	a.API(http.MethodGet, "/api/games", nil, &games)
	g := games[gameID]
	if g.PlayingSince != "" || g.PlaySessions != 1 || g.PlaytimeMs < 30_000 {
		t.Errorf("after the session: playing=%q sessions=%d playtime=%dms, want one session of at least 30s and nothing playing",
			g.PlayingSince, g.PlaySessions, g.PlaytimeMs)
	}
	named := false
	for _, br := range g.Branches {
		for _, s := range br.Snapshots {
			named = named || strings.HasPrefix(s.Comment, "After playing")
		}
	}
	if !named {
		t.Errorf("no snapshot named for the session: %+v", g.Branches)
	}

	if !testutil.WaitFor(60*time.Second, func() bool { return b.ReadSave("slot1.sav") == "after an evening" }) {
		t.Errorf("the save as the session left it never reached the other device: it has %q", b.ReadSave("slot1.sav"))
	}
}
