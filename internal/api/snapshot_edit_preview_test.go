package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// trackedWithSnapshot tracks a game and waits for its first snapshot.
func trackedWithSnapshot(t *testing.T, ts *testServer, name, gameID string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(ts.saveDir, "slot1.sav"), []byte("first"), 0o644); err != nil {
		t.Fatal(err)
	}
	resp, body := ts.do(t, http.MethodPost, "/api/games", map[string]any{"name": name, "savePath": ts.saveDir})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("track: %d %s", resp.StatusCode, body)
	}
	var id string
	if !waitFor(20*time.Second, func() bool {
		snaps, _ := ts.daemon.Store.ListSnapshots(gameID, "main")
		if len(snaps) == 1 {
			id = snaps[0].ID
		}
		return id != ""
	}) {
		t.Fatal("setup: the initial snapshot never landed")
	}
	return id
}

func TestAPI_EditSnapshot(t *testing.T) {
	ts := startTestServer(t)
	snapID := trackedWithSnapshot(t, ts, "Edited", "edited")
	path := "/api/games/edited/snapshot/" + snapID

	resp, body := ts.do(t, http.MethodPatch, path, map[string]any{"pinned": true, "note": "  before the boss "})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("pin + note: %d %s", resp.StatusCode, body)
	}
	var got struct {
		Pinned bool   `json:"pinned"`
		Note   string `json:"note"`
	}
	decode := func(body map[string]json.RawMessage) {
		raw, _ := json.Marshal(body)
		_ = json.Unmarshal(raw, &got)
	}
	decode(body)
	if !got.Pinned || got.Note != "before the boss" {
		t.Errorf("answer = %+v, want it pinned with the note trimmed", got)
	}

	// A field left out is left alone.
	resp, body = ts.do(t, http.MethodPatch, path, map[string]any{"pinned": false})
	decode(body)
	if resp.StatusCode != http.StatusOK || got.Pinned || got.Note != "before the boss" {
		t.Errorf("unpinning: %d %+v — the note must survive an edit that does not mention it", resp.StatusCode, got)
	}

	for _, c := range []struct {
		name string
		path string
		body any
		want int
	}{
		{"nothing to change", path, map[string]any{}, http.StatusBadRequest},
		{"a note past the limit", path, map[string]any{"note": string(make([]byte, 501))}, http.StatusBadRequest},
		{"no such snapshot", "/api/games/edited/snapshot/snap_nope", map[string]any{"pinned": true}, http.StatusNotFound},
		{"another game's snapshot", "/api/games/someone-else/snapshot/" + snapID, map[string]any{"pinned": true}, http.StatusNotFound},
	} {
		if resp, body := ts.do(t, http.MethodPatch, c.path, c.body); resp.StatusCode != c.want {
			t.Errorf("%s: %d %s, want %d", c.name, resp.StatusCode, body, c.want)
		}
	}
}

func TestAPI_PreviewRestore(t *testing.T) {
	ts := startTestServer(t)
	snapID := trackedWithSnapshot(t, ts, "Previewed", "previewed")
	if err := os.WriteFile(filepath.Join(ts.saveDir, "slot1.sav"), []byte("second, longer"), 0o644); err != nil {
		t.Fatal(err)
	}

	resp, body := ts.do(t, http.MethodGet, "/api/games/previewed/snapshot/"+snapID+"/preview", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("preview: %d %s", resp.StatusCode, body)
	}
	var changes []struct {
		Path   string `json:"path"`
		Change string `json:"change"`
	}
	if err := json.Unmarshal(body["changes"], &changes); err != nil || len(changes) != 1 ||
		changes[0].Path != "slot1.sav" || changes[0].Change != "changed" {
		t.Errorf("preview changes = %s (%v), want slot1.sav changed", body["changes"], err)
	}
	// And it only looked.
	if b, _ := os.ReadFile(filepath.Join(ts.saveDir, "slot1.sav")); string(b) != "second, longer" {
		t.Errorf("previewing changed the save: %q", b)
	}
	if resp, _ := ts.do(t, http.MethodGet, "/api/games/previewed/snapshot/snap_nope/preview", nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("preview of a snapshot that does not exist: %d, want 404", resp.StatusCode)
	}
}
