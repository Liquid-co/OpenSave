package e2e

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/opensave/opensave/testutil"
)

// What a device sitting in your relay room can actually read.
//
// This is the threat payload sealing exists for, tested from the attacker's
// own seat rather than from a unit's. The relay forwards every frame to every
// member of the room — its reader loop says so plainly — so anyone you have
// ever given a room code to receives your sync traffic, whether or not you
// paired with them and whether or not they sync anything themselves.
//
// A unit test proving Seal and Open are inverse functions would not have
// caught the state this code was actually in: e2ee.Seal was written, tested,
// and had no callers outside its own package. So these tests do not ask
// whether the crypto works. They ask what arrives at a third party's socket.

// eavesdropper joins a relay room and keeps every frame it is handed.
type eavesdropper struct {
	mu     sync.Mutex
	frames [][]byte
}

func joinAsEavesdropper(t *testing.T, relayURL, room string) *eavesdropper {
	t.Helper()
	url := fmt.Sprintf("%s/?room=%s&device=%s", relayURL, room, "eavesdropper")
	dialCtx, cancelDial := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelDial()

	conn, _, err := websocket.Dial(dialCtx, url, nil)
	if err != nil {
		t.Fatalf("could not join the relay room as a third device: %v", err)
	}
	// The same limit the real client uses: without it a single block message
	// closes this socket and the test reports "saw nothing" for the wrong
	// reason.
	conn.SetReadLimit(16 << 20)

	readCtx, cancelRead := context.WithCancel(context.Background())
	e := &eavesdropper{}
	go func() {
		for {
			_, data, err := conn.Read(readCtx)
			if err != nil {
				return
			}
			e.mu.Lock()
			e.frames = append(e.frames, append([]byte(nil), data...))
			e.mu.Unlock()
		}
	}()
	t.Cleanup(func() {
		cancelRead()
		conn.Close(websocket.StatusNormalClosure, "")
	})
	return e
}

func (e *eavesdropper) snapshot() [][]byte {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([][]byte, len(e.frames))
	copy(out, e.frames)
	return out
}

func (e *eavesdropper) count() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.frames)
}

// leaked reports the first frame containing any form of the marker, or -1.
func (e *eavesdropper) leaked(needles []string) (int, string) {
	for i, f := range e.snapshot() {
		s := string(f)
		for _, n := range needles {
			if strings.Contains(s, n) {
				return i, n
			}
		}
	}
	return -1, ""
}

// sealedFrames counts the relayed frames that carried an encrypted payload.
//
// This is the guard against a green result that means nothing: if the sync
// went over the LAN instead of the relay, or the two daemons never talked at
// all, no marker would appear either and the test would "pass" while proving
// nothing. At least one sealed frame has to have crossed the room.
func (e *eavesdropper) sealedFrames() (sealed, plaintextPayload int) {
	type frame struct {
		Route      string          `json:"route"`
		Body       json.RawMessage `json:"body"`
		Data       json.RawMessage `json:"data"`
		SealedBody []byte          `json:"sealedBody"`
		SealedData []byte          `json:"sealedData"`
	}
	for _, raw := range e.snapshot() {
		var f frame
		if json.Unmarshal(raw, &f) != nil {
			continue
		}
		switch {
		case len(f.SealedBody) > 0 || len(f.SealedData) > 0:
			sealed++
		case len(f.Body) > 0 || len(f.Data) > 0:
			plaintextPayload++
		}
	}
	return sealed, plaintextPayload
}

// markerForms returns every spelling the marker can take on the wire.
//
// Save bytes travel base64-encoded inside JSON, and where the marker starts
// inside the file decides which of three encodings it gets. Searching only for
// the literal string would have let a completely unsealed block payload pass
// as clean — which is the exact failure a test like this is supposed to be
// incapable of.
func markerForms(marker string) []string {
	forms := []string{marker}
	for shift := 0; shift < 3; shift++ {
		enc := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("\x00", shift) + marker))
		// The first and last four characters depend on what surrounds the
		// marker in the file; everything between them depends on the marker
		// alone, so that is the part worth searching for.
		if len(enc) > 12 {
			forms = append(forms, enc[4:len(enc)-4])
		}
	}
	return forms
}

// incompressible returns n bytes gzip cannot shrink, so the block codec sends
// them raw.
//
// Without this the test could be vacuous in the other direction: a small
// repetitive save compresses, and a compressed marker would not be found even
// in a build sending everything in the clear.
func incompressible(n int) string {
	buf := make([]byte, n)
	rnd := uint32(0x9e3779b9)
	for i := range buf {
		rnd = rnd*1664525 + 1013904223
		buf[i] = byte(rnd >> 24)
	}
	return string(buf)
}

// A save synced between two paired devices must not be readable by a third
// device holding the same room code.
func TestRelayEavesdropper_CannotReadASyncedSave(t *testing.T) {
	relayURL := startRelay(t)
	const room = "eaves-room-1"

	a := testutil.NewTestDaemon(t, "Eaves-A")
	b := testutil.NewTestDaemon(t, "Eaves-B")

	// In the room before anything is synced, so nothing is missed.
	spy := joinAsEavesdropper(t, relayURL, room)
	pairOverRelay(t, a, b, relayURL, room)

	// Two markers, because two different things leak differently. A file name
	// travels as plain JSON in the manifest, so it would appear verbatim; file
	// content travels base64-encoded inside a block message.
	const nameMarker = "MY_REAL_NAME_9f3a"
	const bodyMarker = "PLAYER_PROGRESS_MARKER_7b21"
	saveName := "profiles/" + nameMarker + ".sav"
	content := incompressible(200<<10) + bodyMarker + incompressible(200<<10)

	a.WriteSave(saveName, content)
	gameID := a.TrackGame("EavesGame")
	b.API(http.MethodPost, "/api/games",
		map[string]string{"name": "EavesGame", "savePath": b.SaveDir}, nil)

	syncTo(a, gameID, b.NodeID())
	if !testutil.WaitFor(90*time.Second, func() bool {
		return b.ReadSave(saveName) == content
	}) {
		t.Fatalf("setup: the save never reached the peer, so nothing was on the wire to inspect")
	}

	sealed, plain := spy.sealedFrames()
	t.Logf("third device received %d frames: %d carrying sealed payloads, %d carrying plaintext ones",
		spy.count(), sealed, plain)
	if sealed == 0 {
		t.Fatalf("the third device saw no sealed payload at all across %d frames — either the sync "+
			"did not go through the relay or nothing is being encrypted, and a clean result "+
			"below would mean nothing", spy.count())
	}

	if i, form := spy.leaked(markerForms(bodyMarker)); i >= 0 {
		t.Errorf("a device in the room read the save contents in the clear (frame %d, as %q)", i, form)
	}
	if i, _ := spy.leaked([]string{nameMarker}); i >= 0 {
		t.Errorf("a device in the room read the save's file name in the clear (frame %d); "+
			"names are save data too", i)
	}
}

// The update path, separately. The first sync is mostly manifest exchange; the
// second is where a changed file's blocks actually move, and it is the one a
// regression in block sealing would show up on.
func TestRelayEavesdropper_CannotReadAnUpdatedSave(t *testing.T) {
	relayURL := startRelay(t)
	const room = "eaves-room-2"

	a := testutil.NewTestDaemon(t, "EavesUpd-A")
	b := testutil.NewTestDaemon(t, "EavesUpd-B")
	pairOverRelay(t, a, b, relayURL, room)

	first := incompressible(200 << 10)
	a.WriteSave("slot1.sav", first)
	gameID := a.TrackGame("EavesUpdGame")
	b.API(http.MethodPost, "/api/games",
		map[string]string{"name": "EavesUpdGame", "savePath": b.SaveDir}, nil)
	syncTo(a, gameID, b.NodeID())
	if !testutil.WaitFor(90*time.Second, func() bool { return b.ReadSave("slot1.sav") == first }) {
		t.Fatal("setup: the first sync never landed")
	}

	// Only now start listening, so every frame captured belongs to the update.
	spy := joinAsEavesdropper(t, relayURL, room)

	const bodyMarker = "UPDATED_PROGRESS_MARKER_2c84"
	second := incompressible(200<<10) + bodyMarker + incompressible(200<<10)
	a.WriteSave("slot1.sav", second)
	syncTo(a, gameID, b.NodeID())
	if !testutil.WaitFor(90*time.Second, func() bool { return b.ReadSave("slot1.sav") == second }) {
		t.Fatal("setup: the update never reached the peer")
	}

	sealed, plain := spy.sealedFrames()
	t.Logf("third device received %d update frames: %d sealed, %d plaintext", spy.count(), sealed, plain)
	if sealed == 0 {
		t.Fatalf("no sealed payload crossed the room during the update (%d frames); "+
			"a clean result here would be meaningless", spy.count())
	}
	if i, form := spy.leaked(markerForms(bodyMarker)); i >= 0 {
		t.Errorf("a device in the room read the updated save contents in the clear (frame %d, as %q)",
			i, form)
	}
}
