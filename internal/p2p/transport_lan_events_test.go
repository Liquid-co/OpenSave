package p2p

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/opensave/opensave/internal/p2p/syncengine"
)

// Reports reach a peer in the order they were made. A quick sync's "started"
// arriving after its "finished" left the peer showing a sync that was over.
func TestSyncEventReportsArriveInOrder(t *testing.T) {
	var mu sync.Mutex
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			EventType string `json:"eventType"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		// A little uneven, the way a real network is, so that reports sent
		// concurrently would overtake each other.
		if n, _ := strconv.Atoi(body.EventType[1:]); n%3 == 0 {
			time.Sleep(3 * time.Millisecond)
		}
		mu.Lock()
		got = append(got, body.EventType)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	host, portStr, _ := net.SplitHostPort(srv.Listener.Addr().String())
	port, _ := strconv.Atoi(portStr)
	peer := syncengine.Peer{ID: "peer1", Name: "Deck", Address: host, Port: port}

	tr := &lanTransport{}
	const n = 40
	for i := 0; i < n; i++ {
		tr.ReportSyncEvent(peer, "hades", fmt.Sprintf("e%d", i), nil)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		mu.Lock()
		done := len(got) == n
		mu.Unlock()
		if done || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) != n {
		t.Fatalf("received %d of %d reports", len(got), n)
	}
	for i, e := range got {
		if e != fmt.Sprintf("e%d", i) {
			t.Fatalf("report %d arrived as %s: %v", i, e, got)
		}
	}
}
