package pairing

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

// The four-digit check for pairing with a device nearby.
//
// Both devices show four digits worked out from both public keys and a random
// nonce from each side. The user approves only if the two match, so a device
// on the network that swaps the keys in the middle is caught.
//
// Codes from the keys alone could be steered: someone in the middle sees one
// side's code before it picks the key it hands the other side, and can try
// keys until the codes agree, which takes a fraction of a second at four
// digits. So each side's last input stays hidden until the other is fixed:
//
//  1. The asking device sends its key and Commit(its nonce) in the handshake.
//  2. The answering device replies with its key and its own nonce.
//  3. The asking device reveals its nonce, which must match the commitment.
//
// Whoever sits in the middle has to commit to its values on each side before
// it learns that side's nonce, so all it can do is guess: one in 10,000.

const checkSpace = 10000

// NewNonce returns a fresh random nonce for one pairing attempt.
func NewNonce() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

// Commit is what the asking device sends in place of its nonce.
func Commit(nonce string) string {
	sum := sha256.Sum256([]byte("opensave-pair-commit-v1\x00" + nonce))
	return hex.EncodeToString(sum[:])
}

// CheckCode is the four digits both devices show. The roles are fixed, so
// both sides put the same values in the same places.
func CheckCode(askerKey, answererKey, askerNonce, answererNonce string) string {
	h := sha256.New()
	for _, part := range []string{"opensave-pair-check-v1", askerKey, answererKey, askerNonce, answererNonce} {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	n := binary.BigEndian.Uint32(h.Sum(nil)[:4]) % checkSpace
	return fmt.Sprintf("%04d", n)
}
