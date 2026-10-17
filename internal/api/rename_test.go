package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

// Renaming a game changes what this device calls it, and nothing else: the ID
// that sync, snapshots and cloud copies go by stays, and a name no screen can
// show sensibly is refused.
func TestRenameGame(t *testing.T) {
	ts := startTestServer(t)
	resp, raw := ts.do(t, http.MethodPost, "/api/games",
		map[string]string{"name": "Saves", "savePath": ts.saveDir})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("track: %d", resp.StatusCode)
	}
	var id string
	if err := json.Unmarshal(raw["id"], &id); err != nil {
		t.Fatal(err)
	}

	resp, _ = ts.do(t, http.MethodPatch, "/api/games/"+id, map[string]any{"name": "  Hollow Knight  "})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("rename: %d", resp.StatusCode)
	}
	game, err := ts.daemon.Store.GetGame(id)
	if err != nil {
		t.Fatalf("the game is no longer under its ID %q: %v", id, err)
	}
	if game.Name != "Hollow Knight" {
		t.Errorf("name = %q, want the trimmed %q", game.Name, "Hollow Knight")
	}

	for why, name := range map[string]string{
		"empty":        "   ",
		"a line break": "Hollow\nKnight",
	} {
		resp, _ = ts.do(t, http.MethodPatch, "/api/games/"+id, map[string]any{"name": name})
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: rename answered %d, want 400", why, resp.StatusCode)
		}
	}
	if game, _ := ts.daemon.Store.GetGame(id); game.Name != "Hollow Knight" {
		t.Errorf("a refused rename changed the name to %q", game.Name)
	}
}
