package e2e

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/opensave/opensave/testutil"
)

// Restoring a cloud backup made by the other device, when the two tracked the
// same game under different names.
//
// Reported by a user on 2.3.1: auto-scan on one device and Track folder on the
// other produced two different display names for one title. A game's id is the
// slug of its name and a backup is stored as "<id>__<branch>__<snapshot>.zip",
// so the provider ended up holding two differently named sets of files and
// each device would only restore its own — "I can only restore the backup from
// the device I uploaded it from".
//
// Linking the two in Manage > Linked Copies did not help, which is the part
// that made it look broken rather than merely fiddly: aliases were resolved on
// the peer-to-peer path and nowhere in the cloud screens.
func TestCloudLinkedCopies_RestoreABackupUploadedUnderTheOtherDevicesName(t *testing.T) {
	a := testutil.NewTestDaemon(t, "CloudLink-A")

	// One shared provider folder, exactly as two devices signed into the same
	// account would see.
	dir := useLocalCloud(t, a)

	a.WriteSave("profile.sav", "device A save")
	uploaderID := a.TrackGame("Elden Ring")
	if uploaderID != "elden-ring" {
		t.Fatalf("expected the id to be the slug of the name, got %q", uploaderID)
	}

	a.API(http.MethodPost, "/api/games/"+uploaderID+"/snapshot",
		map[string]string{"comment": "from device A"}, nil)
	a.API(http.MethodPost, "/api/cloud/sync-local/"+uploaderID, nil, nil)

	names := waitForUpload(t, dir)
	var remoteName string
	for _, n := range names {
		if strings.HasPrefix(n, uploaderID+"__") {
			remoteName = n
		}
	}
	if remoteName == "" {
		t.Fatalf("device A's snapshot never reached the provider (saw %v)", names)
	}

	// The second device: same title, tracked under a different name, so a
	// different id. This is the whole cause.
	b := testutil.NewTestDaemon(t, "CloudLink-B")
	b.API(http.MethodPost, "/api/settings", map[string]any{
		"cloudSync": map[string]any{"enabled": true, "provider": "local", "url": dir},
	}, nil)
	b.WriteSave("profile.sav", "device B save")
	localID := b.TrackGame("ELDEN RING (PC)")
	if localID == uploaderID {
		t.Fatalf("setup: both devices produced the same id %q, so nothing is being tested", localID)
	}

	// Before linking, the backup is correctly none of this game's business.
	var before []map[string]any
	b.API(http.MethodGet, "/api/cloud/snapshots/"+localID, nil, &before)
	if len(before) != 0 {
		t.Fatalf("an unlinked game listed another id's backups: %v", before)
	}
	code := b.APIStatus(http.MethodPost, "/api/cloud/restore/"+localID,
		map[string]string{"fileName": remoteName}, nil)
	if code != http.StatusBadRequest {
		t.Errorf("restoring an unlinked backup returned %d, want 400 — an unlinked id must stay refused", code)
	}

	// Link them, which is what the user did.
	b.API(http.MethodPost, "/api/games/"+localID+"/link", map[string]string{"alias": uploaderID}, nil)

	// Now the other device's backup belongs to this game.
	var after []map[string]any
	b.API(http.MethodGet, "/api/cloud/snapshots/"+localID, nil, &after)
	if len(after) == 0 {
		t.Fatalf("after linking, the peer's backup is still not listed for this game (%s)", b.LastError())
	}

	code = b.APIStatus(http.MethodPost, "/api/cloud/restore/"+localID,
		map[string]string{"fileName": remoteName}, nil)
	if code != http.StatusOK {
		t.Fatalf("restore of the linked backup returned %d: %s", code, b.LastError())
	}

	if !testutil.WaitFor(30*time.Second, func() bool { return b.ReadSave("profile.sav") == "device A save" }) {
		t.Errorf("the restored save did not land: profile.sav = %q, want device A's content",
			b.ReadSave("profile.sav"))
	}
}

// The listing must show one game rather than two, and must not fall back to a
// bare slug for the id it does not recognise.
func TestCloudLinkedCopies_ListingFoldsLinkedIDsIntoOneGame(t *testing.T) {
	a := testutil.NewTestDaemon(t, "CloudFold-A")
	dir := useLocalCloud(t, a)

	a.WriteSave("profile.sav", "A")
	idA := a.TrackGame("Hollow Knight")
	a.API(http.MethodPost, "/api/games/"+idA+"/snapshot", map[string]string{"comment": "A"}, nil)
	a.API(http.MethodPost, "/api/cloud/sync-local/"+idA, nil, nil)
	waitForUpload(t, dir)

	b := testutil.NewTestDaemon(t, "CloudFold-B")
	b.API(http.MethodPost, "/api/settings", map[string]any{
		"cloudSync": map[string]any{"enabled": true, "provider": "local", "url": dir},
	}, nil)
	b.WriteSave("profile.sav", "B")
	idB := b.TrackGame("Hollow Knight Silksong Save")
	b.API(http.MethodPost, "/api/games/"+idB+"/snapshot", map[string]string{"comment": "B"}, nil)
	b.API(http.MethodPost, "/api/cloud/sync-local/"+idB, nil, nil)
	if !testutil.WaitFor(30*time.Second, func() bool { return len(cloudFiles(t, dir)) >= 2 }) {
		t.Fatalf("expected both devices' backups in the provider, saw %v", cloudFiles(t, dir))
	}

	b.API(http.MethodPost, "/api/games/"+idB+"/link", map[string]string{"alias": idA}, nil)

	var listing []struct {
		GameID   string `json:"gameId"`
		GameName string `json:"gameName"`
		Count    int    `json:"count"`
	}
	b.API(http.MethodGet, "/api/cloud/browse", nil, &listing)
	if len(listing) != 1 {
		t.Fatalf("listing shows %d games, want 1 after linking: %+v", len(listing), listing)
	}
	if listing[0].Count < 2 {
		t.Errorf("the folded game holds %d snapshots, want both devices': %+v", listing[0].Count, listing[0])
	}
	if listing[0].GameName == "" || listing[0].GameName == listing[0].GameID {
		t.Errorf("the folded game is labelled with a bare id (%q) rather than its name", listing[0].GameName)
	}
}
