package p2p

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/opensave/opensave/internal/p2p/syncengine"
)

// serveReports is B's sync-event route on a real server, every request coming
// from A's address.
func (f *raceFixture) serveReports(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.RemoteAddr = f.peer.Address + ":40000"
		rc := chi.NewRouteContext()
		rc.URLParams.Add("gameId", f.gameID)
		f.e.handleSyncEvent(w, r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rc)))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func postReport(client *http.Client, url string, data map[string]any) error {
	body, _ := json.Marshal(map[string]any{"eventType": "sync-complete", "data": data})
	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// A report is answered before what it records is done. Recording waits for
// the database, which a snapshot or a sync can hold for seconds, and the peer
// gave up on the answer long before: the report was lost (upstream CI #385,
// #381). Here recording is held up for as long as the test likes, and the
// answer still comes at once.
func TestAReportIsAnsweredBeforeItIsRecorded(t *testing.T) {
	f := newRaceFixture(t, false)
	release := make(chan struct{})
	recorded := make(chan struct{})
	prev := refreshAfterPull
	refreshAfterPull = func(*Engine, string, syncengine.Peer) {
		<-release
		close(recorded)
	}
	t.Cleanup(func() { refreshAfterPull = prev })
	srv := f.serveReports(t)

	client := &http.Client{Timeout: time.Second}
	err := postReport(client, srv.URL, map[string]any{"pulledFiles": []string{"slot1.sav"}, "reportId": "r1"})
	close(release)
	if err != nil {
		t.Fatalf("the report was not answered while it was being recorded: %v", err)
	}
	select {
	case <-recorded:
	case <-time.After(10 * time.Second):
		t.Fatal("the report was answered but never recorded")
	}
}

// A report sent again because its answer was lost is recorded once: the
// sender sends it with the same ID until it is answered. One without an ID,
// from an older build, is recorded every time, as before.
func TestAReportSentAgainIsRecordedOnce(t *testing.T) {
	f := newRaceFixture(t, false)
	var records atomic.Int32
	prev := refreshAfterPull
	refreshAfterPull = func(*Engine, string, syncengine.Peer) { records.Add(1) }
	t.Cleanup(func() { refreshAfterPull = prev })

	took := map[string]any{"pulledFiles": []string{"slot1.sav"}}
	f.reportOverLAN(t, map[string]any{"pulledFiles": took["pulledFiles"], "reportId": "same"})
	f.reportOverLAN(t, map[string]any{"pulledFiles": took["pulledFiles"], "reportId": "same"})
	if n := records.Load(); n != 1 {
		t.Errorf("a report sent twice was recorded %d times, want once", n)
	}
	f.reportOverLAN(t, map[string]any{"pulledFiles": took["pulledFiles"], "reportId": "other"})
	f.reportOverLAN(t, took)
	f.reportOverLAN(t, took)
	if n := records.Load(); n != 4 {
		t.Errorf("recorded %d times in all, want 4: another report, and two without an ID", n)
	}
}

// The "agreed" report carries state as "sync-complete" does: a device that
// misses it keeps a base a round behind. It is sent again until answered.
func TestTheAgreedReportCarriesState(t *testing.T) {
	for _, ev := range []string{"sync-complete", "in-sync", "agreed"} {
		if !carriesState(ev) {
			t.Errorf("%q is sent once, like a progress report", ev)
		}
	}
	if carriesState("sync-progress") {
		t.Error("a progress report is sent again")
	}
}
