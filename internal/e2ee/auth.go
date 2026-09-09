package e2ee

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"

	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/hkdf"
)

// Request authentication for the relay path.
//
// The problem: over the relay, a request's sender is whatever the sender says
// it is. The relay is a room broadcaster that never stamps identity, and the
// receiving side authorised a request by looking up the peer ID carried in the
// message. Every device in the room also announces the IDs it is paired with,
// so the one value needed to impersonate a paired device was handed to
// everyone present. The room code was the only thing in the way.
//
// The LAN path has the same shape for a different reason: it authorises by
// source IP, which a device on the same network can take.
//
// The fix uses key material that already exists. Pairing pins each side's
// X25519 public key, so the two devices can derive a secret nobody else can —
// including the relay — and a MAC computed with it proves the sender holds the
// private half. An impersonator has the peer ID but not the key, and cannot
// produce the MAC.
//
// A MAC rather than a signature because X25519 keys agree, they do not sign.
// Deriving a shared secret and authenticating with it proves the same thing
// here: only the two paired devices can compute it, so only they can produce
// or check the value. What it cannot do is prove to a THIRD party which of the
// two sent a message, since either could have. That is irrelevant for this —
// the only party who needs convincing is the peer.

// authHKDFInfo domain-separates the request-auth key from the payload key.
//
// Different purpose, different key, from the same X25519 agreement — the same
// discipline hkdfInfo already applies. Reusing one key for both would mean a
// weakness in either use compromised the other.
const authHKDFInfo = "opensave/e2ee/request-auth/v1"

// AuthKeySize is the length of a derived request-auth key.
const AuthKeySize = 32

// MaxAuthSkew is how far a request's timestamp may be from now.
//
// Wide enough that two machines with ordinary clock drift and no NTP still
// talk to each other, narrow enough that a captured request is not replayable
// for long. Replay inside the window is caught by the nonce cache instead;
// this bound is what stops that cache having to remember forever.
const MaxAuthSkew = 5 * 60 * 1000 // milliseconds

// AuthKey derives the key used to authenticate requests between two paired
// devices.
func AuthKey(myPrivate, theirPublic []byte) ([]byte, error) {
	if len(myPrivate) != KeySize || len(theirPublic) != KeySize {
		return nil, errors.New("both keys must be 32 bytes")
	}
	secret, err := curve25519.X25519(myPrivate, theirPublic)
	if err != nil {
		return nil, fmt.Errorf("key agreement failed: %w", err)
	}
	key := make([]byte, AuthKeySize)
	if _, err := io.ReadFull(hkdf.New(sha256.New, secret, nil, []byte(authHKDFInfo)), key); err != nil {
		return nil, fmt.Errorf("derive request-auth key: %w", err)
	}
	return key, nil
}

// NewNonce returns a random nonce for one request.
func NewNonce() (string, error) {
	raw := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		return "", fmt.Errorf("generate request nonce: %w", err)
	}
	return hex.EncodeToString(raw), nil
}

// RequestMAC computes the authenticator for one request.
//
// Every field that decides what the request DOES is covered: who it claims to
// be from, who it is for, the route, the method, and the body. Leaving any of
// them out would let an attacker who captured one authenticated request replay
// it with that field changed — the same MAC, a different effect. The clearest
// example is `to`: without it, a request captured from a room with three
// devices could be re-aimed at a different one.
//
// Lengths are written before each variable-length field. Without them
// ("ab","c") and ("a","bc") produce identical input, and two different
// requests sharing a MAC is exactly what this must not allow.
func RequestMAC(key []byte, from, to, route, method string, body []byte, nonce string, unixMs int64) string {
	return macOver(key, "request", from, to, route, method, body, nonce, unixMs)
}

// CheckRequestMAC reports whether presented matches what this key would
// produce, compared in constant time.
//
// Constant time because a byte-by-byte comparison leaks how much of a guess
// was right, which over enough attempts recovers the whole value. The relay
// path is remote and an attacker can retry freely, so that is not theoretical.
func CheckRequestMAC(key []byte, from, to, route, method string, body []byte, nonce string, unixMs int64, presented string) bool {
	if presented == "" {
		return false
	}
	want := RequestMAC(key, from, to, route, method, body, nonce, unixMs)
	return hmac.Equal([]byte(want), []byte(presented))
}

// ResponseMAC authenticates a reply.
//
// Replies need this as much as requests do: the relay broadcasts to the whole
// room, so every member sees a request's id and could race an answer to it.
// An accepted forged reply means attacker-chosen bytes written into a save
// folder, which is a worse outcome than one being read.
//
// Domain-separated from RequestMAC by the leading tag, so a captured request
// MAC can never be presented as a reply MAC or the other way round.
func ResponseMAC(key []byte, from, to, msgID string, status int, data []byte, nonce string, unixMs int64) string {
	return macOver(key, "response", from, to, msgID, strconv.Itoa(status), data, nonce, unixMs)
}

// CheckResponseMAC reports whether presented matches, in constant time.
func CheckResponseMAC(key []byte, from, to, msgID string, status int, data []byte, nonce string, unixMs int64, presented string) bool {
	if presented == "" {
		return false
	}
	want := ResponseMAC(key, from, to, msgID, status, data, nonce, unixMs)
	return hmac.Equal([]byte(want), []byte(presented))
}

// macOver is the shared construction: a domain tag, then every field, each
// length-prefixed so no two different field splits can hash the same.
func macOver(key []byte, tag, from, to, third, fourth string, payload []byte, nonce string, unixMs int64) string {
	mac := hmac.New(sha256.New, key)
	write := func(s string) {
		mac.Write([]byte(strconv.Itoa(len(s))))
		mac.Write([]byte{':'})
		mac.Write([]byte(s))
	}
	write(tag)
	write(from)
	write(to)
	write(third)
	write(fourth)
	write(nonce)
	write(strconv.FormatInt(unixMs, 10))
	mac.Write([]byte(strconv.Itoa(len(payload))))
	mac.Write([]byte{':'})
	mac.Write(payload)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}
