package p2p

import (
	"fmt"
	"strings"

	"github.com/opensave/opensave/internal/e2ee"
	"github.com/opensave/opensave/internal/store"
)

// End-to-end encryption of relay payloads.
//
// The relay forwards every frame to every member of the room — its reader loop
// says so plainly — so "who can read your saves" was never just the relay
// operator. It was everyone you had ever given the room code to, for every
// game, whether or not you had paired with them. Pairing decides what a device
// may act on; it does not decide what arrives at it.
//
// The key material for closing that has existed since pairing started pinning
// public keys: SharedKeyWith derives a secret only the two paired devices can
// compute. Until now nothing called it outside tests, so e2ee.Seal was written,
// tested, and never used. This is what uses it.
//
// Only the payload is sealed. Type, To, From and MsgID stay readable because
// the receiving client needs them to match a reply to its request before it
// could possibly decrypt anything, and Route stays readable because the
// request has to be dispatched. So a room member still learns that two devices
// are talking and roughly about what; it no longer learns the save.

// sealForPeer encrypts a payload for one peer.
//
// Reports false when there is no shared key — a pairing made before key
// exchange existed. The caller sends plaintext then, exactly as before, rather
// than failing a sync that has always worked.
func (e *Engine) sealForPeer(peerID string, plain []byte) ([]byte, bool) {
	if len(plain) == 0 {
		return nil, false
	}
	if !e.peerCanOpenSealed(peerID) {
		return nil, false
	}
	key, ok, err := e.Store.SharedKeyWith(peerID)
	if err != nil || !ok {
		return nil, false
	}
	sealed, err := e2ee.Seal(key, plain)
	if err != nil {
		// Refusing to fall back to plaintext here is deliberate. This path is
		// only reached when a key exists, so a failure is a real fault, and
		// quietly sending the save in the clear instead is the one outcome
		// worth avoiding.
		e.Log("error", fmt.Sprintf("could not encrypt a payload for %s: %v", peerID, err))
		return nil, false
	}
	return sealed, true
}

// openFromPeer decrypts a payload sealed by one peer.
func (e *Engine) openFromPeer(peerID string, sealed []byte) ([]byte, error) {
	key, ok, err := e.Store.SharedKeyWith(peerID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("no shared key with %s to decrypt with", peerID)
	}
	return e2ee.Open(key, sealed)
}

// wireBody is the payload as it travels: the sealed form when there is one,
// the plaintext otherwise.
//
// The MAC is taken over this rather than over the plaintext, so a receiver can
// verify a message before decrypting it — encrypt-then-MAC, which is the order
// that does not require touching unauthenticated ciphertext.
func wireBody(sealed, plain []byte) []byte {
	if len(sealed) > 0 {
		return sealed
	}
	return plain
}

// isKeyExchangeRoute reports whether a route carries the key exchange itself
// and so must travel unsealed.
//
// Sealing these would be a deadlock, not a hardening. /approve-confirm is the
// message that delivers a device's public key to the peer that is waiting for
// it; the approving side has by then pinned the asking side's key, so it can
// derive a shared secret and would happily encrypt with it — to a recipient
// whose only way of learning the matching key is the message it cannot open.
// Pairing over the relay then fails outright, which is exactly what happened
// the first time the WAN handshake started pinning keys at all.
//
// Nothing is given away by leaving them clear. A public key is public, and the
// device name, type and port beside it are already broadcast to the room as
// presence. What sealing protects is save data, and none travels here.
//
// /ping is included for a different reason: it is answered before a pairing
// exists, so one side routinely holds a key the other does not.
func isKeyExchangeRoute(route string) bool {
	switch route {
	case "/handshake", "/approve-confirm", "/ping":
		return true
	}
	return false
}

// peerCanOpenSealed reports whether a peer has proved it holds the shared key,
// and so can decrypt what we seal for it.
//
// Holding a key for a peer is not the same as that peer holding one for us,
// and the difference is not academic — it is the ordinary state of a pairing
// between a device that has been updated and one that has not. Encrypting
// on the strength of our own key alone would send that peer a payload it has
// no field for, no key for, and no way to report a problem about: the sync
// would simply stop working, and "my saves stopped syncing after I updated"
// is a worse outcome than a payload an eavesdropper could read.
//
// The evidence used is the peer's own authentication. A valid MAC can only be
// produced from the shared secret, which the peer can only derive from our
// public key, which it can only have pinned on a build that reads one. So the
// latch that already exists for authentication answers this question too,
// cryptographically rather than by comparing version strings — which a fork,
// a development build, or a repackaged binary would all get wrong.
//
// The cost is that the first request to a freshly paired peer may go
// unsealed, in the moment before it has answered anything. Approving a
// pairing closes that window immediately where it can; see the
// approve-confirm handler.
func (e *Engine) peerCanOpenSealed(peerID string) bool {
	peer, err := e.Store.GetPeer(peerID)
	if err != nil {
		return false
	}
	return peer.AuthVerifiedMs > 0
}

// PeerProtection is what actually protects traffic with one peer, as opposed
// to what the app would like to be true.
//
// It exists because the difference was invisible. Encryption over the relay
// depends on a key exchanged during pairing, and for a long time pairing over
// a relay did not keep that key — so a pairing that was encrypted and one that
// was not looked exactly alike in the app, on the page that tells people the
// relay cannot read their saves. Whatever the state is, the honest thing is to
// show it per device rather than describe the general case in a paragraph and
// leave the reader to work out which half applies to them.
type PeerProtection struct {
	// Encrypted reports that payloads to this peer are actually being sealed.
	// Both halves are required: a key to seal with, and evidence the peer can
	// open the result.
	Encrypted bool `json:"encrypted"`
	// HasKey reports that a key was exchanged when the two devices paired.
	// False means re-pairing is the only way to get one.
	HasKey bool `json:"hasKey"`
	// Verified reports that this peer has proved it holds the matching key, so
	// requests claiming to be from it are checked rather than taken on trust.
	Verified bool `json:"verified"`
	// OverRelay reports that traffic with this peer currently goes through a
	// relay, which is what makes encryption matter rather than merely nice.
	OverRelay bool `json:"overRelay"`
	// Fingerprint is the short string both devices show for this pairing, for
	// comparing out of band. Empty when there is no key.
	Fingerprint string `json:"fingerprint,omitempty"`
}

// PeerProtection reports the protection in force for one paired device.
func (e *Engine) PeerProtection(peer store.Peer) PeerProtection {
	p := PeerProtection{
		HasKey:    strings.TrimSpace(peer.PublicKey) != "",
		Verified:  peer.AuthVerifiedMs > 0,
		OverRelay: peer.Address == "relay",
	}
	// The same condition sealForPeer applies, deliberately: a badge that says
	// "encrypted" while the send path decides otherwise is worse than no badge
	// at all.
	p.Encrypted = p.HasKey && p.Verified
	if p.HasKey {
		if fp, err := e.Store.PairingFingerprint(peer.ID); err == nil {
			p.Fingerprint = fp
		}
	}
	return p
}
