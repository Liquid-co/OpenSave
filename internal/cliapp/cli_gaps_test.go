package cliapp

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// stubDaemon answers the CLI's requests with canned JSON, and remembers every
// request it is sent, body included.
type stubDaemon struct {
	mu   sync.Mutex
	seen []stubRequest
}

type stubRequest struct {
	Method, Path string
	Body         map[string]any
}

func (s *stubDaemon) requests(method, path string) []stubRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []stubRequest
	for _, r := range s.seen {
		if r.Method == method && r.Path == path {
			out = append(out, r)
		}
	}
	return out
}

// startStubDaemon points the CLI at a stub answering each "METHOD path" with
// the status and body given.
func startStubDaemon(t *testing.T, answers map[string]struct {
	status int
	body   string
}) *stubDaemon {
	t.Helper()
	stub := &stubDaemon{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		stub.mu.Lock()
		stub.seen = append(stub.seen, stubRequest{r.Method, r.URL.Path, body})
		stub.mu.Unlock()
		a, ok := answers[r.Method+" "+r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(a.status)
		_, _ = w.Write([]byte(a.body))
	}))
	t.Cleanup(srv.Close)

	home := t.TempDir()
	for _, v := range []string{"HOME", "USERPROFILE"} {
		t.Setenv(v, home)
	}
	dir := filepath.Join(home, ".opensave")
	if err := os.MkdirAll(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "daemon.addr"), []byte(strings.TrimPrefix(srv.URL, "http://")), 0o666); err != nil {
		t.Fatal(err)
	}
	return stub
}

type answer = struct {
	status int
	body   string
}

// A sync that did not happen says why; one that did says what it did. The
// daemon's reason is used when it gives one, and an older daemon's message
// otherwise.
func TestSyncOutcomes(t *testing.T) {
	for raw, want := range map[string]string{
		`{"status":"error","error":"syncing is paused","reason":"paused"}`:                                  "paused",
		`{"status":"error","error":"no online peers available"}`:                                            "offline",
		`{"status":"error","error":"its save files were all deleted here; it is not synced until you say"}`: "held",
		`{"status":"error","error":"disk full","reason":"error"}`:                                           "error",
		`{"status":"queued"}`:                                                       "queued",
		`{"status":"skipped","reason":"autoSync disabled"}`:                         "skipped",
		`{"p1":{"status":"in_sync","peerName":"Deck"}}`:                             "in-sync",
		`{"p1":{"status":"in_sync"},"p2":{"status":"updated","peerName":"Laptop"}}`: "changed",
		`{"p1":{"status":"conflict","peerName":"Deck"}}`:                            "conflict",
		`{"p1":{"status":"peer_awaiting_folder","peerName":"Deck"}}`:                "waiting",
		`{}`: "in-sync",
	} {
		if got := syncOutcomeOf("g", json.RawMessage(raw)).Kind; got != want {
			t.Errorf("%s -> %q, want %q", raw, got, want)
		}
	}

	s := summarizeSync([]syncOutcome{{Kind: "paused"}, {Kind: "paused"}, {Kind: "skipped"}})
	if s.nothingSynced() != "paused" {
		t.Errorf("all paused: %q", s.nothingSynced())
	}
	s = summarizeSync([]syncOutcome{{Kind: "offline"}, {Kind: "offline"}})
	if s.nothingSynced() != "offline" {
		t.Errorf("all offline: %q", s.nothingSynced())
	}
	s = summarizeSync([]syncOutcome{{Kind: "changed"}, {Kind: "in-sync"}, {Kind: "in-sync"}, {Kind: "offline"}})
	if s.nothingSynced() != "" || s.headline() != "Synced 3 games: 1 updated, 2 already in sync." {
		t.Errorf("mixed: %q / %q", s.nothingSynced(), s.headline())
	}
}

// `sync --all` used to print "Sync started" whatever happened. Now nothing
// synced is an exit code a script can see.
func TestSyncAllSaysWhenNothingSynced(t *testing.T) {
	startStubDaemon(t, map[string]answer{
		"POST /api/games/sync-all": {200, `{"results":{"hades":{"status":"error","error":"no online peers available","reason":"offline"}}}`},
	})
	if code := cmdSync([]string{"--all"}); code != 1 {
		t.Errorf("exit %d with no device online, want 1", code)
	}
}

func TestSyncOneSaysWhyNot(t *testing.T) {
	startStubDaemon(t, map[string]answer{
		"POST /api/games/hades/sync": {409, `{"error":"syncing is paused","reason":"paused"}`},
	})
	if code := cmdSync([]string{"hades"}); code != 1 {
		t.Errorf("exit %d while paused, want 1", code)
	}
}

const statusWithLocations = `{"conflicts":{},"locationConflicts":[
  {"gameId":"hades","root":"Config","peer":{"id":"peer-1","name":"Deck"},"diffTotal":2},
  {"gameId":"celeste","root":"Mods","peer":{"id":"peer-2","name":"Laptop"}},
  {"gameId":"celeste","root":"Replays","peer":{"id":"peer-2","name":"Laptop"}}]}`

// A diverged save folder is settled with --location, and sent with the peer
// id and folder the conflict names — the person types neither.
func TestResolveLocation(t *testing.T) {
	stub := startStubDaemon(t, map[string]answer{
		"GET /api/status": {200, statusWithLocations},
		"POST /api/games/hades/resolve-location-conflict":   {200, `{"accepted":true}`},
		"POST /api/games/celeste/resolve-location-conflict": {200, `{"accepted":true}`},
	})

	if code := cmdResolve([]string{"celeste", "keep-remote", "--location", "replays"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	got := stub.requests("POST", "/api/games/celeste/resolve-location-conflict")
	if len(got) != 1 || got[0].Body["peerId"] != "peer-2" || got[0].Body["root"] != "Replays" || got[0].Body["resolution"] != "keep-remote" {
		t.Errorf("sent %+v", got)
	}

	// The only folder in conflict needs no --location.
	if code := cmdResolve([]string{"hades", "keep-local"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if got := stub.requests("POST", "/api/games/hades/resolve-location-conflict"); len(got) != 1 || got[0].Body["root"] != "Config" {
		t.Errorf("sent %+v", got)
	}

	// Two of them: which one must be said. And a folder cannot keep both.
	if code := cmdResolve([]string{"celeste", "keep-local"}); code == 0 {
		t.Error("resolved one of two folders without being told which")
	}
	if code := cmdResolve([]string{"hades", "keep-both", "--location", "Config"}); code == 0 {
		t.Error("keep-both accepted for a save folder")
	}
	if n := len(stub.requests("POST", "/api/games/celeste/resolve-location-conflict")); n != 1 {
		t.Errorf("%d requests to resolve celeste's folders, want the 1 from before", n)
	}
}

// A game offered by another device can be placed in a folder, or declined.
func TestOffersPlaceAndDecline(t *testing.T) {
	stub := startStubDaemon(t, map[string]answer{
		"GET /api/offered-games":                {200, `[{"gameId":"hades","peerId":"p","name":"Hades","peerPath":"C:/x"}]`},
		"GET /api/peers":                        {200, `{"peers":{"p":{"name":"Deck"}}}`},
		"GET /api/settings":                     {200, `{"unknownGameFromPeer":"ask"}`},
		"POST /api/offered-games/hades/place":   {200, `{"id":"hades","name":"Hades"}`},
		"POST /api/offered-games/hades/decline": {200, `{"declined":"hades"}`},
	})
	if code := cmdOffers(nil); code != 0 {
		t.Errorf("listing exited %d", code)
	}
	folder := t.TempDir()
	if code := cmdOffers([]string{"place", "hades", folder}); code != 0 {
		t.Fatalf("place exited %d", code)
	}
	if got := stub.requests("POST", "/api/offered-games/hades/place"); len(got) != 1 || got[0].Body["path"] != folder {
		t.Errorf("sent %+v, want the folder %s", got, folder)
	}
	if code := cmdOffers([]string{"decline", "hades"}); code != 0 {
		t.Fatalf("decline exited %d", code)
	}
	if len(stub.requests("POST", "/api/offered-games/hades/decline")) != 1 {
		t.Error("decline was not sent")
	}
	if code := cmdOffers([]string{"place", "hades"}); code == 0 {
		t.Error("place without a folder succeeded")
	}
}
