package p2p

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/opensave/opensave/internal/delta"
	"github.com/opensave/opensave/internal/p2p/syncengine"
	"github.com/opensave/opensave/internal/snapshot"
	"github.com/opensave/opensave/internal/store"
)

// A deletion made on one device reaches another as one request per file,
// and the receiving device's own syncs carry on meanwhile. One that lands
// between two of those requests sees its save part-way through the other
// device's deletions: its own copy no longer the state the two agreed on, and
// the other device's not either. That read as both sides having changed —
// a conflict that stopped the game syncing until someone resolved it by
// hand — though nothing had changed on the receiving device but the other
// device's own deletions.
//
// These drive the receiving side's real handlers: the requests are the ones
// device A sends; the sync in between is device B's own.

// deviceA stands in for the device doing the deleting: it answers B's
// manifest requests from a folder, and serves its blocks.
type deviceA struct {
	dir   string
	extra map[string]string // its extra save locations, by name

	mu      sync.Mutex
	reports []sentReport // what B told it about syncs, in order
}

// sentReport is one sync-event B sent to A.
type sentReport struct {
	eventType string
	data      map[string]any
}

func (a *deviceA) base(root string) string {
	if root == "" {
		return a.dir
	}
	return a.extra[root]
}

func (a *deviceA) FetchManifest(ctx context.Context, peer syncengine.Peer, gameID string, q syncengine.ManifestQuery) (syncengine.ManifestResponse, error) {
	m, _, err := delta.BuildMultiManifest(a.dir, a.extra)
	if err != nil {
		return syncengine.ManifestResponse{}, err
	}
	return syncengine.ManifestResponse{Manifest: m, ActiveBranch: "main", Proto: syncengine.ProtoMultiRoot}, nil
}

func (a *deviceA) FetchBlocks(ctx context.Context, peer syncengine.Peer, ref syncengine.FileRef, blockIndices []int, blockSize int) ([]syncengine.BlockData, error) {
	full := filepath.Join(a.base(ref.Root), filepath.FromSlash(ref.RelPath))
	entry, err := delta.HashFile(full)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(full)
	if err != nil {
		return nil, err
	}
	var out []syncengine.BlockData
	for _, i := range blockIndices {
		if i >= len(entry.Blocks) {
			continue
		}
		start := i * entry.BlockSize
		end := min(start+entry.Blocks[i].Length, len(raw))
		out = append(out, syncengine.BlockData{Index: i, Data: raw[start:end], Length: end - start})
	}
	return out, nil
}

func (a *deviceA) DeleteRemote(ctx context.Context, peer syncengine.Peer, ref syncengine.FileRef) error {
	return os.RemoveAll(filepath.Join(a.base(ref.Root), filepath.FromSlash(ref.RelPath)))
}
func (a *deviceA) TriggerPeerPull(peer syncengine.Peer, gameID string) {}
func (a *deviceA) ReportSyncEvent(peer syncengine.Peer, gameID, eventType string, data map[string]any) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.reports = append(a.reports, sentReport{eventType, data})
}

type raceFixture struct {
	e      *Engine
	s      *store.Store
	a      *deviceA
	bDir   string
	peer   syncengine.Peer
	gameID string
	extraB string // B's folder for the "config" location, when there is one
	extraA string
}

// newRaceFixture is device B, tracking a game device A also has, the two in
// sync: three save files each, recorded as agreed.
func newRaceFixture(t *testing.T, withExtra bool) *raceFixture {
	t.Helper()
	e, s := newMatchTestEngine(t)
	root := t.TempDir()
	f := &raceFixture{e: e, s: s, a: &deviceA{dir: filepath.Join(root, "a")}, bDir: filepath.Join(root, "b"), gameID: "race-game"}
	f.peer = syncengine.Peer{ID: "node_a", Name: "Device A", Address: "192.0.2.10", Port: 1}
	if err := s.CreateGame(store.Game{ID: f.gameID, Name: "Race Game", SavePath: f.bDir, ActiveBranch: "main", AutoSync: true, MaxSnapshots: 20}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertPeer(store.Peer{ID: f.peer.ID, Name: f.peer.Name, Address: f.peer.Address, Port: f.peer.Port, Status: "online"}); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{f.a.dir, f.bDir} {
		for name, body := range map[string]string{"slot1.sav": "one", "slot2.sav": "two", "slot3.sav": "three"} {
			writeFile(t, filepath.Join(dir, name), body)
		}
	}
	if withExtra {
		f.extraA, f.extraB = filepath.Join(root, "a-config"), filepath.Join(root, "b-config")
		f.a.extra = map[string]string{"config": f.extraA}
		for _, dir := range []string{f.extraA, f.extraB} {
			for name, body := range map[string]string{"a.cfg": "x", "b.cfg": "y", "c.cfg": "z"} {
				writeFile(t, filepath.Join(dir, name), body)
			}
		}
		if err := s.AddGameRoot(f.gameID, "config", f.extraB); err != nil {
			t.Fatal(err)
		}
	}
	e.Sync = syncengine.New(s, snapshot.New(s), f.a)
	res, err := e.Sync.SyncWithPeer(context.Background(), f.gameID, f.peer)
	if err != nil || res.Status != "in_sync" {
		t.Fatalf("setup: the two devices are not in sync: %+v, %v", res, err)
	}
	if s.GetAgreedHash(f.gameID, f.peer.ID) == "" {
		t.Fatal("setup: no agreed state recorded")
	}
	return f
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o666); err != nil {
		t.Fatal(err)
	}
}

// deleteRequest is one of A's deletion requests arriving at B's LAN route.
func (f *raceFixture) deleteRequest(t *testing.T, rel, root string) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"relPath": rel, "root": root})
	r := httptest.NewRequest(http.MethodPost, "/api/p2p/delete-file/"+f.gameID, bytes.NewReader(body))
	r.RemoteAddr = "198.51.100.7:40000" // not a paired device's address: no lineage refresh races the test
	rc := chi.NewRouteContext()
	rc.URLParams.Add("gameId", f.gameID)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rc))
	w := httptest.NewRecorder()
	f.e.handleDeleteFile(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("delete request for %s: %d %s", rel, w.Code, w.Body.String())
	}
}

func (f *raceFixture) has(dir, rel string) bool {
	_, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel)))
	return err == nil
}

// B's sync, landing between A's two deletions, finishes them instead of
// taking them for a change of B's own.
func TestASyncBetweenTwoPeerDeletionsIsNotAConflict(t *testing.T) {
	f := newRaceFixture(t, false)

	// A deletes two of the three.
	os.Remove(filepath.Join(f.a.dir, "slot1.sav"))
	os.Remove(filepath.Join(f.a.dir, "slot2.sav"))

	f.deleteRequest(t, "slot1.sav", "") // the first of A's requests lands
	res, err := f.e.Sync.SyncWithPeer(context.Background(), f.gameID, f.peer)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status == "conflict" || len(f.e.Sync.ActiveConflicts()) > 0 {
		t.Fatalf("B's sync between A's deletions = %q: a conflict nobody caused", res.Status)
	}
	f.deleteRequest(t, "slot2.sav", "") // the second lands afterwards

	if f.has(f.bDir, "slot1.sav") || f.has(f.bDir, "slot2.sav") || !f.has(f.bDir, "slot3.sav") {
		t.Error("B does not hold what A holds: slot3.sav alone")
	}
	res, err = f.e.Sync.SyncWithPeer(context.Background(), f.gameID, f.peer)
	if err != nil || res.Status != "in_sync" {
		t.Errorf("the next sync = %+v, %v; want in_sync", res, err)
	}
}

// The same over the relay, whose requests are slower, so the window wider.
func TestASyncBetweenTwoRelayedDeletionsIsNotAConflict(t *testing.T) {
	f := newRaceFixture(t, false)
	w := &WanClient{engine: f.e}
	os.Remove(filepath.Join(f.a.dir, "slot1.sav"))
	os.Remove(filepath.Join(f.a.dir, "slot2.sav"))

	body, _ := json.Marshal(map[string]string{"relPath": "slot1.sav"})
	if code, out := w.serveDeleteFile("/delete-file/"+f.gameID, body, "node_unknown"); code != 200 {
		t.Fatalf("relayed delete: %d %v", code, out)
	}
	res, err := f.e.Sync.SyncWithPeer(context.Background(), f.gameID, f.peer)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status == "conflict" {
		t.Fatalf("B's sync between relayed deletions = %q", res.Status)
	}
}

// The same in a game's extra save location, which has an agreed state of its
// own.
func TestASyncBetweenTwoPeerDeletionsInAnExtraLocationIsNotAConflict(t *testing.T) {
	f := newRaceFixture(t, true)
	os.Remove(filepath.Join(f.a.dir, "slot1.sav")) // keeps A's manifest changing in the main folder too
	os.Remove(filepath.Join(f.extraA, "a.cfg"))
	os.Remove(filepath.Join(f.extraA, "b.cfg"))
	f.deleteRequest(t, "slot1.sav", "")
	f.deleteRequest(t, "a.cfg", "config")
	if _, err := f.e.Sync.SyncWithPeer(context.Background(), f.gameID, f.peer); err != nil {
		t.Fatal(err)
	}
	if n := len(f.e.Sync.ActiveRootConflicts()); n > 0 {
		t.Fatalf("%d conflict(s) in the extra location, from A's deletions alone", n)
	}
	if f.has(f.extraB, "b.cfg") || !f.has(f.extraB, "c.cfg") {
		t.Error("B's config location does not hold what A's does: c.cfg alone")
	}
}

// Something B really did change still counts: a save of B's own made while
// A's deletions arrive is a change on both sides, as before.
func TestAChangeOfBsOwnAmongPeerDeletionsIsStillAConflict(t *testing.T) {
	f := newRaceFixture(t, false)
	os.Remove(filepath.Join(f.a.dir, "slot1.sav"))
	os.Remove(filepath.Join(f.a.dir, "slot2.sav"))

	f.deleteRequest(t, "slot1.sav", "")
	writeFile(t, filepath.Join(f.bDir, "slot3.sav"), "B played on") // B's own save
	res, err := f.e.Sync.SyncWithPeer(context.Background(), f.gameID, f.peer)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "conflict" {
		t.Errorf("B changed slot3.sav and A deleted files: sync = %q, want a conflict", res.Status)
	}
}
