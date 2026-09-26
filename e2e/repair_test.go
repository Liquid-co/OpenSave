package e2e

import (
	"context"
	"crypto/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/opensave/opensave/testutil"
)

type checkReport struct {
	Checked  int `json:"checked"`
	Repaired int `json:"repaired"`
	Damaged  []struct {
		SnapshotID string `json:"snapshotId"`
	} `json:"damaged"`
}

// waitUploaded waits until a snapshot's upload has finished, not merely begun.
//
// The local provider creates the file under its final name and then copies
// into it, so the name turning up in the folder means the copy has started.
// Until it ends the archive here is still open — Windows refuses to delete it,
// which is how this failed in CI — and the copy there is still short.
func waitUploaded(t *testing.T, td *testutil.TestDaemon, dir, snapID string) {
	t.Helper()
	if !testutil.WaitFor(30*time.Second, func() bool {
		for _, e := range td.Daemon.Log.History() {
			if e.Level == "success" && strings.Contains(e.Message, "cloud: uploaded") &&
				strings.Contains(e.Message, snapID) {
				return true
			}
		}
		return false
	}) {
		t.Fatalf("setup: the snapshot never finished reaching the cloud: %v", cloudFiles(t, dir))
	}
}

// A snapshot whose archive went missing here, with its cloud copy whole, is
// put back from the cloud by the check itself — and can be restored again.
func TestRepair_AMissingArchiveComesBackFromTheCloud(t *testing.T) {
	td := testutil.NewTestDaemon(t, "Repair")
	dir := useLocalCloud(t, td)
	td.WriteSave("slot1.sav", "worth keeping")
	gameID := td.TrackGame("Repair Game")
	var snap struct {
		ID      string `json:"id"`
		ZipPath string `json:"zipPath"`
	}
	td.API(http.MethodPost, "/api/games/"+gameID+"/snapshot", map[string]string{"comment": "keep"}, &snap)
	waitUploaded(t, td, dir, snap.ID)

	// The archive goes, here only.
	if err := os.Remove(snap.ZipPath); err != nil {
		t.Fatal(err)
	}
	var report checkReport
	td.API(http.MethodPost, "/api/snapshots/check", nil, &report)
	if report.Repaired != 1 || len(report.Damaged) != 0 {
		t.Fatalf("check = %+v; want it found and put back from the cloud", report)
	}
	if _, err := os.Stat(snap.ZipPath); err != nil {
		t.Fatal("the archive is not back")
	}
	td.WriteSave("slot1.sav", "changed since")
	if code := td.APIStatus(http.MethodPost, "/api/games/"+gameID+"/rollback", map[string]string{"snapshotId": snap.ID}, nil); code != http.StatusOK {
		t.Fatalf("restoring the repaired snapshot: %d", code)
	}
	if got := td.ReadSave("slot1.sav"); got != "worth keeping" {
		t.Errorf("restored %q", got)
	}
}

// A compacted snapshot (snapshot/shared.go) is damaged when a shared file it
// names is gone. Its cloud copy was uploaded whole, and it comes back whole:
// what reads it gets the archive exactly as it was taken, and the next
// compaction shares its files again rather than tripping over the old list.
func TestRepair_ACompactedSnapshotComesBackWhole(t *testing.T) {
	td := testutil.NewTestDaemon(t, "RepairShared")
	dir := useLocalCloud(t, td)
	slot := func() string {
		b := make([]byte, 40<<10)
		rand.Read(b)
		return string(b)
	}
	td.WriteSave("slot1.sav", slot())
	td.WriteSave("slot2.sav", slot())
	gameID := td.TrackGame("Repair Shared Game")
	type snapT struct {
		ID      string `json:"id"`
		ZipPath string `json:"zipPath"`
	}
	var old snapT
	td.API(http.MethodPost, "/api/games/"+gameID+"/snapshot", map[string]string{"comment": "old"}, &old)
	want := zipContents(t, readFile(t, old.ZipPath))
	oldSlot1 := td.ReadSave("slot1.sav")
	td.WriteSave("slot2.sav", slot())
	td.API(http.MethodPost, "/api/games/"+gameID+"/snapshot", map[string]string{"comment": "newer"}, nil)
	waitUploaded(t, td, dir, old.ID)
	if _, err := td.Daemon.Snapshots.CompactAll(context.Background(), 0, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old.ZipPath); err == nil {
		t.Fatal("setup: the older snapshot was not compacted")
	}

	// Every shared file goes.
	shared := filepath.Join(filepath.Dir(filepath.Dir(old.ZipPath)), ".shared")
	if err := os.RemoveAll(shared); err != nil {
		t.Fatal(err)
	}
	var report checkReport
	td.API(http.MethodPost, "/api/snapshots/check", nil, &report)
	if report.Repaired == 0 || len(report.Damaged) != 0 {
		t.Fatalf("check = %+v; want the compacted ones (the first snapshot, taken on tracking, too) found and put back", report)
	}
	sameContents(t, "the repaired snapshot", zipContents(t, readFile(t, old.ZipPath)), want)

	// Shared again, by the next pass, and still whole after.
	if _, err := td.Daemon.Snapshots.CompactAll(context.Background(), 0, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old.ZipPath); err == nil {
		t.Error("the repaired snapshot was not compacted again")
	}
	td.API(http.MethodPost, "/api/snapshots/check", nil, &report)
	if len(report.Damaged) != 0 {
		t.Fatalf("after compacting again, %d damaged", len(report.Damaged))
	}
	td.WriteSave("slot1.sav", "changed since")
	if code := td.APIStatus(http.MethodPost, "/api/games/"+gameID+"/rollback", map[string]string{"snapshotId": old.ID}, nil); code != http.StatusOK {
		t.Fatalf("restoring the repaired snapshot: %d", code)
	}
	if td.ReadSave("slot1.sav") != oldSlot1 {
		t.Error("the restore did not put the snapshot's save back")
	}
}

// With no copy anywhere, the record can be let go: the snapshots that cannot
// be restored leave the history, and the rest stay.
func TestRepair_WhatCannotBeRepairedCanBeRemoved(t *testing.T) {
	td := testutil.NewTestDaemon(t, "RepairRemove")
	td.WriteSave("slot1.sav", "one")
	gameID := td.TrackGame("Repair Remove Game")
	type snapT struct {
		ID      string `json:"id"`
		ZipPath string `json:"zipPath"`
	}
	var lost, kept snapT
	td.API(http.MethodPost, "/api/games/"+gameID+"/snapshot", map[string]string{"comment": "lost"}, &lost)
	td.WriteSave("slot1.sav", "two")
	td.API(http.MethodPost, "/api/games/"+gameID+"/snapshot", map[string]string{"comment": "kept"}, &kept)
	if err := os.Remove(lost.ZipPath); err != nil {
		t.Fatal(err)
	}

	var report checkReport
	td.API(http.MethodPost, "/api/snapshots/check", nil, &report)
	if len(report.Damaged) != 1 || report.Damaged[0].SnapshotID != lost.ID {
		t.Fatalf("check = %+v; want the one missing found", report)
	}
	var repair struct {
		Repaired  []any `json:"repaired"`
		Remaining []any `json:"remaining"`
	}
	td.API(http.MethodPost, "/api/snapshots/repair", nil, &repair)
	if len(repair.Repaired) != 0 || len(repair.Remaining) != 1 {
		t.Errorf("repair with no cloud = %+v; want it left as it was", repair)
	}

	var removed struct {
		Removed []struct {
			SnapshotID string `json:"snapshotId"`
		} `json:"removed"`
	}
	td.API(http.MethodPost, "/api/snapshots/forget-damaged", nil, &removed)
	if len(removed.Removed) != 1 || removed.Removed[0].SnapshotID != lost.ID {
		t.Fatalf("removed %+v; want only the damaged one", removed)
	}
	var games map[string]struct {
		Branches map[string]struct {
			Snapshots []struct {
				ID string `json:"id"`
			} `json:"snapshots"`
		} `json:"branches"`
	}
	td.API(http.MethodGet, "/api/games", nil, &games)
	has := map[string]bool{}
	for _, b := range games[gameID].Branches {
		for _, s := range b.Snapshots {
			has[s.ID] = true
		}
	}
	if has[lost.ID] || !has[kept.ID] {
		t.Errorf("after removing: lost present %v, kept present %v", has[lost.ID], has[kept.ID])
	}
	td.API(http.MethodPost, "/api/snapshots/check", nil, &report)
	if len(report.Damaged) != 0 {
		t.Errorf("a check afterwards still finds %d damaged", len(report.Damaged))
	}
	if _, err := os.Stat(filepath.Dir(kept.ZipPath)); err != nil {
		t.Error("the game's snapshot folder went with it")
	}
}

// The schedule is a setting: weekly unless changed, and off when set to 0.
func TestRepair_CheckScheduleIsASetting(t *testing.T) {
	td := testutil.NewTestDaemon(t, "VerifySchedule")
	var s struct {
		VerifyEveryDays int `json:"verifyEveryDays"`
	}
	td.API(http.MethodGet, "/api/settings", nil, &s)
	if s.VerifyEveryDays != 7 {
		t.Errorf("verifyEveryDays = %d, want a week by default", s.VerifyEveryDays)
	}
	td.API(http.MethodPost, "/api/settings", map[string]any{"verifyEveryDays": 0}, nil)
	td.API(http.MethodGet, "/api/settings", nil, &s)
	if s.VerifyEveryDays != 0 {
		t.Errorf("verifyEveryDays = %d after turning it off", s.VerifyEveryDays)
	}
}
