package p2p

import (
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/opensave/opensave/internal/snapshot"
	"github.com/opensave/opensave/internal/store"
)

func waitUntil(d time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return cond()
}

// A PC switched off since the last run is in the database as "online". It must
// not stay so for the three probes a device seen this run is allowed: the
// first unanswered probe after start says it is not there.
func TestPresence_StaleOnlineFromLastRunClearsAtOnce(t *testing.T) {
	old := presenceInterval
	presenceInterval = time.Hour // only the probe at start
	t.Cleanup(func() { presenceInterval = old })

	s, err := store.Open(filepath.Join(t.TempDir(), "opensave.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.EnsureDefaultSettings(t.TempDir(), t.TempDir()); err != nil {
		t.Fatal(err)
	}
	// Nothing listens here: the PC is off.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	lastRun := time.Now().Add(-10 * time.Hour).UnixMilli()
	if err := s.UpsertPeer(store.Peer{ID: "node_off", Name: "Switched off", Address: "127.0.0.1", Port: port, Status: "online", LastSeenMs: lastRun}); err != nil {
		t.Fatal(err)
	}

	e := New(s, snapshot.New(s), func(string, string) {})
	stop := make(chan struct{})
	start := time.Now()
	e.startPresenceLoop(stop)
	t.Cleanup(func() { close(stop); e.cancel(); e.bgSyncWG.Wait() })

	if !waitUntil(5*time.Second, func() bool {
		p, err := s.GetPeer("node_off")
		return err == nil && p.Status == "offline"
	}) {
		t.Fatal("a peer online only in the last run's record was still online after the first probe")
	}
	t.Logf("shown offline %.1fs after start", time.Since(start).Seconds())
}

// A peer's online state follows it on its own ticker, whatever else the
// engine is doing — not only when a sync or the minute-long reconcile happens
// to probe. A peer reached over a VPN gets no UDP broadcasts, so without this
// its indicator lagged by minutes.
func TestPresenceLoop_FollowsPeer(t *testing.T) {
	old := presenceInterval
	presenceInterval = 50 * time.Millisecond
	t.Cleanup(func() { presenceInterval = old })

	var up atomic.Bool
	up.Store(true)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !up.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","appVersion":"2.4.0"}`))
	}))
	t.Cleanup(srv.Close)
	host, portStr, _ := net.SplitHostPort(srv.Listener.Addr().String())
	port, _ := strconv.Atoi(portStr)

	s, err := store.Open(filepath.Join(t.TempDir(), "opensave.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.EnsureDefaultSettings(t.TempDir(), t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertPeer(store.Peer{ID: "node_vpn", Name: "Over VPN", Address: host, Port: port, Status: "offline"}); err != nil {
		t.Fatal(err)
	}

	e := New(s, snapshot.New(s), func(string, string) {})
	stop := make(chan struct{})
	e.startPresenceLoop(stop)
	t.Cleanup(func() { close(stop); e.cancel(); e.bgSyncWG.Wait() })

	status := func() string {
		p, err := s.GetPeer("node_vpn")
		if err != nil {
			return ""
		}
		return p.Status
	}
	if !waitUntil(3*time.Second, func() bool { return status() == "online" }) {
		t.Fatalf("peer answering pings still %q", status())
	}
	up.Store(false)
	if !waitUntil(3*time.Second, func() bool { return status() == "offline" }) {
		t.Fatalf("peer no longer answering still %q", status())
	}
}
