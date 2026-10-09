package e2e

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/opensave/opensave/internal/p2p/syncengine"
	"github.com/opensave/opensave/internal/store"
	"github.com/opensave/opensave/testutil"
)

func sideDir(t *testing.T, td *testutil.TestDaemon, name string) string {
	t.Helper()
	dir := filepath.Join(filepath.Dir(td.SaveDir), name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func writeHoldFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readHoldFile(t *testing.T, dir, rel string) string {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatalf("reading save fixture %q: %v", path, err)
	}
	return string(raw)
}

func fileHash(dir, rel string) string {
	raw, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

type holdGame struct {
	ID               string `json:"id"`
	AutoSync         bool   `json:"autoSync"`
	ProvisioningHold bool   `json:"provisioningHold"`
	SavePath         string `json:"savePath"`
}

func createHeld(t *testing.T, td *testutil.TestDaemon, id, name, path string) holdGame {
	t.Helper()
	var got holdGame
	td.API(http.MethodPost, "/api/games", map[string]any{
		"id": id, "name": name, "savePath": path, "provisioningHold": true,
	}, &got)
	if got.ID != id || got.AutoSync || !got.ProvisioningHold {
		t.Fatalf("held create = %+v", got)
	}
	held, err := td.Daemon.Store.ProvisioningHeld(id)
	if err != nil || !held {
		t.Fatalf("hold not committed for %s: %v", id, err)
	}
	return got
}

func assertHeldQuiet(t *testing.T, source, bridge *testutil.TestDaemon, id, sourceDir, bridgeDir, marker string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	source.Daemon.P2P.PingPairedPeers(ctx)
	source.Daemon.P2P.SyncAllGames(ctx)
	bridge.Daemon.P2P.PingPairedPeers(ctx)
	bridge.Daemon.P2P.SyncAllGames(ctx)
	if _, err := source.Daemon.P2P.Sync.SyncGame(ctx, id, source.Daemon.P2P.OnlinePeers()); !errors.Is(err, syncengine.ErrProvisioning) {
		t.Fatalf("held game must refuse direct sync, got %v", err)
	}
	var syncResp struct {
		Error  string `json:"error"`
		Reason string `json:"reason"`
	}
	status := source.APIStatus(http.MethodPost, "/api/games/"+id+"/sync", nil, &syncResp)
	if status != http.StatusConflict || syncResp.Reason != "provisioning" {
		t.Fatalf("direct sync of a held game returned %d %+v", status, syncResp)
	}
	if readHoldFile(t, bridgeDir, marker) != "" || readHoldFile(t, sourceDir, "from-bridge.sav") != "" {
		t.Fatalf("held game transferred: bridge=%q source=%q", readHoldFile(t, bridgeDir, marker), readHoldFile(t, sourceDir, "from-bridge.sav"))
	}
	if _, err := bridge.Daemon.Store.GetGame(id); err != nil {
		t.Fatal("bridge lost the held game")
	}
}

// A paired source can record a new game without the bridge learning it, and
// the bridge can record the same id itself. Restart, reconcile, a direct
// sync and the retry path must not move bytes. An unrelated game still syncs.
func TestProvisioningHold_PairedCreateRestartAndIsolation(t *testing.T) {
	source := testutil.NewTestDaemon(t, "Hold-Source")
	bridge := testutil.NewTestDaemon(t, "Hold-Bridge")

	unrelatedSrc := sideDir(t, source, "unrelated")
	unrelatedBr := sideDir(t, bridge, "unrelated")
	writeHoldFile(t, unrelatedSrc, "keep/unrelated.sav", "ONLY-UNRELATED")
	writeHoldFile(t, unrelatedBr, "keep/unrelated.sav", "ONLY-UNRELATED")
	source.API(http.MethodPost, "/api/games", map[string]string{"name": "Hold Unrelated", "savePath": unrelatedSrc}, nil)
	bridge.API(http.MethodPost, "/api/games", map[string]string{"name": "Hold Unrelated", "savePath": unrelatedBr}, nil)
	source.PairWith(bridge)

	newSrc := sideDir(t, source, "new-game")
	newBr := sideDir(t, bridge, "new-game")
	writeHoldFile(t, newSrc, "profile/source.sav", "ONLY-SOURCE")
	const id = "hold-new-game"
	createHeld(t, source, id, "Hold New Game", newSrc)
	// No offer is required. The bridge is given the source's id directly.
	offers, _ := bridge.Daemon.Store.ListOfferedGames()
	if len(offers) != 0 {
		t.Fatalf("held source published an offer: %+v", offers)
	}
	createHeld(t, bridge, id, "Hold New Game", newBr)

	writeHoldFile(t, unrelatedSrc, "keep/fresh.sav", "UNRELATED-FRESH")
	if !testutil.WaitFor(30*time.Second, func() bool {
		source.API(http.MethodPost, "/api/games/"+store.SlugifyGameID("Hold Unrelated")+"/sync", nil, nil)
		return readHoldFile(t, unrelatedBr, "keep/fresh.sav") == "UNRELATED-FRESH"
	}) {
		t.Fatal("unrelated game stopped syncing while another game was held")
	}
	if readHoldFile(t, newBr, "profile/source.sav") != "" {
		t.Fatal("unrelated sync also copied the held game")
	}

	// Crash after the row is committed: the next process is a fresh daemon.New.
	source.Restart()
	bridge.Restart()
	for _, node := range []*testutil.TestDaemon{source, bridge} {
		if node.Daemon.SyncPauseStatus().Paused {
			t.Fatal("restart kept an in-memory pause; the hold must not depend on one")
		}
		held, err := node.Daemon.Store.ProvisioningHeld(id)
		if err != nil || !held {
			t.Fatalf("%s lost the hold across restart: %v", node.Name(), err)
		}
	}
	assertHeldQuiet(t, source, bridge, id, newSrc, newBr, "profile/source.sav")

	var manifest struct {
		Error string `json:"error"`
	}
	if status := bridge.APIStatus(http.MethodGet, "/api/p2p/manifest/"+id, nil, &manifest); status < 400 || manifest.Error != syncengine.ProvisioningMessage {
		t.Fatalf("manifest of a held game = %d %q", status, manifest.Error)
	}
	if manifest.Error != "" && (len(manifest.Error) > 0 && containsNotFound(manifest.Error)) {
		t.Fatalf("provisioning refusal looks like not-found: %q", manifest.Error)
	}
	writeHoldFile(t, newBr, "profile/bridge.sav", "ONLY-BRIDGE")
	var deleted struct {
		Error string `json:"error"`
	}
	if status := bridge.APIStatus(http.MethodPost, "/api/p2p/delete-file/"+id, map[string]string{"relPath": "profile/bridge.sav"}, &deleted); status < 400 {
		t.Fatalf("delete-file on a held game returned %d", status)
	}
	if readHoldFile(t, newBr, "profile/bridge.sav") != "ONLY-BRIDGE" {
		t.Fatal("inbound delete changed a held save")
	}

	// One side released, then both processes replaced. The other hold remains.
	var rel struct {
		Released bool `json:"released"`
	}
	source.API(http.MethodPost, "/api/games/"+id+"/release-provisioning", nil, &rel)
	if !rel.Released {
		t.Fatal("source release did not clear the hold")
	}
	source.Restart()
	bridge.Restart()
	held, err := bridge.Daemon.Store.ProvisioningHeld(id)
	if err != nil || !held {
		t.Fatal("bridge hold did not survive a restart after only the source was released")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	source.Daemon.P2P.PingPairedPeers(ctx)
	source.Daemon.P2P.SyncAllGames(ctx)
	if readHoldFile(t, newBr, "profile/source.sav") != "" || readHoldFile(t, newSrc, "profile/bridge.sav") != "" {
		t.Fatal("one-sided release transferred saves")
	}

	bridge.API(http.MethodPost, "/api/games/"+id+"/release-provisioning", nil, &rel)
	if !testutil.WaitFor(45*time.Second, func() bool {
		source.API(http.MethodPost, "/api/games/"+id+"/sync", nil, nil)
		return readHoldFile(t, newBr, "profile/source.sav") == "ONLY-SOURCE" &&
			fileHash(newSrc, "profile/source.sav") == fileHash(newBr, "profile/source.sav")
	}) {
		t.Fatalf("release did not sync: bridge=%q", readHoldFile(t, newBr, "profile/source.sav"))
	}
	if readHoldFile(t, unrelatedBr, "keep/fresh.sav") != "UNRELATED-FRESH" {
		t.Fatal("release disturbed the unrelated game")
	}
}

func containsNotFound(s string) bool {
	return len(s) >= 9 && (s == "Game not found." || (len(s) > 0 && (containsFold(s, "not found"))))
}

func containsFold(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if equalFoldASCII(s[i:i+len(sub)], sub) {
				return true
			}
		}
		return false
	})())
}

func equalFoldASCII(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

func TestProvisioningHold_PlaceOfferWithoutSyncing(t *testing.T) {
	source := testutil.NewTestDaemon(t, "Place-Source")
	bridge := testutil.NewTestDaemon(t, "Place-Bridge")
	source.PairWith(bridge)
	setUnknownGamePolicy(t, bridge, store.UnknownGameAsk)

	source.WriteSave("slot.sav", "FROM-SOURCE")
	gameID := source.TrackGame("Place Held Game")
	syncTo(source, gameID, bridge.NodeID())
	if !testutil.WaitFor(20*time.Second, func() bool {
		offers, _ := bridge.Daemon.Store.ListOfferedGames()
		return len(offers) == 1 && offers[0].GameID == gameID
	}) {
		t.Fatal("bridge did not record an offer to place")
	}

	dest := sideDir(t, bridge, "placed")
	var placed holdGame
	bridge.API(http.MethodPost, "/api/offered-games/"+gameID+"/place", map[string]any{
		"path": dest, "autoSync": false,
	}, &placed)
	if placed.ID != gameID || !placed.ProvisioningHold || placed.AutoSync {
		t.Fatalf("held place = %+v", placed)
	}
	bridge.API(http.MethodPost, "/api/offered-games/"+gameID+"/place", map[string]any{
		"path": dest, "provisioningHold": true,
	}, &placed)
	games, _ := bridge.Daemon.Store.ListGames()
	n := 0
	for _, g := range games {
		if g.ID == gameID {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("repeated place created %d games", n)
	}
	source.API(http.MethodPost, "/api/games/"+gameID+"/sync", nil, nil)
	if readHoldFile(t, dest, "slot.sav") != "" {
		t.Fatal("placing with autoSync false synced the save")
	}

	bridge.API(http.MethodPost, "/api/games/"+gameID+"/release-provisioning", nil, nil)
	if !testutil.WaitFor(45*time.Second, func() bool {
		source.API(http.MethodPost, "/api/games/"+gameID+"/sync", nil, nil)
		return readHoldFile(t, dest, "slot.sav") == "FROM-SOURCE"
	}) {
		t.Fatalf("release did not deliver the save, dest=%q", readHoldFile(t, dest, "slot.sav"))
	}
}

func TestProvisioningHold_RejectsUnsafePathsAndDuplicates(t *testing.T) {
	node := testutil.NewTestDaemon(t, "Hold-Paths")
	live := sideDir(t, node, "live")
	node.API(http.MethodPost, "/api/games", map[string]string{"name": "Live Occupant", "savePath": live}, nil)

	var errBody struct {
		Error string `json:"error"`
	}
	if status := node.APIStatus(http.MethodPost, "/api/games", map[string]any{
		"id": "other-id", "name": "Other", "savePath": live, "provisioningHold": true,
	}, &errBody); status < 400 {
		t.Fatal("held create accepted an occupied folder")
	}
	if g, err := node.Daemon.Store.GetGame("other-id"); err == nil {
		t.Fatalf("occupied create still inserted %+v", g)
	}

	outside := sideDir(t, node, "outside")
	link := filepath.Join(filepath.Dir(node.SaveDir), "escape-link")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if status := node.APIStatus(http.MethodPost, "/api/games", map[string]any{
		"id": "symlink-game", "name": "Symlink", "savePath": link, "provisioningHold": true,
	}, &errBody); status < 400 {
		t.Fatal("held create accepted a symlink")
	}

	primary := sideDir(t, node, "primary")
	createHeld(t, node, "dup-game", "Dup", primary)
	other := sideDir(t, node, "other")
	if status := node.APIStatus(http.MethodPost, "/api/games", map[string]any{
		"id": "dup-game", "name": "Dup", "savePath": other, "provisioningHold": true,
	}, &errBody); status < 400 {
		t.Fatal("a second path for a held id was accepted")
	}
	again := createHeld(t, node, "dup-game", "Dup", primary)
	if again.SavePath == "" {
		t.Fatal("retry of the same held create failed")
	}
	if status := node.APIStatus(http.MethodPost, "/api/games", map[string]any{
		"id": "pinned", "name": "Pinned", "savePath": primary,
	}, &errBody); status != http.StatusBadRequest {
		t.Fatalf("explicit id without a hold returned %d", status)
	}
}

func TestProvisioningHold_NamedRootsBeforeRelease(t *testing.T) {
	source := testutil.NewTestDaemon(t, "Roots-Source")
	bridge := testutil.NewTestDaemon(t, "Roots-Bridge")
	source.PairWith(bridge)
	const id = "roots-game"
	src := sideDir(t, source, "main")
	br := sideDir(t, bridge, "main")
	srcA, srcB := sideDir(t, source, "root-a"), sideDir(t, source, "root-b")
	brA, brB := sideDir(t, bridge, "root-a"), sideDir(t, bridge, "root-b")
	writeHoldFile(t, src, "main.sav", "MAIN")
	writeHoldFile(t, srcA, "a.sav", "ROOT-A")
	writeHoldFile(t, srcB, "b.sav", "ROOT-B")
	createHeld(t, source, id, "Roots Game", src)
	createHeld(t, bridge, id, "Roots Game", br)
	for _, spec := range []struct {
		node       *testutil.TestDaemon
		name, path string
	}{
		{source, "config", srcA},
		{source, "mods", srcB},
		{bridge, "config", brA},
		{bridge, "mods", brB},
	} {
		spec.node.API(http.MethodPost, "/api/games/"+id+"/roots", map[string]string{"name": spec.name, "path": spec.path}, nil)
	}
	if status := source.APIStatus(http.MethodPost, "/api/games/"+id+"/sync", nil, nil); status < 400 {
		t.Fatalf("sync of a held game returned %d", status)
	}
	if readHoldFile(t, br, "main.sav") != "" || readHoldFile(t, brA, "a.sav") != "" || readHoldFile(t, brB, "b.sav") != "" {
		t.Fatal("named roots synced before release")
	}
	source.API(http.MethodPost, "/api/games/"+id+"/release-provisioning", nil, nil)
	bridge.API(http.MethodPost, "/api/games/"+id+"/release-provisioning", nil, nil)
	if !testutil.WaitFor(45*time.Second, func() bool {
		source.API(http.MethodPost, "/api/games/"+id+"/sync", nil, nil)
		return readHoldFile(t, br, "main.sav") == "MAIN" && readHoldFile(t, brA, "a.sav") == "ROOT-A" && readHoldFile(t, brB, "b.sav") == "ROOT-B"
	}) {
		t.Fatalf("roots after release: main=%q a=%q b=%q", readHoldFile(t, br, "main.sav"), readHoldFile(t, brA, "a.sav"), readHoldFile(t, brB, "b.sav"))
	}
}

func TestProvisioningHold_HundredGamesStayOneDaemon(t *testing.T) {
	source := testutil.NewTestDaemon(t, "Bulk-Source")
	receiver := testutil.NewTestDaemon(t, "Bulk-Receiver")
	liveSrc := sideDir(t, source, "live")
	liveRecv := sideDir(t, receiver, "live")
	writeHoldFile(t, liveSrc, "live.sav", "LIVE")
	source.API(http.MethodPost, "/api/games", map[string]string{"name": "Bulk Live", "savePath": liveSrc}, nil)
	receiver.API(http.MethodPost, "/api/games", map[string]string{"name": "Bulk Live", "savePath": liveRecv}, nil)
	source.PairWith(receiver)
	liveID := store.SlugifyGameID("Bulk Live")
	if !testutil.WaitFor(30*time.Second, func() bool {
		source.API(http.MethodPost, "/api/games/"+liveID+"/sync", nil, nil)
		return readHoldFile(t, liveSrc, "live.sav") == "LIVE" && readHoldFile(t, liveRecv, "live.sav") == "LIVE"
	}) {
		t.Fatal("the unrelated live game did not establish an initial sync")
	}
	srcBase := source.Daemon.Store.GetAgreedHash(liveID, receiver.NodeID())
	recvBase := receiver.Daemon.Store.GetAgreedHash(liveID, source.NodeID())
	if srcBase == "" || recvBase == "" || srcBase != recvBase {
		t.Fatalf("initial sync left no bilateral agreed base: source=%q receiver=%q", srcBase, recvBase)
	}

	start := time.Now()
	for i := 0; i < 100; i++ {
		id := "bulk-" + hex.EncodeToString([]byte{byte(i)})
		dir := sideDir(t, source, id)
		writeHoldFile(t, dir, "held.sav", "HELD-"+id)
		createHeld(t, source, id, "Bulk "+id, dir)
	}
	if elapsed := time.Since(start); elapsed > 45*time.Second {
		t.Fatalf("100 held creates took %s; this must not be a daemon per game", elapsed)
	}
	offers, err := receiver.Daemon.Store.ListOfferedGames()
	if err != nil {
		t.Fatal(err)
	}
	if len(offers) != 0 {
		t.Fatalf("held creates published %d offers", len(offers))
	}
	writeHoldFile(t, liveSrc, "live.sav", "LIVE-UPDATED")
	if !testutil.WaitFor(30*time.Second, func() bool {
		source.API(http.MethodPost, "/api/games/"+liveID+"/sync", nil, nil)
		return readHoldFile(t, liveRecv, "live.sav") == "LIVE-UPDATED" && readHoldFile(t, liveSrc, "live.sav") == "LIVE-UPDATED"
	}) {
		t.Fatal("the live game did not sync beside 100 held games")
	}
	if _, err := os.Stat(filepath.Join(liveRecv, "held.sav")); !os.IsNotExist(err) {
		t.Fatalf("a held save appeared on the receiver: %v", err)
	}
	source.Restart()
	set, err := source.Daemon.Store.ProvisioningHeldSet()
	if err != nil {
		t.Fatal(err)
	}
	if len(set) != 100 {
		t.Fatalf("restart kept %d holds, want 100", len(set))
	}
}

func TestProvisioningHold_DoesNotConvertAnExistingGame(t *testing.T) {
	node := testutil.NewTestDaemon(t, "Hold-Existing")
	var live holdGame
	node.API(http.MethodPost, "/api/games", map[string]string{
		"name": "Already Live", "savePath": node.SaveDir,
	}, &live)
	writeHoldFile(t, node.SaveDir, "keep.sav", "LIVE-BYTES")
	var errBody struct {
		Error string `json:"error"`
	}
	if status := node.APIStatus(http.MethodPost, "/api/games", map[string]any{
		"id": live.ID, "name": "Already Live", "savePath": node.SaveDir, "provisioningHold": true,
	}, &errBody); status < 400 {
		t.Fatal("a hold was accepted for a game that was already tracking")
	}
	got, err := node.Daemon.Store.GetGame(live.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.AutoSync {
		t.Fatal("the failed hold turned AutoSync off")
	}
	held, err := node.Daemon.Store.ProvisioningHeld(live.ID)
	if err != nil || held {
		t.Fatalf("existing game held=%v err=%v", held, err)
	}
	if readHoldFile(t, node.SaveDir, "keep.sav") != "LIVE-BYTES" {
		t.Fatal("the failed hold changed save bytes")
	}
}

// Three peers in a star: ends talk only to the middle. Releasing the two ends
// that should converge must not move the third while it is still held, and
// release itself must not start that transfer.
func TestProvisioningHold_StarLeavesHeldEndpointAlone(t *testing.T) {
	leafA := testutil.NewTestDaemon(t, "Star-LeafA")
	hub := testutil.NewTestDaemon(t, "Star-Hub")
	leafB := testutil.NewTestDaemon(t, "Star-LeafB")
	leafA.PairWith(hub)
	leafB.PairWith(hub)
	if _, err := leafA.Daemon.Store.GetPeer(leafB.NodeID()); err == nil {
		t.Fatal("leafA is paired with leafB; the fixture is not a star")
	}

	const id = "star-game"
	bz := sideDir(t, leafA, "star")
	br := sideDir(t, hub, "star")
	dk := sideDir(t, leafB, "star")
	writeHoldFile(t, bz, "progress.sav", "LEAF-A-ONLY")
	writeHoldFile(t, dk, "progress.sav", "LEAF-B-ONLY")
	createHeld(t, leafA, id, "Star Game", bz)
	createHeld(t, hub, id, "Star Game", br)
	createHeld(t, leafB, id, "Star Game", dk)

	var rel struct {
		Released bool `json:"released"`
	}
	leafA.API(http.MethodPost, "/api/games/"+id+"/release-provisioning", nil, &rel)
	hub.API(http.MethodPost, "/api/games/"+id+"/release-provisioning", nil, &rel)
	// Default release enables AutoSync; the released pair may converge
	// before this assertion. The held third peer must remain untouched.
	if readHoldFile(t, dk, "progress.sav") != "LEAF-B-ONLY" {
		t.Fatal("default release touched the held third peer")
	}
	if !testutil.WaitFor(45*time.Second, func() bool {
		leafA.API(http.MethodPost, "/api/games/"+id+"/sync", nil, nil)
		return readHoldFile(t, br, "progress.sav") == "LEAF-A-ONLY" &&
			readHoldFile(t, dk, "progress.sav") == "LEAF-B-ONLY" &&
			readHoldFile(t, bz, "progress.sav") == "LEAF-A-ONLY"
	}) {
		t.Fatalf("star sync moved a held endpoint or failed to converge the released pair: hub=%q leafB=%q leafA=%q",
			readHoldFile(t, br, "progress.sav"), readHoldFile(t, dk, "progress.sav"), readHoldFile(t, bz, "progress.sav"))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	hub.Daemon.P2P.PingPairedPeers(ctx)
	hub.Daemon.P2P.SyncAllGames(ctx)
	if readHoldFile(t, dk, "progress.sav") != "LEAF-B-ONLY" || readHoldFile(t, bz, "progress.sav") != "LEAF-A-ONLY" {
		t.Fatal("reconcile on the middle peer moved a held endpoint or overwrote the released source")
	}
}

// Default release turns AutoSync on. The periodic reconcile and a peer
// coming online call SyncAllGames, not the explicit sync route. That is
// enough to move bytes. This is the witness that "release does not call
// SyncGame" is not "nothing syncs until asked".
func TestProvisioningHold_DefaultReleaseLetsReconcileSync(t *testing.T) {
	source := testutil.NewTestDaemon(t, "Reconcile-Source")
	bridge := testutil.NewTestDaemon(t, "Reconcile-Bridge")
	source.PairWith(bridge)
	const id = "reconcile-game"
	src := sideDir(t, source, "recon")
	br := sideDir(t, bridge, "recon")
	writeHoldFile(t, src, "slot.sav", "RECONCILE-BYTES")
	createHeld(t, source, id, "Reconcile Game", src)
	createHeld(t, bridge, id, "Reconcile Game", br)
	source.API(http.MethodPost, "/api/games/"+id+"/release-provisioning", nil, nil)
	bridge.API(http.MethodPost, "/api/games/"+id+"/release-provisioning", nil, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	source.Daemon.P2P.PingPairedPeers(ctx)
	source.Daemon.P2P.SyncAllGames(ctx)
	if !testutil.WaitFor(20*time.Second, func() bool {
		return readHoldFile(t, br, "slot.sav") == "RECONCILE-BYTES"
	}) {
		t.Fatal("default release plus reconcile did not sync; the witness is wrong or the path changed")
	}
}

// Staged release clears the hold and leaves AutoSync off. Reconcile, a
// restart, and a file write must not move bytes. An explicit sync may, and
// a peer that is still held must refuse. Both ends of the star are tried as
// the writer.
func TestProvisioningHold_StagedReleaseSkipsReconcile(t *testing.T) {
	for _, writer := range []string{"leafA", "leafB"} {
		t.Run(writer, func(t *testing.T) {
			leafA := testutil.NewTestDaemon(t, "Staged-LeafA")
			hub := testutil.NewTestDaemon(t, "Staged-Hub")
			leafB := testutil.NewTestDaemon(t, "Staged-LeafB")
			leafA.PairWith(hub)
			leafB.PairWith(hub)
			const id = "staged-game"
			bz := sideDir(t, leafA, "staged")
			br := sideDir(t, hub, "staged")
			dk := sideDir(t, leafB, "staged")
			writeHoldFile(t, bz, "progress.sav", "LEAF-A-ONLY")
			writeHoldFile(t, dk, "progress.sav", "LEAF-B-ONLY")
			createHeld(t, leafA, id, "Staged Game", bz)
			createHeld(t, hub, id, "Staged Game", br)
			createHeld(t, leafB, id, "Staged Game", dk)

			progressed := leafA
			progressedDir, quietDir, quietBytes := bz, dk, "LEAF-B-ONLY"
			if writer == "leafB" {
				progressed = leafB
				progressedDir, quietDir, quietBytes = dk, bz, "LEAF-A-ONLY"
			}
			var rel struct {
				Released bool `json:"released"`
				AutoSync bool `json:"autoSync"`
			}
			progressed.API(http.MethodPost, "/api/games/"+id+"/release-provisioning", map[string]any{"autoSync": false}, &rel)
			hub.API(http.MethodPost, "/api/games/"+id+"/release-provisioning", map[string]any{"autoSync": false}, &rel)
			if rel.AutoSync {
				t.Fatal("staged release turned AutoSync on")
			}
			writeHoldFile(t, progressedDir, "later.sav", "AFTER-RELEASE")
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			for _, node := range []*testutil.TestDaemon{leafA, hub, leafB} {
				node.Daemon.P2P.PingPairedPeers(ctx)
				node.Daemon.P2P.SyncAllGames(ctx)
			}
			if readHoldFile(t, br, "progress.sav") != "" || readHoldFile(t, quietDir, "progress.sav") != quietBytes {
				t.Fatalf("staged reconcile moved bytes: hub=%q quiet=%q", readHoldFile(t, br, "progress.sav"), readHoldFile(t, quietDir, "progress.sav"))
			}
			progressed.Restart()
			hub.Restart()
			progressed.Daemon.P2P.PingPairedPeers(ctx)
			progressed.Daemon.P2P.SyncAllGames(ctx)
			if readHoldFile(t, br, "progress.sav") != "" {
				t.Fatal("restart after staged release let reconcile copy the save")
			}
			if !testutil.WaitFor(45*time.Second, func() bool {
				progressed.API(http.MethodPost, "/api/games/"+id+"/sync", nil, nil)
				return readHoldFile(t, br, "progress.sav") == map[string]string{"leafA": "LEAF-A-ONLY", "leafB": "LEAF-B-ONLY"}[writer]
			}) {
				t.Fatalf("explicit sync after staged release did not converge the released pair: hub=%q", readHoldFile(t, br, "progress.sav"))
			}
			if readHoldFile(t, br, "later.sav") != "AFTER-RELEASE" {
				t.Fatal("explicit sync omitted later save progress")
			}
			for _, rel := range []string{"progress.sav", "later.sav"} {
				want, got := fileHash(progressedDir, rel), fileHash(br, rel)
				if want == "" || want != got {
					t.Fatalf("sync hash differs for %q: source=%q hub=%q", rel, want, got)
				}
			}
			if readHoldFile(t, quietDir, "progress.sav") != quietBytes || readHoldFile(t, quietDir, "later.sav") != "" {
				t.Fatal("explicit sync moved the peer that is still held")
			}
		})
	}
}

// A staged release blocks local auto-sync attempts but NOT an unheld
// peer's incoming request. This endpoint is not peer-specific authorization.
func TestProvisioningHold_StagedReleaseAllowsPeerInitiatedSync(t *testing.T) {
	source := testutil.NewTestDaemon(t, "SyncRequester")
	receiver := testutil.NewTestDaemon(t, "StagedReceiver")
	source.WriteSave("slot.sav", "PEER-INITIATED")
	gameID := source.TrackGame("Peer Initiated Staged Game")
	recvDir := sideDir(t, receiver, "incoming")
	createHeld(t, receiver, gameID, "Peer Initiated Staged Game", recvDir)
	source.PairWith(receiver)
	var state struct {
		Released bool `json:"released"`
		AutoSync bool `json:"autoSync"`
	}
	receiver.API(http.MethodPost, "/api/games/"+gameID+"/release-provisioning",
		map[string]any{"autoSync": false}, &state)
	if !state.Released || state.AutoSync {
		t.Fatalf("staged release state = %+v", state)
	}
	if !testutil.WaitFor(45*time.Second, func() bool {
		source.API(http.MethodPost, "/api/games/"+gameID+"/sync", nil, nil)
		return readHoldFile(t, recvDir, "slot.sav") == "PEER-INITIATED"
	}) {
		t.Fatal("released receiver did not accept peer-initiated sync")
	}
	game, err := receiver.Daemon.Store.GetGame(gameID)
	if err != nil || game.AutoSync {
		t.Fatalf("peer request changed local AutoSync: %+v, %v", game, err)
	}
}

func TestProvisioningHold_PatchDoesNotUnhold(t *testing.T) {
	source := testutil.NewTestDaemon(t, "Patch-Source")
	bridge := testutil.NewTestDaemon(t, "Patch-Bridge")
	source.PairWith(bridge)
	const id = "patch-game"
	src := sideDir(t, source, "patch")
	br := sideDir(t, bridge, "patch")
	writeHoldFile(t, src, "slot.sav", "PATCH-BYTES")
	createHeld(t, source, id, "Patch Game", src)
	createHeld(t, bridge, id, "Patch Game", br)
	var patched struct {
		Name                    string `json:"name"`
		AutoSync                bool   `json:"autoSync"`
		ProvisioningHold        bool   `json:"provisioningHold"`
		ProvisioningHoldUnknown bool   `json:"provisioningHoldUnknown"`
	}
	source.API(http.MethodPatch, "/api/games/"+id, map[string]any{
		"name": "Renamed", "savePath": src, "autoSync": true,
	}, &patched)
	if patched.Name != "Renamed" || patched.AutoSync || !patched.ProvisioningHold || patched.ProvisioningHoldUnknown {
		t.Fatalf("patch changed the hold contract: %+v", patched)
	}
	held, err := source.Daemon.Store.ProvisioningHeld(id)
	if err != nil || !held {
		t.Fatal("PATCH cleared the hold")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	source.Daemon.P2P.PingPairedPeers(ctx)
	source.Daemon.P2P.SyncAllGames(ctx)
	if readHoldFile(t, br, "slot.sav") != "" {
		t.Fatal("PATCH autoSync true let reconcile copy a held save")
	}
}

// Path updates to a still-held game must reject symlink roots, as
// initial held creation and named-root admission already do.
func TestProvisioningHold_HeldPathUpdateRejectsSymlink(t *testing.T) {
	node := testutil.NewTestDaemon(t, "HeldPathUpdate")
	original := sideDir(t, node, "primary")
	createHeld(t, node, "held-path-update", "Held Path Update", original)
	target := sideDir(t, node, "target")
	link := filepath.Join(filepath.Dir(node.SaveDir), "held-path-link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	var resp struct {
		Error string `json:"error"`
	}
	status := node.APIStatus(http.MethodPatch, "/api/games/held-path-update",
		map[string]any{"savePath": link}, &resp)
	if status < 400 || status >= 500 {
		t.Fatalf("held game accepted symlink update: HTTP %d %+v", status, resp)
	}
	game, err := node.Daemon.Store.GetGame("held-path-update")
	if err != nil || game.SavePath != original || game.AutoSync {
		t.Fatalf("rejected path update changed game: %+v, %v", game, err)
	}
	held, err := node.Daemon.Store.ProvisioningHeld(game.ID)
	if err != nil || !held {
		t.Fatalf("rejected path update removed hold: %v, %v", held, err)
	}
}

func TestProvisioningHold_OrdinaryTrackUnchanged(t *testing.T) {
	node := testutil.NewTestDaemon(t, "Ordinary")
	var got holdGame
	node.API(http.MethodPost, "/api/games", map[string]string{"name": "Ordinary Game", "savePath": node.SaveDir}, &got)
	if !got.AutoSync || got.ProvisioningHold {
		t.Fatalf("ordinary track = %+v; omitting the hold must keep today's behaviour", got)
	}
}
