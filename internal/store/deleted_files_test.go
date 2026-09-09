package store

import (
	"testing"
	"time"
)

func TestDeletedFileRoundTrip(t *testing.T) {
	s := openTestStore(t)

	if err := s.RecordDeletedFiles("game1", []DeletedFile{
		{Root: "", Path: "slot1.sav", Hash: "abc123"},
		{Root: "config", Path: "opts.ini", Hash: "def456"},
	}); err != nil {
		t.Fatal(err)
	}

	primary, err := s.DeletedFiles("game1", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(primary) != 1 {
		t.Fatalf("primary root has %d records, want 1", len(primary))
	}
	got := primary["slot1.sav"]
	if got.Hash != "abc123" {
		t.Errorf("hash = %q, want abc123 — without it the record cannot be checked "+
			"against the peer's copy and is unsafe to act on", got.Hash)
	}
	if got.DeletedAtMs == 0 {
		t.Error("deleted_at was not stamped")
	}

	// Roots are separate: the same relative path can exist in more than one.
	extra, _ := s.DeletedFiles("game1", "config")
	if len(extra) != 1 || extra["opts.ini"].Hash != "def456" {
		t.Errorf("the extra root's records are %+v", extra)
	}
}

// A record with no hash cannot be checked against the peer's copy, so acting
// on it would delete their file on faith. Better to have no record at all.
func TestDeletedFileWithoutAHashIsNotStored(t *testing.T) {
	s := openTestStore(t)
	if err := s.RecordDeletedFiles("game1", []DeletedFile{
		{Path: "nohash.sav", Hash: ""},
		{Path: "", Hash: "abc"},
		{Path: "good.sav", Hash: "abc"},
	}); err != nil {
		t.Fatal(err)
	}
	got, _ := s.DeletedFiles("game1", "")
	if _, bad := got["nohash.sav"]; bad {
		t.Error("a record with no hash was stored; it could only be acted on blindly")
	}
	if len(got) != 1 {
		t.Errorf("stored %d records, want only the usable one: %+v", len(got), got)
	}
}

// A file that comes back must stop being remembered as deleted, or a later
// sync would remove the peer's copy of a file this device now has.
func TestClearingARecordStopsItBeingRemembered(t *testing.T) {
	s := openTestStore(t)
	if err := s.RecordDeletedFiles("game1", []DeletedFile{{Path: "back.sav", Hash: "h"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.ClearDeletedFile("game1", "", "back.sav"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.DeletedFiles("game1", "")
	if len(got) != 0 {
		t.Errorf("the record survived being cleared: %+v", got)
	}
}

// Re-deleting a file records the newer content, not the older: the peer must
// be matched against what was actually there the last time.
func TestReDeletingUpdatesTheRecordedContent(t *testing.T) {
	s := openTestStore(t)
	older := time.Now().Add(-time.Hour).UnixMilli()
	if err := s.RecordDeletedFiles("game1", []DeletedFile{
		{Path: "slot.sav", Hash: "old", DeletedAtMs: older},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordDeletedFiles("game1", []DeletedFile{
		{Path: "slot.sav", Hash: "new"},
	}); err != nil {
		t.Fatal(err)
	}
	got, _ := s.DeletedFiles("game1", "")
	if len(got) != 1 {
		t.Fatalf("re-deleting created %d records, want 1", len(got))
	}
	if got["slot.sav"].Hash != "new" {
		t.Errorf("hash = %q, want the content it held when last deleted", got["slot.sav"].Hash)
	}
	if got["slot.sav"].DeletedAtMs <= older {
		t.Error("the timestamp did not move forward on re-deletion")
	}
}

// Records expire. Syncthing's own tracker has a case of stale deletion records
// resurrecting files long afterwards; a bound is what prevents that here.
func TestOldRecordsArePruned(t *testing.T) {
	s := openTestStore(t)
	stale := time.Now().Add(-DeletedFileRetention - time.Hour).UnixMilli()
	fresh := time.Now().UnixMilli()
	if err := s.RecordDeletedFiles("game1", []DeletedFile{
		{Path: "ancient.sav", Hash: "h", DeletedAtMs: stale},
		{Path: "recent.sav", Hash: "h", DeletedAtMs: fresh},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.PruneDeletedFiles(); err != nil {
		t.Fatal(err)
	}
	got, _ := s.DeletedFiles("game1", "")
	if _, still := got["ancient.sav"]; still {
		t.Error("a record past its retention survived the prune")
	}
	if _, gone := got["recent.sav"]; !gone {
		t.Error("the prune took a record that was still within retention")
	}
}

// Untracking a game drops its records: they describe a folder this device no
// longer has an opinion about.
func TestUntrackingAGameForgetsItsDeletions(t *testing.T) {
	s := openTestStore(t)
	if err := s.RecordDeletedFiles("game1", []DeletedFile{{Path: "a.sav", Hash: "h"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordDeletedFiles("game2", []DeletedFile{{Path: "b.sav", Hash: "h"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.ClearDeletedFilesForGame("game1"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.DeletedFiles("game1", ""); len(got) != 0 {
		t.Errorf("game1 kept %d records", len(got))
	}
	if got, _ := s.DeletedFiles("game2", ""); len(got) != 1 {
		t.Error("clearing one game took another game's records with it")
	}
}
