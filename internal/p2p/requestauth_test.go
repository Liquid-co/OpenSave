package p2p

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/opensave/opensave/internal/e2ee"
	"github.com/opensave/opensave/internal/store"
)

// authFixture is one device (the receiver) with one paired peer (the sender),
// plus the key the sender would use.
type authFixture struct {
	engine   *Engine
	store    *store.Store
	peer     store.Peer
	senderID e2ee.Identity
	authKey  []byte
}

// newAuthFixture pairs two devices for real: the receiver's identity comes
// from its own store, so the key the test signs with is the same one
// production would derive.
func newAuthFixture(t *testing.T, withKey bool) *authFixture {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "opensave.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	// DeviceIdentity reads (and lazily writes) the settings row, so the
	// receiver needs one before it has a key of its own.
	if err := s.EnsureDefaultSettings(t.TempDir(), t.TempDir()); err != nil {
		t.Fatalf("EnsureDefaultSettings: %v", err)
	}

	e := &Engine{Store: s, Log: func(string, string) {}}

	sender, err := e2ee.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	peer := store.Peer{
		ID: "peer-sender", Name: "Sender", DeviceType: "desktop",
		Address: "relay", Port: 0, Status: "online",
	}
	if withKey {
		peer.PublicKey = e2ee.EncodeKey(sender.Public)
	}
	if err := s.UpsertPeer(peer); err != nil {
		t.Fatalf("UpsertPeer: %v", err)
	}
	if withKey {
		if err := s.SetPeerPublicKey(peer.ID, peer.PublicKey); err != nil {
			t.Fatalf("SetPeerPublicKey: %v", err)
		}
		// A pairing that has exchanged a message, which is the state nearly
		// every test here means by "paired". Kept separate from the key
		// because the two are genuinely different facts — we hold their key,
		// and they have proved they hold ours — and sealing turns on the
		// second one. See TestSealForPeer_WaitsUntilThePeerHasProvedItCanDecrypt.
		if err := s.MarkPeerAuthVerified(peer.ID, time.Now().UnixMilli()); err != nil {
			t.Fatalf("MarkPeerAuthVerified: %v", err)
		}
	}

	f := &authFixture{engine: e, store: s, senderID: sender}
	if withKey {
		receiver, err := s.DeviceIdentity()
		if err != nil {
			t.Fatalf("DeviceIdentity: %v", err)
		}
		if f.authKey, err = e2ee.AuthKey(sender.Private, receiver.Public); err != nil {
			t.Fatalf("AuthKey: %v", err)
		}
	}
	stored, err := s.GetPeer(peer.ID)
	if err != nil {
		t.Fatalf("GetPeer: %v", err)
	}
	f.peer = stored
	return f
}

func (f *authFixture) reloadPeer(t *testing.T) store.Peer {
	t.Helper()
	p, err := f.store.GetPeer(f.peer.ID)
	if err != nil {
		t.Fatalf("GetPeer: %v", err)
	}
	return p
}

// signedRequest builds a request the paired sender would produce.
func (f *authFixture) signedRequest(route string) RelayMessage {
	body, _ := json.Marshal(map[string]string{"gameId": "g1"})
	msg := RelayMessage{
		Type: "request", From: f.peer.ID, To: "peer-receiver",
		Route: route, Method: "GET", Body: body,
		Nonce: "nonce-0001", AuthMs: time.Now().UnixMilli(),
	}
	msg.Auth = e2ee.RequestMAC(f.authKey, msg.From, msg.To, msg.Route, msg.Method, msg.Body, msg.Nonce, msg.AuthMs)
	return msg
}

func TestRequestAuth_ValidMACIsAccepted(t *testing.T) {
	f := newAuthFixture(t, true)
	outcome, err := f.engine.verifyRequestAuth(f.peer, f.signedRequest("/manifest/g1"))
	if outcome != authVerified {
		t.Fatalf("outcome = %v, err = %v; want authVerified", outcome, err)
	}
}

// The whole point: knowing the peer ID must no longer be enough.
func TestRequestAuth_ImpersonatorWithoutTheKeyIsRefused(t *testing.T) {
	f := newAuthFixture(t, true)

	// An attacker who has read the room's presence traffic knows the peer ID
	// and both public keys, and holds its own private key. Nothing more.
	attacker, err := e2ee.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	receiver, err := f.store.DeviceIdentity()
	if err != nil {
		t.Fatal(err)
	}
	forgedKey, err := e2ee.AuthKey(attacker.Private, receiver.Public)
	if err != nil {
		t.Fatal(err)
	}

	msg := f.signedRequest("/blocks/g1")
	msg.Auth = e2ee.RequestMAC(forgedKey, msg.From, msg.To, msg.Route, msg.Method, msg.Body, msg.Nonce, msg.AuthMs)

	if outcome, _ := f.engine.verifyRequestAuth(f.peer, msg); outcome != authRefused {
		t.Fatalf("an impersonated request was not refused (outcome = %v)", outcome)
	}
}

// Replaying a captured request must fail even though its MAC is genuine and
// its timestamp still fresh.
func TestRequestAuth_ReplayIsRefused(t *testing.T) {
	f := newAuthFixture(t, true)
	msg := f.signedRequest("/delete-file/g1")

	if outcome, err := f.engine.verifyRequestAuth(f.peer, msg); outcome != authVerified {
		t.Fatalf("first send should verify, got %v (%v)", outcome, err)
	}
	peer := f.reloadPeer(t)
	if outcome, _ := f.engine.verifyRequestAuth(peer, msg); outcome != authRefused {
		t.Fatal("the same request was accepted twice; replay protection is not working")
	}
}

func TestRequestAuth_StaleTimestampIsRefused(t *testing.T) {
	f := newAuthFixture(t, true)
	msg := f.signedRequest("/manifest/g1")
	// Re-sign at an old timestamp, so the MAC is valid and only the age is wrong.
	msg.AuthMs = time.Now().UnixMilli() - e2ee.MaxAuthSkew - 60_000
	msg.Auth = e2ee.RequestMAC(f.authKey, msg.From, msg.To, msg.Route, msg.Method, msg.Body, msg.Nonce, msg.AuthMs)

	if outcome, _ := f.engine.verifyRequestAuth(f.peer, msg); outcome != authRefused {
		t.Fatal("a request far outside the clock-skew window was accepted")
	}
}

// A captured request must not be re-aimed at a different route.
func TestRequestAuth_RouteCannotBeSwapped(t *testing.T) {
	f := newAuthFixture(t, true)
	msg := f.signedRequest("/manifest/g1")
	msg.Route = "/delete-file/g1" // MAC still covers the original route

	if outcome, _ := f.engine.verifyRequestAuth(f.peer, msg); outcome != authRefused {
		t.Fatal("a request with a rewritten route was accepted")
	}
}

// A pairing made before end-to-end encryption existed has no key to derive
// from. It must keep working rather than being locked out by an upgrade.
func TestRequestAuth_LegacyPairingStillWorks(t *testing.T) {
	f := newAuthFixture(t, false)
	msg := RelayMessage{
		Type: "request", From: f.peer.ID, To: "peer-receiver",
		Route: "/manifest/g1", Method: "GET",
	}
	outcome, err := f.engine.verifyRequestAuth(f.peer, msg)
	if outcome != authNotPossible {
		t.Fatalf("outcome = %v, err = %v; want authNotPossible", outcome, err)
	}
}

// The latch: once a peer has authenticated, an unsigned request claiming to be
// it must be refused. Without this an attacker just omits the MAC.
func TestRequestAuth_LatchesSoAuthCannotBeDroppedLater(t *testing.T) {
	f := newAuthFixture(t, true)

	if outcome, err := f.engine.verifyRequestAuth(f.peer, f.signedRequest("/manifest/g1")); outcome != authVerified {
		t.Fatalf("first request should verify, got %v (%v)", outcome, err)
	}
	peer := f.reloadPeer(t)
	if peer.AuthVerifiedMs == 0 {
		t.Fatal("a verified request did not record the latch")
	}

	unsigned := RelayMessage{
		Type: "request", From: peer.ID, To: "peer-receiver",
		Route: "/blocks/g1", Method: "GET",
	}
	if outcome, _ := f.engine.verifyRequestAuth(peer, unsigned); outcome != authRefused {
		t.Fatal("an unsigned request was accepted from a peer that had already authenticated")
	}
}

// A peer that has authenticated before but whose key has since become
// unreadable must be refused, not quietly downgraded.
func TestRequestAuth_LatchedPeerWithBrokenKeyIsRefused(t *testing.T) {
	f := newAuthFixture(t, true)
	if outcome, _ := f.engine.verifyRequestAuth(f.peer, f.signedRequest("/manifest/g1")); outcome != authVerified {
		t.Fatal("setup: first request should verify")
	}
	peer := f.reloadPeer(t)
	peer.PublicKey = "" // key lost or cleared underneath us

	unsigned := RelayMessage{From: peer.ID, To: "peer-receiver", Route: "/blocks/g1", Method: "GET"}
	if outcome, _ := f.engine.verifyRequestAuth(peer, unsigned); outcome != authRefused {
		t.Fatal("a latched peer with an unusable key was allowed through unauthenticated")
	}
}

func TestRequestAuth_MACWithoutNonceIsRefused(t *testing.T) {
	f := newAuthFixture(t, true)
	msg := f.signedRequest("/manifest/g1")
	msg.Nonce = ""
	if outcome, _ := f.engine.verifyRequestAuth(f.peer, msg); outcome != authRefused {
		t.Fatal("a MAC with no nonce was accepted")
	}
}

// A failed verification must not consume the nonce, or an attacker could burn
// a real peer's nonces and have its genuine requests read as replays.
func TestRequestAuth_FailedCheckDoesNotBurnTheNonce(t *testing.T) {
	f := newAuthFixture(t, true)
	genuine := f.signedRequest("/manifest/g1")

	bad := genuine
	bad.Auth = "not-a-valid-mac"
	if outcome, _ := f.engine.verifyRequestAuth(f.peer, bad); outcome != authRefused {
		t.Fatal("setup: a bad MAC should be refused")
	}
	if outcome, err := f.engine.verifyRequestAuth(f.peer, genuine); outcome != authVerified {
		t.Fatalf("the genuine request was rejected after a forgery used its nonce: %v (%v)", outcome, err)
	}
}

// signedResponse builds the reply the paired peer would send.
func (f *authFixture) signedResponse(msgID string, status int) RelayMessage {
	data, _ := json.Marshal(map[string]string{"ok": "yes"})
	msg := RelayMessage{
		Type: "response", From: f.peer.ID, To: "peer-receiver",
		MsgID: msgID, Status: status, Data: data,
		Nonce: "resp-nonce-1", AuthMs: time.Now().UnixMilli(),
	}
	msg.Auth = e2ee.ResponseMAC(f.authKey, msg.From, msg.To, msg.MsgID, msg.Status, msg.Data, msg.Nonce, msg.AuthMs)
	return msg
}

func TestResponseAuth_ValidReplyIsAccepted(t *testing.T) {
	f := newAuthFixture(t, true)
	if !f.engine.verifyResponseAuth(f.peer.ID, f.signedResponse("m1", 200)) {
		t.Fatal("a correctly authenticated reply was rejected")
	}
}

// The reason responses need this at all: the relay broadcasts to the room, so
// any member sees a request id and can race an answer carrying save content.
func TestResponseAuth_ForgedReplyIsRejected(t *testing.T) {
	f := newAuthFixture(t, true)
	attacker, err := e2ee.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	receiver, err := f.store.DeviceIdentity()
	if err != nil {
		t.Fatal(err)
	}
	forgedKey, _ := e2ee.AuthKey(attacker.Private, receiver.Public)

	msg := f.signedResponse("m1", 200)
	msg.Auth = e2ee.ResponseMAC(forgedKey, msg.From, msg.To, msg.MsgID, msg.Status, msg.Data, msg.Nonce, msg.AuthMs)

	if f.engine.verifyResponseAuth(f.peer.ID, msg) {
		t.Fatal("a forged reply was accepted; attacker-chosen bytes could reach a save folder")
	}
}

// A reply's data must be covered, or a genuine reply could be captured and its
// payload swapped.
func TestResponseAuth_DataCannotBeSwapped(t *testing.T) {
	f := newAuthFixture(t, true)
	msg := f.signedResponse("m1", 200)
	msg.Data = json.RawMessage(`{"ok":"no"}`)
	if f.engine.verifyResponseAuth(f.peer.ID, msg) {
		t.Fatal("a reply with rewritten data was accepted")
	}
}

// A request MAC must not be presentable as a response MAC.
func TestResponseAuth_RequestMACIsNotAcceptedAsAResponse(t *testing.T) {
	f := newAuthFixture(t, true)
	msg := f.signedResponse("m1", 200)
	msg.Auth = e2ee.RequestMAC(f.authKey, msg.From, msg.To, msg.MsgID, "200", msg.Data, msg.Nonce, msg.AuthMs)
	if f.engine.verifyResponseAuth(f.peer.ID, msg) {
		t.Fatal("a request MAC was accepted as a reply MAC; the domains are not separated")
	}
}

// A pairing with no key must keep working, as with requests.
func TestResponseAuth_LegacyPairingStillWorks(t *testing.T) {
	f := newAuthFixture(t, false)
	msg := RelayMessage{Type: "response", From: f.peer.ID, To: "peer-receiver", MsgID: "m1", Status: 200}
	if !f.engine.verifyResponseAuth(f.peer.ID, msg) {
		t.Fatal("a legacy pairing's unauthenticated reply was rejected")
	}
}

// Once a peer has authenticated, an unsigned reply from it must be refused.
func TestResponseAuth_LatchesAfterFirstVerifiedReply(t *testing.T) {
	f := newAuthFixture(t, true)
	if !f.engine.verifyResponseAuth(f.peer.ID, f.signedResponse("m1", 200)) {
		t.Fatal("setup: the first reply should verify")
	}
	unsigned := RelayMessage{Type: "response", From: f.peer.ID, To: "peer-receiver", MsgID: "m2", Status: 200}
	if f.engine.verifyResponseAuth(f.peer.ID, unsigned) {
		t.Fatal("an unsigned reply was accepted after the peer had authenticated")
	}
}

// Pairing answers /handshake and /ping before either device has recorded the
// other. Refusing those replies makes pairing impossible — it broke every
// relay test in the suite, and none of the unit tests noticed because they all
// start from an already-paired fixture.
func TestResponseAuth_ReplyFromAnUnpairedPeerIsAllowed(t *testing.T) {
	f := newAuthFixture(t, true)
	msg := RelayMessage{
		Type: "response", From: "node_never_paired", To: "peer-receiver",
		MsgID: "m1", Status: 200,
	}
	if !f.engine.verifyResponseAuth("node_never_paired", msg) {
		t.Fatal("a reply from a not-yet-paired device was dropped; pairing cannot complete")
	}
}

// lanRequest builds a signed LAN request the paired sender would send.
func (f *authFixture) lanRequest(t *testing.T, method, target string, body []byte) *http.Request {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, target, rdr)
	local, err := f.store.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	nonce := "lan-nonce-1"
	at := time.Now().UnixMilli()
	req.Header.Set(lanAuthPeerHeader, f.peer.ID)
	req.Header.Set(lanAuthNonceHeader, nonce)
	req.Header.Set(lanAuthTimeHeader, strconv.FormatInt(at, 10))
	req.Header.Set(lanAuthHeader,
		e2ee.RequestMAC(f.authKey, f.peer.ID, local.NodeID, req.URL.RequestURI(), method, body, nonce, at))
	return req
}

func TestLANAuth_ValidRequestIdentifiesThePeer(t *testing.T) {
	f := newAuthFixture(t, true)
	id, _, ok := f.engine.verifyLANRequest(f.lanRequest(t, http.MethodGet, "/api/p2p/manifest/g1", nil))
	if !ok || id != f.peer.ID {
		t.Fatalf("verifyLANRequest = %q, %v; want the peer id and ok", id, ok)
	}
}

// The point: being at the right address is no longer enough.
func TestLANAuth_ForgedMACIsRefused(t *testing.T) {
	f := newAuthFixture(t, true)
	req := f.lanRequest(t, http.MethodGet, "/api/p2p/manifest/g1", nil)
	req.Header.Set(lanAuthHeader, "not-the-right-mac")
	if _, _, ok := f.engine.verifyLANRequest(req); ok {
		t.Fatal("a forged LAN MAC was accepted")
	}
}

// A captured request must not be replayable at a more destructive route.
func TestLANAuth_RouteCannotBeSwapped(t *testing.T) {
	f := newAuthFixture(t, true)
	req := f.lanRequest(t, http.MethodGet, "/api/p2p/manifest/g1", nil)
	req.URL.Path = "/api/p2p/delete-file/g1"
	if _, _, ok := f.engine.verifyLANRequest(req); ok {
		t.Fatal("a LAN request with a rewritten route was accepted")
	}
}

// The body is covered, and must be handed back intact for the handler.
func TestLANAuth_BodyIsCoveredAndPreserved(t *testing.T) {
	f := newAuthFixture(t, true)
	body := []byte(`{"relPath":"save.dat"}`)
	req := f.lanRequest(t, http.MethodPost, "/api/p2p/delete-file/g1", body)

	id, got, ok := f.engine.verifyLANRequest(req)
	if !ok || id != f.peer.ID {
		t.Fatalf("a signed POST was refused: %q %v", id, ok)
	}
	if string(got) != string(body) {
		t.Errorf("body returned as %q, want it intact", got)
	}
	rest, _ := io.ReadAll(req.Body)
	if string(rest) != string(body) {
		t.Errorf("r.Body was consumed: handler would read %q", rest)
	}

	tampered := f.lanRequest(t, http.MethodPost, "/api/p2p/delete-file/g1", body)
	tampered.Body = io.NopCloser(bytes.NewReader([]byte(`{"relPath":"other.dat"}`)))
	if _, _, ok := f.engine.verifyLANRequest(tampered); ok {
		t.Fatal("a swapped body was accepted")
	}
}

func TestLANAuth_ReplayIsRefused(t *testing.T) {
	f := newAuthFixture(t, true)
	req := f.lanRequest(t, http.MethodGet, "/api/p2p/manifest/g1", nil)
	if _, _, ok := f.engine.verifyLANRequest(req); !ok {
		t.Fatal("setup: first request should verify")
	}
	again := f.lanRequest(t, http.MethodGet, "/api/p2p/manifest/g1", nil)
	again.Header.Set(lanAuthNonceHeader, "lan-nonce-1")
	// Re-sign so only the nonce reuse is wrong.
	local, _ := f.store.GetSettings()
	at, _ := strconv.ParseInt(again.Header.Get(lanAuthTimeHeader), 10, 64)
	again.Header.Set(lanAuthHeader, e2ee.RequestMAC(f.authKey, f.peer.ID, local.NodeID,
		again.URL.RequestURI(), http.MethodGet, nil, "lan-nonce-1", at))
	if _, _, ok := f.engine.verifyLANRequest(again); ok {
		t.Fatal("a replayed LAN request was accepted")
	}
}

// An unsigned request is not refused outright — that is a peer on an older
// build, and the address check still covers it.
func TestLANAuth_UnsignedRequestIsLeftToTheAddressCheck(t *testing.T) {
	f := newAuthFixture(t, true)
	req := httptest.NewRequest(http.MethodGet, "/api/p2p/manifest/g1", nil)
	id, _, ok := f.engine.verifyLANRequest(req)
	if !ok {
		t.Fatal("an unsigned request was refused outright; older peers would stop syncing")
	}
	if id != "" {
		t.Errorf("an unsigned request proved an identity: %q", id)
	}
}
