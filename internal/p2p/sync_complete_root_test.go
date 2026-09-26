package p2p

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/opensave/opensave/internal/p2p/syncengine"
)

// A peer's report that it finished pulling from this device names the save
// location the files went into, and they are recorded against that location.
//
// They were recorded against the main save folder whatever the location. A
// second folder's settings.ini was then "shared" in the main folder as well,
// and the record keeps a path while either device still holds it there — so
// the settings.ini the game later wrote into its main folder read as one the
// other device had deleted, and the next sync deleted it.
//
// These drive the receiving side's real handlers, on the LAN and over the
// relay; device A is the one reporting.

// refreshInLine runs the lineage refresh a report starts in line, so a test
// can look at the result without racing it — or skips it, to look at what the
// report itself recorded.
func refreshInLine(t *testing.T, run bool) {
	t.Helper()
	prev := refreshAfterPull
	refreshAfterPull = func(e *Engine, gameID string, peer syncengine.Peer) {
		if !run {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		e.Sync.RefreshLineage(ctx, gameID, peer)
	}
	t.Cleanup(func() { refreshAfterPull = prev })
}

// reportOverLAN is A's sync-complete arriving at B's LAN route.
func (f *raceFixture) reportOverLAN(t *testing.T, data map[string]any) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"eventType": "sync-complete", "data": data})
	r := httptest.NewRequest(http.MethodPost, "/api/p2p/sync-event/"+f.gameID, bytes.NewReader(body))
	r.RemoteAddr = f.peer.Address + ":40000" // A's address: the report is attributed to A
	rc := chi.NewRouteContext()
	rc.URLParams.Add("gameId", f.gameID)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rc))
	w := httptest.NewRecorder()
	f.e.handleSyncEvent(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("sync-complete: %d %s", w.Code, w.Body.String())
	}
}

// reportOverRelay is the same report arriving through the relay.
func (f *raceFixture) reportOverRelay(t *testing.T, data map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(data)
	w := &WanClient{engine: f.e}
	w.handleMessage(context.Background(), RelayMessage{
		Type: "sync-event", From: f.peer.ID, GameID: f.gameID, EventType: "sync-complete", Data: raw,
	})
}

func (f *raceFixture) lineage(t *testing.T, root string) []string {
	t.Helper()
	files, _, err := f.s.GetSyncStateForRoot(f.gameID, f.peer.ID, root)
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestAPulledReportIsRecordedAgainstItsLocation(t *testing.T) {
	for name, report := range map[string]func(*raceFixture, *testing.T, map[string]any){
		"LAN":   (*raceFixture).reportOverLAN,
		"relay": (*raceFixture).reportOverRelay,
	} {
		t.Run(name, func(t *testing.T) {
			refreshInLine(t, true)
			f := newRaceFixture(t, true)

			// B handed its config folder's settings.ini to A, which wrote it.
			writeFile(t, filepath.Join(f.extraB, "settings.ini"), "config")
			writeFile(t, filepath.Join(f.extraA, "settings.ini"), "config")
			// And the game has a settings.ini of its own in B's main save
			// folder, which A has never had.
			writeFile(t, filepath.Join(f.bDir, "settings.ini"), "main folder's own")

			report(f, t, map[string]any{"pulledFiles": []string{"settings.ini"}, "root": "config"})

			if slices.Contains(f.lineage(t, ""), "settings.ini") {
				t.Errorf("the config folder's settings.ini was recorded as shared in the main save folder: %v", f.lineage(t, ""))
			}
			if !slices.Contains(f.lineage(t, "config"), "settings.ini") {
				t.Errorf("the config location's record lacks the file A reported: %v", f.lineage(t, "config"))
			}

			// What that record decides: B's own main-folder settings.ini is new,
			// to be sent — not something A deleted, to be deleted here.
			if _, err := f.e.Sync.SyncWithPeer(context.Background(), f.gameID, f.peer); err != nil {
				t.Fatal(err)
			}
			if !f.has(f.bDir, "settings.ini") {
				t.Error("B's own settings.ini in its main save folder was deleted as if A had deleted it")
			}
		})
	}
}

// The other end: a device that has just pulled into a location says which one.
// Without it, nothing the receiving side does can put the files anywhere but
// the main folder.
func TestAPulledReportNamesItsLocation(t *testing.T) {
	f := newRaceFixture(t, true)
	writeFile(t, filepath.Join(f.extraA, "d.cfg"), "new on A")
	f.a.mu.Lock()
	f.a.reports = nil
	f.a.mu.Unlock()

	if _, err := f.e.Sync.SyncWithPeer(context.Background(), f.gameID, f.peer); err != nil {
		t.Fatal(err)
	}
	if !f.has(f.extraB, "d.cfg") {
		t.Fatal("setup: B did not pull d.cfg into its config location")
	}
	f.a.mu.Lock()
	defer f.a.mu.Unlock()
	for _, r := range f.a.reports {
		if r.eventType != "sync-complete" {
			continue
		}
		if root, _ := r.data["root"].(string); root != "config" {
			t.Errorf("the report of the pull into config names location %q", root)
		}
		return
	}
	t.Fatalf("B never reported finishing the pull: %v", f.a.reports)
}

// What the report records on its own, with no refresh after it: a report from
// a version that names no location means the main folder, as it always did;
// one naming a location this device does not have is recorded nowhere; and an
// excluded file stays out of a location's record as it does the main one's.
func TestAPulledReportWithoutOrWithAnUnknownLocation(t *testing.T) {
	refreshInLine(t, false)
	f := newRaceFixture(t, true)

	f.reportOverLAN(t, map[string]any{"pulledFiles": []string{"slot4.sav"}})
	if !slices.Contains(f.lineage(t, ""), "slot4.sav") {
		t.Errorf("a report naming no location was not recorded against the main folder: %v", f.lineage(t, ""))
	}

	f.reportOverLAN(t, map[string]any{"pulledFiles": []string{"mod.pak"}, "root": "mods"})
	if slices.Contains(f.lineage(t, ""), "mod.pak") || slices.Contains(f.lineage(t, "mods"), "mod.pak") {
		t.Errorf("a location this device does not have was recorded: main %v, mods %v",
			f.lineage(t, ""), f.lineage(t, "mods"))
	}

	game, err := f.s.GetGame(f.gameID)
	if err != nil {
		t.Fatal(err)
	}
	game.SyncIgnore = "*.ini"
	if err := f.s.UpdateGame(game); err != nil {
		t.Fatal(err)
	}
	f.reportOverLAN(t, map[string]any{"pulledFiles": []string{"settings.ini", "d.cfg"}, "root": "config"})
	if got := f.lineage(t, "config"); slices.Contains(got, "settings.ini") || !slices.Contains(got, "d.cfg") {
		t.Errorf("config record = %v; want d.cfg and not the excluded settings.ini", got)
	}
}
