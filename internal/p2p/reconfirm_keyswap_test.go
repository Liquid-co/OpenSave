package p2p

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/opensave/opensave/internal/e2ee"
	"github.com/opensave/opensave/internal/p2p/pairing"
)

// An approve-confirm that no handshake of ours stands behind, naming a paired
// device's id with a key of its own. The id is public — every device in a
// relay room sees it — so this is what anyone in the room, or the relay
// itself, could send.
func keySwapBody(t *testing.T, f *authFixture) []byte {
	t.Helper()
	attacker, err := e2ee.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{
		"peerId": f.peer.ID, "deviceName": "Sender", "port": 9999,
		"publicKey": e2ee.EncodeKey(attacker.Public),
	})
	return body
}

func assertUnchanged(t *testing.T, f *authFixture) {
	t.Helper()
	got, err := f.store.GetPeer(f.peer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.PublicKey != f.peer.PublicKey {
		t.Error("the paired device's pinned key was replaced")
	}
	if got.Address != f.peer.Address || got.Port != f.peer.Port {
		t.Errorf("the paired device's address moved to %s:%d", got.Address, got.Port)
	}
}

func TestApproveConfirm_LANRefusesAKeySwap(t *testing.T) {
	f := newAuthFixture(t, true)
	f.engine.Pairing = pairing.New()
	r := chi.NewRouter()
	f.engine.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/api/p2p/approve-confirm", strings.NewReader(string(keySwapBody(t, f))))
	req.RemoteAddr = "192.0.2.66:40000"
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Errorf("status = %d, want the key swap refused", rec.Code)
	}
	assertUnchanged(t, f)
}

func TestApproveConfirm_WANRefusesAKeySwap(t *testing.T) {
	f := newAuthFixture(t, true)
	f.engine.Pairing = pairing.New()
	w := &WanClient{engine: f.engine}

	status, _ := w.routeRequest(t.Context(), RelayMessage{
		Type: "request", From: f.peer.ID, Route: "/approve-confirm", Method: http.MethodPost,
		Body: keySwapBody(t, f),
	})
	if status == http.StatusOK {
		t.Errorf("status = %d, want the key swap refused", status)
	}
	assertUnchanged(t, f)
}

// A repeat of a confirm already applied — same key — is still answered.
func TestApproveConfirm_RepeatWithThePinnedKeyIsAnswered(t *testing.T) {
	f := newAuthFixture(t, true)
	f.engine.Pairing = pairing.New()
	w := &WanClient{engine: f.engine}
	body, _ := json.Marshal(map[string]any{"peerId": f.peer.ID, "deviceName": "Sender", "publicKey": f.peer.PublicKey})

	if status, _ := w.routeRequest(t.Context(), RelayMessage{
		Type: "request", From: f.peer.ID, Route: "/approve-confirm", Method: http.MethodPost, Body: body,
	}); status != http.StatusOK {
		t.Errorf("status = %d, want 200", status)
	}
	assertUnchanged(t, f)
}

// A confirm this device asked for — the user started a pairing with it —
// still pins the key it carries.
func TestApproveConfirm_AHandshakeWeStartedStillPins(t *testing.T) {
	f := newAuthFixture(t, false)
	f.engine.Pairing = pairing.New()
	f.engine.Pairing.RecordSent(f.peer.ID)
	w := &WanClient{engine: f.engine}
	key, _ := e2ee.GenerateIdentity()
	body, _ := json.Marshal(map[string]any{"peerId": f.peer.ID, "deviceName": "Sender", "publicKey": e2ee.EncodeKey(key.Public)})

	if status, _ := w.routeRequest(t.Context(), RelayMessage{
		Type: "request", From: f.peer.ID, Route: "/approve-confirm", Method: http.MethodPost, Body: body,
	}); status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if got, _ := f.store.GetPeer(f.peer.ID); got.PublicKey != e2ee.EncodeKey(key.Public) {
		t.Error("the key from a handshake we started was not pinned")
	}
}
