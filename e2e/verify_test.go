package e2e

import (
	"archive/zip"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/opensave/opensave/testutil"
)

// A snapshot whose archive has gone bad is found by a check, shown as such,
// and refused by a restore before the save is touched.
func TestVerify_ADamagedSnapshotIsFoundAndNotRestored(t *testing.T) {
	td := testutil.NewTestDaemon(t, "Verify")
	td.WriteSave("slot1.sav", strings.Repeat("a save worth keeping, ", 300))
	gameID := td.TrackGame("Verify Game")

	var snap struct {
		ID      string `json:"id"`
		ZipPath string `json:"zipPath"`
	}
	td.API(http.MethodPost, "/api/games/"+gameID+"/snapshot", map[string]string{"comment": "before the boss"}, &snap)
	if snap.ZipPath == "" {
		t.Fatal("setup: no snapshot")
	}

	// Every snapshot fine to begin with.
	var report struct {
		Checked int `json:"checked"`
		Damaged []struct {
			SnapshotID string `json:"snapshotId"`
			Problem    string `json:"problem"`
		} `json:"damaged"`
	}
	td.API(http.MethodPost, "/api/snapshots/check", nil, &report)
	if report.Checked == 0 || len(report.Damaged) != 0 {
		t.Fatalf("a fresh check: %+v, want everything checked and nothing damaged", report)
	}

	// A byte of the save's stored data goes bad on disk.
	r, err := zip.OpenReader(snap.ZipPath)
	if err != nil {
		t.Fatal(err)
	}
	at, _ := r.File[0].DataOffset()
	size := int64(r.File[0].CompressedSize64)
	r.Close()
	raw, err := os.ReadFile(snap.ZipPath)
	if err != nil {
		t.Fatal(err)
	}
	raw[at+size/2] ^= 0xFF
	if err := os.WriteFile(snap.ZipPath, raw, 0o666); err != nil {
		t.Fatal(err)
	}

	td.API(http.MethodPost, "/api/snapshots/check", nil, &report)
	if len(report.Damaged) != 1 || report.Damaged[0].SnapshotID != snap.ID {
		t.Fatalf("after the damage: %+v, want that snapshot found", report)
	}

	td.WriteSave("slot1.sav", "the save as it is now")
	if code := td.APIStatus(http.MethodPost, "/api/games/"+gameID+"/rollback", map[string]string{"snapshotId": snap.ID}, nil); code == http.StatusOK {
		t.Errorf("restoring the damaged snapshot was accepted")
	}
	if got := td.ReadSave("slot1.sav"); got != "the save as it is now" {
		t.Errorf("a refused restore changed the save: %q", got)
	}

	var games map[string]struct {
		Branches map[string]struct {
			Snapshots []struct {
				ID      string `json:"id"`
				Problem string `json:"problem"`
			} `json:"snapshots"`
		} `json:"branches"`
	}
	td.API(http.MethodGet, "/api/games", nil, &games)
	shown := false
	for _, b := range games[gameID].Branches {
		for _, s := range b.Snapshots {
			shown = shown || (s.ID == snap.ID && s.Problem != "")
		}
	}
	if !shown {
		t.Errorf("the game's snapshot does not say it is damaged")
	}
}
