package p2p

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/opensave/opensave/internal/store"
)

// A paired device that has proved who it is asks; this device answers with
// its own sessions from the mark, oldest first.
func TestPlaySessions_ASignedPeerIsToldThisDevicesSessions(t *testing.T) {
	f := newAuthFixture(t, true)
	trackForTest(t, f.store, "elden")
	for _, s := range [][2]int64{{100, 200}, {300, 400}} {
		if err := f.store.AddPlaySession("elden", s[0], s[1]); err != nil {
			t.Fatal(err)
		}
	}
	r := chi.NewRouter()
	f.engine.RegisterRoutes(r)

	body := []byte(`{"since":300}`)
	req := f.lanRequest(t, http.MethodPost, "/api/p2p/play-sessions", body)
	req.RemoteAddr = "192.0.2.10:40000"
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("HTTP %d: %s", rec.Code, rec.Body.String())
	}
	var reply playSessionsReply
	if err := json.Unmarshal(rec.Body.Bytes(), &reply); err != nil {
		t.Fatal(err)
	}
	if len(reply.Sessions) != 1 || reply.Sessions[0].EndedMs != 400 || reply.More {
		t.Fatalf("reply = %+v, want the one session ending at 400", reply)
	}
}

// Who played what, and when, is not given to a request that cannot prove
// where it came from.
func TestPlaySessions_AnUnsignedRequestIsRefused(t *testing.T) {
	f := newAuthFixture(t, true)
	peer := f.peer
	peer.Address = "192.0.2.10"
	if err := f.store.UpdatePeer(peer); err != nil {
		t.Fatal(err)
	}
	r := chi.NewRouter()
	f.engine.RegisterRoutes(r)
	req := httptest.NewRequest(http.MethodPost, "/api/p2p/play-sessions", nil)
	req.RemoteAddr = "192.0.2.10:40000"
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("an unsigned request from the paired address was answered: HTTP %d", rec.Code)
	}
}

func TestPlaySessions_OverTheRelayNeedsAProvenPairing(t *testing.T) {
	keyless := newAuthFixture(t, false)
	w := &WanClient{engine: keyless.engine}
	if status, _ := w.routeRequest(t.Context(), RelayMessage{Type: "request", From: keyless.peer.ID, Route: playSessionsRoute, Method: http.MethodPost}); status != http.StatusUnauthorized {
		t.Fatalf("a keyless pairing was answered: HTTP %d, want 401", status)
	}
	stranger := newAuthFixture(t, true)
	w = &WanClient{engine: stranger.engine}
	if status, _ := w.routeRequest(t.Context(), RelayMessage{Type: "request", From: "someone-else", Route: playSessionsRoute, Method: http.MethodPost}); status != http.StatusUnauthorized {
		t.Fatalf("an unpaired device was answered: HTTP %d, want 401", status)
	}

	f := newAuthFixture(t, true)
	trackForTest(t, f.store, "elden")
	if err := f.store.AddPlaySession("elden", 100, 200); err != nil {
		t.Fatal(err)
	}
	w = &WanClient{engine: f.engine}
	status, data := w.routeRequest(t.Context(), f.signedRequest(playSessionsRoute))
	if status != http.StatusOK {
		t.Fatalf("a signed request from the paired device: HTTP %d (%v)", status, data)
	}
	if reply, ok := data.(playSessionsReply); !ok || len(reply.Sessions) != 1 {
		t.Fatalf("reply = %#v, want this device's one session", data)
	}
}

// fakePeer is a paired device on the LAN that answers the play-sessions
// route from a list, a page at a time, as a real one does.
type fakePeer struct {
	sessions []store.PlaySession
	status   int
	asked    atomic.Int32
}

func (p *fakePeer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.asked.Add(1)
	if p.status != 0 {
		http.Error(w, "404 page not found", p.status)
		return
	}
	var body playSessionsRequest
	_ = json.NewDecoder(r.Body).Decode(&body)
	out := []store.PlaySession{}
	for _, s := range p.sessions {
		if s.EndedMs >= body.Since && len(out) < playSessionsPage {
			out = append(out, s)
		}
	}
	_ = json.NewEncoder(w).Encode(playSessionsReply{Sessions: out, More: len(out) == playSessionsPage})
}

func newPullFixture(t *testing.T, peer *fakePeer) (*Engine, *store.Store) {
	t.Helper()
	playSkips = &playUnsupported{until: map[string]time.Time{}}
	srv := httptest.NewServer(peer)
	t.Cleanup(srv.Close)
	host, portText, _ := net.SplitHostPort(srv.Listener.Addr().String())
	port, _ := strconv.Atoi(portText)

	s, err := store.Open(filepath.Join(t.TempDir(), "opensave.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.EnsureDefaultSettings(t.TempDir(), t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertPeer(store.Peer{ID: "deck", Name: "Steam Deck", DeviceType: "deck", Address: host, Port: port, Status: "online"}); err != nil {
		t.Fatal(err)
	}
	return &Engine{Store: s, Log: func(string, string) {}}, s
}

func TestPullPlaySessions_PagesThroughAndKeepsEachOnce(t *testing.T) {
	peer := &fakePeer{}
	for i := int64(1); i <= 750; i++ {
		peer.sessions = append(peer.sessions, store.PlaySession{GameID: "elden", StartedMs: i * 1000, EndedMs: i*1000 + 500})
	}
	e, s := newPullFixture(t, peer)

	if got := e.PullPlaySessions(context.Background()); got != 750 {
		t.Fatalf("first pull kept %d, want all 750 across two pages", got)
	}
	if got := e.PullPlaySessions(context.Background()); got != 0 {
		t.Fatalf("second pull kept %d new, want 0: only the one at the mark comes again", got)
	}
	stats, _ := s.PeerPlayStatsByGame()
	if st := stats["elden"]["deck"]; st.Sessions != 750 || st.PlaytimeMs != 750*500 {
		t.Fatalf("kept %+v, want 750 sessions of 500 ms", st)
	}
}

func TestPullPlaySessions_AnOlderDeviceIsAskedAgainOnlyLater(t *testing.T) {
	peer := &fakePeer{status: http.StatusNotFound}
	e, _ := newPullFixture(t, peer)
	e.PullPlaySessions(context.Background())
	e.PullPlaySessions(context.Background())
	if n := peer.asked.Load(); n != 1 {
		t.Fatalf("a device without the route was asked %d times, want once until the hour is up", n)
	}
}

func TestPullPlaySessions_OnlyOnlineDevicesAreAsked(t *testing.T) {
	peer := &fakePeer{}
	e, s := newPullFixture(t, peer)
	p, _ := s.GetPeer("deck")
	p.Status = "offline"
	if err := s.UpdatePeer(p); err != nil {
		t.Fatal(err)
	}
	e.PullPlaySessions(context.Background())
	if n := peer.asked.Load(); n != 0 {
		t.Fatalf("an offline device was asked %d times", n)
	}
}

func trackForTest(t *testing.T, s *store.Store, id string) {
	t.Helper()
	if err := s.CreateGame(store.Game{ID: id, Name: id, SavePath: filepath.Join(t.TempDir(), id), ActiveBranch: "main"}); err != nil {
		t.Fatal(err)
	}
}
