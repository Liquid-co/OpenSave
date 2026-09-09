package e2ee

import (
	"strings"
	"testing"
)

func pair(t *testing.T) (a, b Identity) {
	t.Helper()
	var err error
	if a, err = GenerateIdentity(); err != nil {
		t.Fatal(err)
	}
	if b, err = GenerateIdentity(); err != nil {
		t.Fatal(err)
	}
	return a, b
}

// Both devices must derive the same key from opposite halves, or nothing
// either of them signs will verify on the other side.
func TestAuthKey_BothSidesAgree(t *testing.T) {
	a, b := pair(t)
	ka, err := AuthKey(a.Private, b.Public)
	if err != nil {
		t.Fatal(err)
	}
	kb, err := AuthKey(b.Private, a.Public)
	if err != nil {
		t.Fatal(err)
	}
	if string(ka) != string(kb) {
		t.Fatal("the two devices derived different request-auth keys")
	}
	if len(ka) != AuthKeySize {
		t.Errorf("key is %d bytes, want %d", len(ka), AuthKeySize)
	}
}

// The auth key must not be the payload key. Same agreement, different purpose:
// if they were equal, a weakness in one use would carry into the other.
func TestAuthKey_IsNotThePayloadKey(t *testing.T) {
	a, b := pair(t)
	auth, err := AuthKey(a.Private, b.Public)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := SharedKey(a.Private, b.Public)
	if err != nil {
		t.Fatal(err)
	}
	if string(auth) == string(payload) {
		t.Fatal("request-auth key equals the payload key; the HKDF info is not separating them")
	}
}

// A third device holding neither private half must not be able to produce a
// MAC the pair accepts. This is the whole point.
func TestRequestMAC_AnotherDeviceCannotForge(t *testing.T) {
	a, b := pair(t)
	attacker, err := GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}

	good, err := AuthKey(a.Private, b.Public)
	if err != nil {
		t.Fatal(err)
	}
	// The attacker knows both public keys — they are broadcast — and its own
	// private key. That is the most it can have.
	forged, err := AuthKey(attacker.Private, b.Public)
	if err != nil {
		t.Fatal(err)
	}

	body := []byte(`{"gameId":"g1"}`)
	mac := RequestMAC(forged, "peer-a", "peer-b", "/blocks/g1", "POST", body, "n1", 1000)
	if CheckRequestMAC(good, "peer-a", "peer-b", "/blocks/g1", "POST", body, "n1", 1000, mac) {
		t.Fatal("a MAC from an unpaired device was accepted")
	}
}

func TestRequestMAC_RoundTrips(t *testing.T) {
	a, b := pair(t)
	ka, _ := AuthKey(a.Private, b.Public)
	kb, _ := AuthKey(b.Private, a.Public)

	body := []byte(`{"gameId":"g1"}`)
	mac := RequestMAC(ka, "peer-a", "peer-b", "/manifest/g1", "GET", body, "nonce", 1234)
	if !CheckRequestMAC(kb, "peer-a", "peer-b", "/manifest/g1", "GET", body, "nonce", 1234, mac) {
		t.Fatal("a MAC made by one side did not verify on the other")
	}
}

// Every field the MAC covers must actually change it. A field left out of the
// computation is a field an attacker can rewrite on a captured request.
func TestRequestMAC_CoversEveryField(t *testing.T) {
	a, b := pair(t)
	key, _ := AuthKey(a.Private, b.Public)

	base := func() string {
		return RequestMAC(key, "from", "to", "/route", "GET", []byte("body"), "nonce", 100)
	}
	original := base()

	cases := []struct {
		name string
		mac  string
	}{
		{"from", RequestMAC(key, "OTHER", "to", "/route", "GET", []byte("body"), "nonce", 100)},
		{"to", RequestMAC(key, "from", "OTHER", "/route", "GET", []byte("body"), "nonce", 100)},
		{"route", RequestMAC(key, "from", "to", "/OTHER", "GET", []byte("body"), "nonce", 100)},
		{"method", RequestMAC(key, "from", "to", "/route", "POST", []byte("body"), "nonce", 100)},
		{"body", RequestMAC(key, "from", "to", "/route", "GET", []byte("OTHER"), "nonce", 100)},
		{"nonce", RequestMAC(key, "from", "to", "/route", "GET", []byte("body"), "OTHER", 100)},
		{"timestamp", RequestMAC(key, "from", "to", "/route", "GET", []byte("body"), "nonce", 999)},
	}
	for _, c := range cases {
		if c.mac == original {
			t.Errorf("changing %q did not change the MAC — that field is unauthenticated", c.name)
		}
	}
}

// Length-prefixing: two different field splits must not produce one MAC.
// Without it ("ab","c") and ("a","bc") hash identically.
func TestRequestMAC_FieldBoundariesAreUnambiguous(t *testing.T) {
	a, b := pair(t)
	key, _ := AuthKey(a.Private, b.Public)

	one := RequestMAC(key, "ab", "c", "/r", "GET", nil, "n", 1)
	two := RequestMAC(key, "a", "bc", "/r", "GET", nil, "n", 1)
	if one == two {
		t.Fatal("two different requests share a MAC; the fields are not delimited")
	}
}

func TestCheckRequestMAC_RejectsEmpty(t *testing.T) {
	a, b := pair(t)
	key, _ := AuthKey(a.Private, b.Public)
	if CheckRequestMAC(key, "f", "t", "/r", "GET", nil, "n", 1, "") {
		t.Fatal("an empty MAC was accepted")
	}
}

func TestNewNonce_IsRandomAndLongEnough(t *testing.T) {
	seen := map[string]bool{}
	for range make([]struct{}, 500) {
		n, err := NewNonce()
		if err != nil {
			t.Fatal(err)
		}
		if len(n) != 32 {
			t.Fatalf("nonce %q is %d hex chars, want 32 (16 bytes)", n, len(n))
		}
		if seen[n] {
			t.Fatalf("nonce %q repeated", n)
		}
		seen[n] = true
	}
}

func TestAuthKey_RejectsMalformedKeys(t *testing.T) {
	a, _ := pair(t)
	if _, err := AuthKey(a.Private, []byte("short")); err == nil {
		t.Error("expected an error for a short public key")
	}
	if _, err := AuthKey([]byte("short"), a.Public); err == nil {
		t.Error("expected an error for a short private key")
	}
	// An all-zero public key is a low-order point: X25519 must reject it
	// rather than agreeing on a predictable secret.
	if _, err := AuthKey(a.Private, make([]byte, KeySize)); err == nil {
		t.Error("expected an error for an all-zero public key")
	} else if !strings.Contains(err.Error(), "agreement") {
		t.Errorf("unexpected error for a low-order point: %v", err)
	}
}
