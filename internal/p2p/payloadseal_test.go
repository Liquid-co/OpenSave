package p2p

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/opensave/opensave/internal/e2ee"
	"github.com/opensave/opensave/internal/store"
)

func TestSealForPeer_RoundTrips(t *testing.T) {
	f := newAuthFixture(t, true)
	plain := []byte(`{"blocks":["a save block"]}`)

	sealed, ok := f.engine.sealForPeer(f.peer.ID, plain)
	if !ok {
		t.Fatal("sealForPeer refused a peer that has a pinned key")
	}
	if bytes.Equal(sealed, plain) {
		t.Fatal("sealForPeer returned the plaintext unchanged")
	}
	back, err := f.engine.openFromPeer(f.peer.ID, sealed)
	if err != nil {
		t.Fatalf("openFromPeer: %v", err)
	}
	if !bytes.Equal(back, plain) {
		t.Fatalf("round trip changed the payload: %q", back)
	}
}

// The point of the whole exercise: the save must not be readable by a room
// member, and the relay hands every frame to every member.
func TestSealForPeer_SaveBytesAreNotOnTheWire(t *testing.T) {
	f := newAuthFixture(t, true)
	secret := []byte(`{"save":"PLAYER_POSITION_SECRET_MARKER"}`)

	sealed, ok := f.engine.sealForPeer(f.peer.ID, secret)
	if !ok {
		t.Fatal("sealForPeer refused a keyed peer")
	}
	msg := RelayMessage{
		Type: "response", From: f.peer.ID, To: "peer-receiver",
		MsgID: "m1", Status: 200, SealedData: sealed,
	}
	wire, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(wire, []byte("PLAYER_POSITION_SECRET_MARKER")) {
		t.Fatal("the save content appears in the relayed frame in the clear")
	}
}

// A device in the room that is not the paired peer must not be able to read it,
// even holding both public keys — which the room broadcasts.
func TestSealForPeer_AnotherRoomMemberCannotDecrypt(t *testing.T) {
	f := newAuthFixture(t, true)
	plain := []byte(`{"save":"content"}`)
	sealed, ok := f.engine.sealForPeer(f.peer.ID, plain)
	if !ok {
		t.Fatal("sealForPeer refused a keyed peer")
	}

	attacker, err := e2ee.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	receiver, err := f.store.DeviceIdentity()
	if err != nil {
		t.Fatal(err)
	}
	wrongKey, err := e2ee.SharedKey(attacker.Private, receiver.Public)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e2ee.Open(wrongKey, sealed); err == nil {
		t.Fatal("an unpaired room member decrypted the payload")
	}
}

// Altering ciphertext must be detected rather than producing garbage that gets
// written to a save folder.
func TestSealForPeer_TamperingIsDetected(t *testing.T) {
	f := newAuthFixture(t, true)
	sealed, ok := f.engine.sealForPeer(f.peer.ID, []byte(`{"save":"content"}`))
	if !ok {
		t.Fatal("sealForPeer refused a keyed peer")
	}
	sealed[len(sealed)-1] ^= 0x01
	if _, err := f.engine.openFromPeer(f.peer.ID, sealed); err == nil {
		t.Fatal("a modified payload was accepted")
	}
}

// A pairing made before keys existed has nothing to seal with, and must keep
// working in plaintext rather than failing.
func TestSealForPeer_LegacyPairingSendsPlaintext(t *testing.T) {
	f := newAuthFixture(t, false)
	if _, ok := f.engine.sealForPeer(f.peer.ID, []byte("payload")); ok {
		t.Fatal("sealForPeer claimed to seal for a peer with no pinned key")
	}
}

// Encrypt-then-MAC: the MAC must be taken over the sealed bytes, so a receiver
// can verify before decrypting. This checks the two halves agree.
func TestSealedRequest_VerifiesWithoutDecryptingFirst(t *testing.T) {
	f := newAuthFixture(t, true)
	plain := []byte(`{"gameId":"g1"}`)
	sealed, ok := f.engine.sealForPeer(f.peer.ID, plain)
	if !ok {
		t.Fatal("sealForPeer refused a keyed peer")
	}

	msg := RelayMessage{
		Type: "request", From: f.peer.ID, To: "peer-receiver",
		Route: "/blocks/g1", Method: "POST", SealedBody: sealed,
		Nonce: "n1", AuthMs: time.Now().UnixMilli(),
	}
	msg.Auth = e2ee.RequestMAC(f.authKey, msg.From, msg.To, msg.Route, msg.Method,
		wireBody(msg.SealedBody, msg.Body), msg.Nonce, msg.AuthMs)

	if outcome, err := f.engine.verifyRequestAuth(f.peer, msg); outcome != authVerified {
		t.Fatalf("a sealed request failed authentication: %v (%v)", outcome, err)
	}
}

// wireBody must prefer the sealed form, or the MAC would be computed over the
// wrong bytes on one side and never match.
func TestWireBody_PrefersSealed(t *testing.T) {
	if got := wireBody([]byte("sealed"), []byte("plain")); string(got) != "sealed" {
		t.Errorf("wireBody = %q, want the sealed bytes", got)
	}
	if got := wireBody(nil, []byte("plain")); string(got) != "plain" {
		t.Errorf("wireBody = %q, want the plaintext when nothing is sealed", got)
	}
}

// Holding a peer's key is not permission to encrypt to it.
//
// The two facts are separate and a pairing routinely has only one of them: we
// pin their key when they hand it over, but whether they pinned ours depends
// on the build they are running. Encrypt on our own key alone and a peer on an
// older build receives a field it does not know, no readable payload, and no
// way to say so — the sync stops, silently, right after an update. That is a
// worse failure than the one encryption is preventing, so sealing waits for
// proof.
//
// The proof is the peer's own authentication: a valid MAC can only come from
// the shared secret, which the peer can only derive from our public key.
func TestSealForPeer_WaitsUntilThePeerHasProvedItCanDecrypt(t *testing.T) {
	f := newAuthFixture(t, true)
	plain := []byte(`{"blocks":["a save block"]}`)

	// A second peer in the state a pairing with an older device sits in: we
	// hold its key, it has never authenticated. Built rather than rolled back,
	// because MarkPeerAuthVerified only ever moves forward — deliberately, so
	// the latch cannot be cleared by anything a peer sends.
	other := store.Peer{
		ID: "peer-not-yet-proved", Name: "Older Device", DeviceType: "desktop",
		Address: "relay", Port: 0, Status: "online",
		PublicKey: e2ee.EncodeKey(f.senderID.Public),
	}
	if err := f.store.UpsertPeer(other); err != nil {
		t.Fatal(err)
	}
	if err := f.store.SetPeerPublicKey(other.ID, other.PublicKey); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.engine.sealForPeer(other.ID, plain); ok {
		t.Error("sealed a payload for a peer that has never proved it can decrypt one; " +
			"a peer on an older build would receive nothing it could read")
	}

	// It authenticates once — now it has demonstrably derived the same
	// secret, so it can open what we send.
	if err := f.store.MarkPeerAuthVerified(other.ID, time.Now().UnixMilli()); err != nil {
		t.Fatal(err)
	}
	sealed, ok := f.engine.sealForPeer(other.ID, plain)
	if !ok {
		t.Fatal("refused to seal for a peer that has authenticated, so nothing would ever be encrypted")
	}
	back, err := f.engine.openFromPeer(other.ID, sealed)
	if err != nil || !bytes.Equal(back, plain) {
		t.Fatalf("round trip failed after the peer proved itself: %v", err)
	}
}

// An unknown peer is not sealed for either. Reached when a message arrives
// from a device that was unpaired between the send and the lookup.
func TestSealForPeer_UnknownPeerIsNotSealedFor(t *testing.T) {
	f := newAuthFixture(t, true)
	if _, ok := f.engine.sealForPeer("node-never-seen", []byte(`{"a":1}`)); ok {
		t.Error("sealed a payload for a peer that is not paired at all")
	}
}

// The key exchange cannot be encrypted with the key it is exchanging.
//
// This is not a preference. The approving device pins the asking device's key
// before it sends its confirmation back, so it can derive a shared secret and
// would encrypt with it — to a recipient whose only source for the matching
// key is the message it has just been made unable to read. Pairing over the
// relay deadlocks, which is exactly what happened the first time the WAN
// handshake began pinning keys.
func TestKeyExchangeRoutesAreNeverSealed(t *testing.T) {
	for _, route := range []string{"/handshake", "/approve-confirm", "/ping"} {
		if !isKeyExchangeRoute(route) {
			t.Errorf("%s would be sealed; pairing over the relay cannot complete if it is", route)
		}
	}
	// Everything carrying save data must still be sealed.
	for _, route := range []string{"/manifest/game1", "/blocks/game1", "/delete-file/save.dat", "/snapshot/game1"} {
		if isKeyExchangeRoute(route) {
			t.Errorf("%s is exempt from sealing, and it carries save data", route)
		}
	}
}

// The badge the app shows must agree with what the send path does.
//
// This is the whole reason the indicator is worth having: an interface that
// says "encrypted" while the sender decided otherwise is worse than saying
// nothing, because it converts an absent protection into a false assurance.
// The two must be computed from the same condition, and this test is what
// stops them drifting apart later.
func TestPeerProtection_MatchesWhatIsActuallySealed(t *testing.T) {
	f := newAuthFixture(t, true)
	payload := []byte(`{"blocks":["x"]}`)

	cases := []struct {
		name     string
		peerID   string
		hasKey   bool
		verified bool
	}{
		{"key and proof", "p-both", true, true},
		{"key, no proof yet", "p-keyonly", true, false},
		{"no key at all", "p-nokey", false, false},
	}
	for _, c := range cases {
		peer := store.Peer{
			ID: c.peerID, Name: c.name, DeviceType: "desktop",
			Address: "relay", Port: 0, Status: "online",
		}
		if c.hasKey {
			peer.PublicKey = e2ee.EncodeKey(f.senderID.Public)
		}
		if err := f.store.UpsertPeer(peer); err != nil {
			t.Fatal(err)
		}
		if c.hasKey {
			if err := f.store.SetPeerPublicKey(c.peerID, peer.PublicKey); err != nil {
				t.Fatal(err)
			}
		}
		if c.verified {
			if err := f.store.MarkPeerAuthVerified(c.peerID, time.Now().UnixMilli()); err != nil {
				t.Fatal(err)
			}
		}

		stored, err := f.store.GetPeer(c.peerID)
		if err != nil {
			t.Fatal(err)
		}
		shown := f.engine.PeerProtection(stored)
		_, actuallySealed := f.engine.sealForPeer(c.peerID, payload)

		if shown.Encrypted != actuallySealed {
			t.Errorf("%s: the app would show encrypted=%v while the sender seals=%v — "+
				"one of them is lying to the user", c.name, shown.Encrypted, actuallySealed)
		}
		if shown.HasKey != c.hasKey {
			t.Errorf("%s: hasKey reported %v, want %v", c.name, shown.HasKey, c.hasKey)
		}
		if shown.Verified != c.verified {
			t.Errorf("%s: verified reported %v, want %v", c.name, shown.Verified, c.verified)
		}
		if !shown.OverRelay {
			t.Errorf("%s: a peer routed through the relay is not reported as such", c.name)
		}
		// A fingerprint exists to be compared across two screens, so it must
		// be there exactly when there is a key behind it.
		if c.hasKey && shown.Fingerprint == "" {
			t.Errorf("%s: no fingerprint for a pairing that has a key, so there is nothing to compare", c.name)
		}
		if !c.hasKey && shown.Fingerprint != "" {
			t.Errorf("%s: a fingerprint was offered for a pairing with no key: %q", c.name, shown.Fingerprint)
		}
	}
}

// A device reached over the local network is reported as such rather than as
// an encryption failure. Nothing passes through a relay on that route, so
// "not encrypted" would be a true statement that gives the wrong impression.
func TestPeerProtection_LocalPeerIsNotReportedAsRelayTraffic(t *testing.T) {
	f := newAuthFixture(t, true)
	peer := store.Peer{
		ID: "p-lan", Name: "Living Room PC", DeviceType: "desktop",
		Address: "192.168.1.24", Port: 8383, Status: "online",
	}
	if err := f.store.UpsertPeer(peer); err != nil {
		t.Fatal(err)
	}
	stored, err := f.store.GetPeer(peer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := f.engine.PeerProtection(stored); got.OverRelay {
		t.Error("a peer at a LAN address is reported as going through a relay")
	}
}
