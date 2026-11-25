package syncengine

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestLinkKind(t *testing.T) {
	cases := map[string]string{
		"192.168.178.20": "lan",
		"10.0.0.5":       "lan",
		"127.0.0.1":      "lan",
		"100.99.121.82":  "vpn",
		"203.0.113.9":    "internet",
	}
	for addr, want := range cases {
		if got := linkKind(Peer{Address: addr}); got != want {
			t.Errorf("%s: %s, want %s", addr, got, want)
		}
	}
	if got := linkKind(Peer{Address: "relay"}); got != "relay" {
		t.Errorf("relay: %s", got)
	}
}

// Before anything is measured the kind of address orders the devices; once a
// link is measured, the measurement does.
func TestOrderBySpeed(t *testing.T) {
	env := setupEngine(t)
	e := env.engine
	home := Peer{ID: "home", Address: "192.168.178.20"}
	vpn := Peer{ID: "vpn", Address: "100.99.121.82"}
	relay := Peer{ID: "relay", Address: "relay"}

	got := e.OrderBySpeed([]Peer{relay, vpn, home})
	if got[0].ID != "home" || got[1].ID != "vpn" || got[2].ID != "relay" {
		t.Errorf("by address: %v", ids(got))
	}

	// A VPN link that turns out fast, and a home one that turns out slow.
	e.noteLinkRate(vpn, 200<<20, 2*time.Second)
	e.noteLinkRate(home, 1<<20, 10*time.Second)
	got = e.OrderBySpeed([]Peer{home, vpn, relay})
	if got[0].ID != "vpn" {
		t.Errorf("by measurement: %v", ids(got))
	}
	// Remembered across restarts.
	e2 := New(env.store, nil, env.transport)
	if e2.Link(vpn).BytesPerSec == 0 {
		t.Error("the measured link was not kept")
	}
}

func TestNoteLinkRate_IgnoresTinyTransfers(t *testing.T) {
	env := setupEngine(t)
	p := Peer{ID: "p", Address: "192.168.1.2"}
	env.engine.noteLinkRate(p, 10<<10, 2*time.Second)
	if env.engine.Link(p).BytesPerSec != 0 {
		t.Error("a transfer of a few KB was taken as the link's speed")
	}
}

// A close device is waited for before a much slower one, and the wait ends as
// soon as it reports that it has finished.
func TestWaitForClose(t *testing.T) {
	env := setupEngine(t)
	e := env.engine
	home := Peer{ID: "home", Address: "192.168.178.20"}
	vpn := Peer{ID: "vpn", Address: "100.99.121.82"}
	peers := e.OrderBySpeed([]Peer{vpn, home})
	if !e.waitsForClose(peers, 0) {
		t.Fatal("a close device is not waited for before a slow one")
	}
	if e.waitsForClose(peers, 1) {
		t.Error("the last device is waited for, with nothing after it")
	}
	if e.waitsForClose(e.OrderBySpeed([]Peer{home}), 0) {
		t.Error("waited with no slower device to follow")
	}

	done := make(chan struct{})
	start := time.Now()
	go func() {
		e.waitForPull(context.Background(), "game1", home, 1<<30)
		close(done)
	}()
	time.Sleep(100 * time.Millisecond)
	e.NotePeerPulled("game1", home.ID)
	select {
	case <-done:
		if time.Since(start) > 5*time.Second {
			t.Error("the wait did not end when the device reported")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the wait did not end when the device reported")
	}
}

type probeTransport struct {
	*fakeTransport
	fail  bool
	calls int
	// perMB is how long each MB takes; a second when unset.
	perMB time.Duration
}

func (p *probeTransport) ProbeSpeed(ctx context.Context, peer Peer, n int) (int64, time.Duration, error) {
	p.calls++
	if p.fail {
		return 0, 0, errors.New("peer returned 404")
	}
	if p.perMB > 0 {
		return int64(n), time.Duration(n>>20) * p.perMB, nil
	}
	return int64(n), time.Second, nil
}

// A speed test that fails — a peer not yet updated to answer one — is tried
// again after minutes, not hours; a measured link is not tested again soon.
func TestProbeLink_RetriesAFailureSoon(t *testing.T) {
	env := setupEngine(t)
	pt := &probeTransport{fakeTransport: env.transport, fail: true}
	env.engine.Transport = pt
	p := Peer{ID: "p", Name: "P", Address: "192.168.1.9"}

	if !env.engine.ProbeLinkIfDue(context.Background(), p) {
		t.Fatal("an unmeasured link was not tested")
	}
	if env.engine.ProbeLinkIfDue(context.Background(), p) {
		t.Fatal("tested again straight after a failure")
	}
	// Eleven minutes later.
	env.engine.linkMu.Lock()
	l := env.engine.links[p.ID]
	l.ProbedMs = time.Now().Add(-11 * time.Minute).UnixMilli()
	env.engine.links[p.ID] = l
	env.engine.linkMu.Unlock()
	pt.fail = false
	if !env.engine.ProbeLinkIfDue(context.Background(), p) {
		t.Fatal("a failed test was not tried again after ten minutes")
	}
	if env.engine.Link(p).BytesPerSec == 0 {
		t.Fatal("the successful test was not recorded")
	}
	if env.engine.ProbeLinkIfDue(context.Background(), p) {
		t.Error("a freshly measured link was tested again")
	}
}

// A home network finishes the test before it has measured much: it is run
// again larger, and the result is kept — not discarded as too short, which
// left the fastest link unmeasured and sorted behind the slow ones.
func TestProbeLink_AFastLinkIsMeasured(t *testing.T) {
	env := setupEngine(t)
	pt := &probeTransport{fakeTransport: env.transport, perMB: 20 * time.Millisecond}
	env.engine.Transport = pt
	p := Peer{ID: "lan", Name: "LAN", Address: "192.168.178.20"}
	env.engine.ProbeLinkIfDue(context.Background(), p)
	if got := env.engine.Link(p).BytesPerSec; got < 20<<20 {
		t.Errorf("a 50 MB/s link measured as %.0f B/s", got)
	}
	if pt.calls != 2 {
		t.Errorf("%d test(s), want the short one and a larger one", pt.calls)
	}
}

func ids(ps []Peer) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.ID
	}
	return out
}
