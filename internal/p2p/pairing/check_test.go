package pairing

import (
	"errors"
	"regexp"
	"testing"
	"time"
)

func TestCheckCode_IsFourDigitsAndStable(t *testing.T) {
	code := CheckCode("key-a", "key-b", "nonce-a", "nonce-b")
	if !regexp.MustCompile(`^\d{4}$`).MatchString(code) {
		t.Fatalf("code %q is not four digits", code)
	}
	if again := CheckCode("key-a", "key-b", "nonce-a", "nonce-b"); again != code {
		t.Fatalf("same inputs gave %q then %q", code, again)
	}
	// The parts are separated, so moving a character between two of them
	// is a different input.
	if code == CheckCode("key-ak", "ey-b", "nonce-a", "nonce-b") {
		t.Fatal("shifting a character between parts gave the same code")
	}
	// And the roles are not interchangeable.
	if code == CheckCode("key-b", "key-a", "nonce-b", "nonce-a") {
		t.Fatal("swapping the roles gave the same code")
	}
}

func pending(m *Manager, nonce string) {
	m.RecordIncoming(IncomingRequest{
		PeerID: "laptop", DeviceName: "Laptop", Address: "192.168.1.20", Port: 8383,
		PublicKey: "key-laptop", Checks: true, Commit: Commit(nonce), Nonce: "nonce-here", OwnKey: "key-here",
	})
}

func TestReveal_OnlyTheCommittedNonceSetsTheCode(t *testing.T) {
	m := New()
	pending(m, "the-real-nonce")

	if _, ok := m.Reveal("laptop", "192.168.1.20", "another-nonce"); ok {
		t.Fatal("a nonce the handshake did not commit to was taken")
	}
	if _, ok := m.Reveal("laptop", "192.168.1.66", "the-real-nonce"); ok {
		t.Fatal("the nonce was taken from another address")
	}
	if got := m.PendingRequests()[0].CheckCode; got != "" {
		t.Fatalf("a refused reveal left the code %q", got)
	}

	code, ok := m.Reveal("laptop", "192.168.1.20", "the-real-nonce")
	want := CheckCode("key-laptop", "key-here", "the-real-nonce", "nonce-here")
	if !ok || code != want {
		t.Fatalf("Reveal = %q, %v; want %q, true", code, ok, want)
	}
	if got := m.PendingRequests()[0].CheckCode; got != want {
		t.Fatalf("the request shows %q, want %q", got, want)
	}
}

func TestReveal_RefusedForARequestWithoutTheCheck(t *testing.T) {
	m := New()
	m.RecordIncoming(IncomingRequest{PeerID: "old", Address: "192.168.1.30", PublicKey: "key-old"})
	if _, ok := m.Reveal("old", "192.168.1.30", ""); ok {
		t.Fatal("a request from a version without the check took a reveal")
	}
	if _, ok := m.Reveal("nobody", "192.168.1.30", "x"); ok {
		t.Fatal("a reveal for no request was taken")
	}
}

func TestTakeChecked_RefusesACodeTheUserWasNotShown(t *testing.T) {
	m := New()
	pending(m, "n1")
	code, _ := m.Reveal("laptop", "192.168.1.20", "n1")

	stale := "0000"
	if code == stale {
		stale = "0001"
	}
	if _, err := m.TakeChecked("laptop", stale); !errors.Is(err, ErrCheckChanged) {
		t.Fatalf("approving with another code: err = %v, want ErrCheckChanged", err)
	}
	if !m.HasIncoming("laptop") {
		t.Fatal("a refused approval consumed the request")
	}
	if _, err := m.TakeChecked("laptop", code); err != nil {
		t.Fatalf("approving with the code shown: %v", err)
	}
	if _, err := m.TakeChecked("laptop", code); !errors.Is(err, ErrNoRequest) {
		t.Fatalf("second approval: err = %v, want ErrNoRequest", err)
	}
}

func TestTakeChecked_AcceptsAnApprovalThatShowedNoCode(t *testing.T) {
	m := New()
	pending(m, "n1")
	m.Reveal("laptop", "192.168.1.20", "n1")
	if _, err := m.TakeChecked("laptop", ""); err != nil {
		t.Fatalf("approval without a code (the CLI's): %v", err)
	}
}

func TestKeyMismatch_PinsTheKeyTheHandshakeWasAnsweredWith(t *testing.T) {
	m := New()
	now := time.Now()
	m.now = func() time.Time { return now }
	m.RecordSent("192.168.1.40", "192.168.1.40:8383")
	m.ExpectKey("key-deck", "192.168.1.40", "192.168.1.40:8383")

	if m.KeyMismatch("deck", "192.168.1.40", 8383, "key-deck") {
		t.Fatal("the key it answered with was refused")
	}
	if !m.KeyMismatch("deck", "192.168.1.40", 8383, "key-other") {
		t.Fatal("another key was let through")
	}
	if !m.KeyMismatch("deck", "192.168.1.40", 8383, "") {
		t.Fatal("a confirm without a key was let through after one was answered with")
	}

	// Once the grace window has gone the request is dead anyway, and
	// ValidateConfirm turns it away.
	now = now.Add(graceWindow + time.Second)
	if m.KeyMismatch("deck", "192.168.1.40", 8383, "key-other") {
		t.Fatal("an expired attempt still pinned a key")
	}
}

func TestKeyMismatch_NewAttemptForgetsTheLastAnswer(t *testing.T) {
	m := New()
	m.RecordSent("192.168.1.40", "192.168.1.40:8383")
	m.ExpectKey("key-before", "192.168.1.40", "192.168.1.40:8383")
	// Asked again, and answered by a version that sends no key.
	m.RecordSent("192.168.1.40", "192.168.1.40:8383")
	if m.KeyMismatch("deck", "192.168.1.40", 8383, "key-after") {
		t.Fatal("the last attempt's key was held against this one")
	}
}

func TestValidateConfirm_ConsumesTheExpectedKey(t *testing.T) {
	m := New()
	m.RecordSent("192.168.1.40", "192.168.1.40:8383")
	m.ExpectKey("key-deck", "192.168.1.40", "192.168.1.40:8383")
	if !m.ValidateConfirm("deck", "192.168.1.40", 8383, false) {
		t.Fatal("the confirm was refused")
	}
	if len(m.expected) != 0 {
		t.Fatalf("expected keys left behind: %v", m.expected)
	}
}
