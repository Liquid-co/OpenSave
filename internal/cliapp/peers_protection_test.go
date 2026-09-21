package cliapp

import (
	"encoding/json"
	"strings"
	"testing"
)

// The terminal must describe a pairing's protection exactly as the app does.
//
// There are three copies of this rule — the daemon's PeerProtection, the
// app's protection.js, and this one — and a terminal user and an app user
// looking at the same pairing must get the same answer. Each case below is
// the same fixture the app's protection.test.js uses.
func TestPeerProtection_MatchesTheApp(t *testing.T) {
	prev := colorEnabled
	colorEnabled = false // plain words, no glyphs or escapes, so the strings are comparable
	defer func() { colorEnabled = prev }()

	cases := []struct {
		name string
		row  string
		want string
	}{
		{"relay, key, proved", `{"address":"relay","overRelay":true,"hasKey":true,"encrypted":true}`, "encrypted"},
		{"relay, key, not proved yet", `{"address":"relay","overRelay":true,"hasKey":true,"encrypted":false}`, "encrypting shortly"},
		{"relay, no key", `{"address":"relay","overRelay":true,"hasKey":false,"encrypted":false}`, "not encrypted"},
		{"local network", `{"address":"192.168.1.5","port":8383,"overRelay":false,"hasKey":true,"encrypted":false}`, "direct"},
	}
	for _, c := range cases {
		var p peerRow
		if err := json.Unmarshal([]byte(c.row), &p); err != nil {
			t.Fatal(err)
		}
		if got := p.protection(); got != c.want {
			t.Errorf("%s: printed %q, app shows %q", c.name, got, c.want)
		}
	}
}

// A daemon from before these fields existed has not said the pairing is
// unprotected; it has said nothing. Printing "not encrypted" would turn
// silence into an accusation. It must say it does not know.
func TestPeerProtection_OldDaemonIsUnknownNotUnencrypted(t *testing.T) {
	prev := colorEnabled
	colorEnabled = false
	defer func() { colorEnabled = prev }()

	var p peerRow
	if err := json.Unmarshal([]byte(`{"name":"Deck","status":"online","address":"relay"}`), &p); err != nil {
		t.Fatal(err)
	}
	got := p.protection()
	if strings.Contains(got, "not encrypted") {
		t.Fatalf("a relay pairing on a daemon that reports no protection fields printed %q", got)
	}
	if got != "unknown" {
		t.Errorf("printed %q, want %q", got, "unknown")
	}
	if p.needsRepair() {
		t.Error("an old daemon's pairing was flagged for re-pairing on no evidence")
	}

	// And a local pairing on that same old daemon is simply direct, as the
	// app's address fallback also concludes.
	if err := json.Unmarshal([]byte(`{"name":"PC","address":"10.0.0.2","port":8383}`), &p); err != nil {
		t.Fatal(err)
	}
	if got := p.protection(); got != "direct" {
		t.Errorf("old daemon, LAN address: printed %q, want %q", got, "direct")
	}
}

// Only a relay pairing with no key is worth telling someone to re-pair; the
// pending state clears itself, and a local pairing has nothing to repair.
func TestPeerProtection_RepairHintOnlyWhenRepairHelps(t *testing.T) {
	for _, c := range []struct {
		row  string
		want bool
	}{
		{`{"address":"relay","overRelay":true,"hasKey":false,"encrypted":false}`, true},
		{`{"address":"relay","overRelay":true,"hasKey":true,"encrypted":false}`, false},
		{`{"address":"relay","overRelay":true,"hasKey":true,"encrypted":true}`, false},
		{`{"address":"192.168.1.5","overRelay":false,"hasKey":false,"encrypted":false}`, false},
	} {
		var p peerRow
		_ = json.Unmarshal([]byte(c.row), &p)
		if got := p.needsRepair(); got != c.want {
			t.Errorf("%s: needsRepair = %v, want %v", c.row, got, c.want)
		}
	}
}
