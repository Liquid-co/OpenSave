package e2e

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/opensave/opensave/testutil"
)

// A deleted save must stay deleted even when the shared lineage no longer
// proves the two devices ever had the file.
//
// This is the resurrection bug, made deterministic. In the wild it appeared as
// a race: the lineage is rebuilt from the intersection of the two devices'
// current manifests, so a rebuild landing after a local deletion drops the very
// path that was the evidence. The deletion then reads as "the peer has a file
// we lack" and the file is pulled back onto the machine it was deleted from.
//
// Rather than race for it, the lineage is cleared outright before the deletion
// syncs — the same state a badly-timed rebuild produces, reached on purpose.
// Without a recorded deletion this cannot work at all; there is nothing left
// to distinguish "I deleted it" from "they added it".
func TestDeletion_SurvivesTheLineageBeingLost(t *testing.T) {
	a := testutil.NewTestDaemon(t, "DelRec-A")
	b := testutil.NewTestDaemon(t, "DelRec-B")
	a.PairWith(b)

	a.WriteSave("keep.sav", "keep me")
	a.WriteSave("drop.sav", "drop me")
	gameID := a.TrackGame("RecordedDeletion")

	var gameB struct {
		ID string `json:"id"`
	}
	b.API(http.MethodPost, "/api/games",
		map[string]string{"name": "RecordedDeletion", "savePath": b.SaveDir}, &gameB)

	syncTo(a, gameID, b.NodeID())
	if !testutil.WaitFor(45*time.Second, func() bool { return b.ReadSave("drop.sav") == "drop me" }) {
		t.Fatal("setup: the file never reached the peer")
	}
	testutil.SettleSync(t, gameID, a, b)

	// Delete it, and snapshot so the deletion is written down.
	if err := os.Remove(filepath.Join(a.SaveDir, "drop.sav")); err != nil {
		t.Fatal(err)
	}
	a.API(http.MethodPost, "/api/games/"+gameID+"/snapshot",
		map[string]string{"comment": "deleted a save"}, nil)

	records, err := a.Daemon.Store.DeletedFiles(gameID, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := records["drop.sav"]; !ok {
		t.Fatalf("the deletion was not recorded at snapshot time; records: %+v", records)
	}

	// Now destroy the evidence the old mechanism depended on.
	if err := a.Daemon.Store.SetSyncState(gameID, b.NodeID(), nil, nil); err != nil {
		t.Fatal(err)
	}

	syncTo(a, gameID, b.NodeID())

	if !testutil.WaitFor(45*time.Second, func() bool { return b.ReadSave("drop.sav") == "" }) {
		t.Errorf("the deletion did not reach the peer with the lineage gone — the "+
			"record is what has to carry it (peer still holds %q)", b.ReadSave("drop.sav"))
	}
	if got := a.ReadSave("drop.sav"); got != "" {
		t.Errorf("the deleted file came back on the device that deleted it (%q) — "+
			"this is the resurrection the record exists to prevent", got)
	}
	if got := b.ReadSave("keep.sav"); got != "keep me" {
		t.Errorf("an unrelated save was disturbed on the peer: %q", got)
	}
}

// The safety property, end to end: a recorded deletion must never remove a
// copy the other device has since edited. Their bytes are newer and win.
//
// This is what makes the record safe to act on without version vectors. It is
// stricter than Syncthing, which lets a deletion win and keeps the loser as a
// .sync-conflict copy; here a deletion simply cannot remove content that
// differs from what was deleted.
func TestDeletion_NeverRemovesAnEditThePeerMadeAfterwards(t *testing.T) {
	a := testutil.NewTestDaemon(t, "DelSafe-A")
	b := testutil.NewTestDaemon(t, "DelSafe-B")
	a.PairWith(b)

	a.WriteSave("shared.sav", "original")
	gameID := a.TrackGame("SafeDeletion")

	var gameB struct {
		ID string `json:"id"`
	}
	b.API(http.MethodPost, "/api/games",
		map[string]string{"name": "SafeDeletion", "savePath": b.SaveDir}, &gameB)

	syncTo(a, gameID, b.NodeID())
	if !testutil.WaitFor(45*time.Second, func() bool { return b.ReadSave("shared.sav") == "original" }) {
		t.Fatal("setup: the file never reached the peer")
	}
	testutil.SettleSync(t, gameID, a, b)

	// A deletes it and records that.
	if err := os.Remove(filepath.Join(a.SaveDir, "shared.sav")); err != nil {
		t.Fatal(err)
	}
	a.API(http.MethodPost, "/api/games/"+gameID+"/snapshot",
		map[string]string{"comment": "deleted"}, nil)

	// B, meanwhile, kept playing and changed the same file.
	b.WriteSave("shared.sav", "B PLAYED ON AND SAVED")

	// Lineage cleared, so only the record could drive a deletion.
	if err := a.Daemon.Store.SetSyncState(gameID, b.NodeID(), nil, nil); err != nil {
		t.Fatal(err)
	}
	syncTo(a, gameID, b.NodeID())

	if got := b.ReadSave("shared.sav"); got != "B PLAYED ON AND SAVED" {
		t.Errorf("the peer's newer save is %q — a recorded deletion destroyed work "+
			"that was done after the file was deleted elsewhere", got)
	}
}
