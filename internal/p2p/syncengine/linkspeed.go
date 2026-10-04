package syncengine

import (
	"context"
	"fmt"
	"net"
	"sort"
	"time"

	"github.com/opensave/opensave/internal/store"
)

// Network-aware syncing.
//
// Devices are not equally close. Two on the same home network move a save at
// tens of MB a second; one reached over a VPN relay or the internet relay may
// manage a hundred KB. A save that goes to every device at once spends hours
// crawling over the slow links while the device next door, which could have
// had it in seconds and then handed it on, waits in the same queue.
//
// So each device keeps how fast it has found its connection to every other —
// measured from the transfers it makes, and from a short speed test when it
// has nothing recent — and works fastest first: a sync goes to the close
// devices first, and a device that is behind takes the newer save from the
// fastest device that holds it. Before anything is measured, the kind of
// address stands in: a home-network address is taken for fast, a VPN address
// (Tailscale's range) for slower, the internet relay for slowest.

// LinkStat is what this device knows about its connection to a peer.
type LinkStat struct {
	// BytesPerSec is the measured rate, 0 when nothing is measured yet.
	BytesPerSec float64 `json:"bytesPerSec"`
	MeasuredMs  int64   `json:"measuredMs"`
	// Kind is what the address says: "lan", "vpn", "relay" or "internet".
	Kind string `json:"kind"`
}

const (
	// minMeasureBytes and minMeasureTime: smaller transfers say more about
	// round trips than about the link, and are not counted.
	minMeasureBytes = 512 << 10
	minMeasureTime  = 500 * time.Millisecond
	// probeEvery is how often a link is tested again when no transfer has
	// measured it since; probeBytes how much a test moves.
	probeEvery = 6 * time.Hour
	probeBytes = 2 << 20
	// probeRetry is how soon a speed test that failed is tried again.
	probeRetry = 10 * time.Minute
)

// prior rates, by address kind, used until a link is measured.
var priorRate = map[string]float64{
	"lan":      40 << 20,
	"vpn":      2 << 20,
	"internet": 1 << 20,
	"relay":    256 << 10,
}

var (
	_, tailscaleNet, _ = net.ParseCIDR("100.64.0.0/10")
	privateNets        = func() []*net.IPNet {
		var out []*net.IPNet
		for _, c := range []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "169.254.0.0/16", "fc00::/7", "fe80::/10"} {
			_, n, _ := net.ParseCIDR(c)
			out = append(out, n)
		}
		return out
	}()
)

// linkKind says what a peer's address is.
func linkKind(peer Peer) string {
	if peer.Wan() {
		return "relay"
	}
	ip := net.ParseIP(peer.Address)
	if ip == nil {
		return "internet"
	}
	if ip.IsLoopback() {
		return "lan"
	}
	if tailscaleNet.Contains(ip) {
		return "vpn"
	}
	for _, n := range privateNets {
		if n.Contains(ip) {
			return "lan"
		}
	}
	return "internet"
}

func (e *Engine) loadLinksLocked() {
	if e.links != nil {
		return
	}
	e.links = map[string]store.PeerLink{}
	if e.Store == nil {
		return
	}
	if links, err := e.Store.PeerLinks(); err == nil {
		e.links = links
	}
}

// Link is what this device knows about its connection to a peer.
func (e *Engine) Link(peer Peer) LinkStat {
	e.linkMu.Lock()
	defer e.linkMu.Unlock()
	e.loadLinksLocked()
	l := e.links[peer.ID]
	return LinkStat{BytesPerSec: l.BytesPerSec, MeasuredMs: l.MeasuredMs, Kind: linkKind(peer)}
}

// expectedRate is how fast a transfer with a peer is expected to go: what was
// measured, or the address kind's guess.
func (e *Engine) expectedRate(peer Peer) float64 {
	l := e.Link(peer)
	if l.BytesPerSec > 0 {
		return l.BytesPerSec
	}
	return priorRate[l.Kind]
}

// noteLinkRate records a transfer with a peer: bytes in d.
func (e *Engine) noteLinkRate(peer Peer, bytes int64, d time.Duration) {
	e.recordLinkRate(peer, bytes, d, minMeasureTime)
}

// recordLinkRate records bytes in d, when d is at least minTime.
func (e *Engine) recordLinkRate(peer Peer, bytes int64, d, minTime time.Duration) {
	if bytes < minMeasureBytes || d < minTime || d <= 0 {
		return
	}
	rate := float64(bytes) / d.Seconds()
	e.linkMu.Lock()
	defer e.linkMu.Unlock()
	e.loadLinksLocked()
	l := e.links[peer.ID]
	l.PeerID = peer.ID
	if l.BytesPerSec > 0 {
		// A running average, weighted to the latest: a link changes (a VPN
		// that found a direct path, a laptop that moved networks), and the
		// order should follow it within a transfer or two.
		l.BytesPerSec = 0.4*l.BytesPerSec + 0.6*rate
	} else {
		l.BytesPerSec = rate
	}
	l.MeasuredMs = time.Now().UnixMilli()
	e.links[peer.ID] = l
	if e.Store != nil {
		_ = e.Store.SavePeerLink(l)
	}
}

// OrderBySpeed sorts peers fastest first, keeping the given order among
// equals.
func (e *Engine) OrderBySpeed(peers []Peer) []Peer {
	out := append([]Peer(nil), peers...)
	rates := make(map[string]float64, len(out))
	for _, p := range out {
		rates[p.ID] = e.expectedRate(p)
	}
	sort.SliceStable(out, func(i, j int) bool { return rates[out[i].ID] > rates[out[j].ID] })
	return out
}

// SpeedProber is implemented by transports that can test a link: fetch n
// bytes of nothing in particular from the peer and say how long it took.
type SpeedProber interface {
	ProbeSpeed(ctx context.Context, peer Peer, n int) (int64, time.Duration, error)
}

// ProbeLinkIfDue tests the link to a peer when nothing has measured it for a
// while. Not over the internet relay, which is the slowest link whatever a
// test says and would only be loaded further. Reports whether it ran.
func (e *Engine) ProbeLinkIfDue(ctx context.Context, peer Peer) bool {
	prober, ok := e.Transport.(SpeedProber)
	if !ok || peer.Wan() {
		return false
	}
	e.linkMu.Lock()
	e.loadLinksLocked()
	l := e.links[peer.ID]
	now := time.Now()
	// A link never measured is tried again soon after a test that failed;
	// one measured before only when that has gone stale.
	retryAfter := probeEvery
	if l.BytesPerSec == 0 {
		retryAfter = probeRetry
	}
	due := now.Sub(time.UnixMilli(l.MeasuredMs)) > probeEvery && now.Sub(time.UnixMilli(l.ProbedMs)) > retryAfter
	if due {
		l.PeerID = peer.ID
		l.ProbedMs = now.UnixMilli()
		e.links[peer.ID] = l
		if e.Store != nil {
			_ = e.Store.SavePeerLink(l)
		}
	}
	e.linkMu.Unlock()
	if !due {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	n, d, err := prober.ProbeSpeed(ctx, peer, probeBytes)
	if err == nil && d < minMeasureTime {
		// Over a home network the test is over before it measures much:
		// once more, larger, for a figure worth having.
		n, d, err = prober.ProbeSpeed(ctx, peer, 4*probeBytes)
	}
	if err != nil {
		// A link never measured is tried again after probeRetry (see due
		// above): the peer may simply not have been ready — or not yet on a
		// build that answers a speed test, which once left every link
		// unmeasured for hours.
		e.Log("info", fmt.Sprintf("speed test with %s did not finish: %v", peer.Name, err))
		return true
	}
	// A test is a measurement however short: it moved nothing but the test,
	// so no round trips of a sync's own are in it.
	e.recordLinkRate(peer, n, d, time.Millisecond)
	e.Log("info", fmt.Sprintf("connection to %s: %s/s (%s)", peer.Name, humanBytes(int64(e.Link(peer).BytesPerSec)), linkKind(peer)))
	return true
}

// Waiting for close devices before slow ones.

// fastLink is the rate from which a device counts as close, and slowerBy how
// much slower the next device must be for this one to be waited for first.
const (
	fastLink   = 4 << 20
	slowerBy   = 4.0
	maxWaitFor = 20 * time.Minute
)

// NotePeerPulled is told that a peer finished (or gave up) pulling a game from
// this device, and lets a sync waiting for it go on.
func (e *Engine) NotePeerPulled(gameID, peerID string) {
	e.linkMu.Lock()
	ch := e.pullWaits[gameID+"|"+peerID]
	delete(e.pullWaits, gameID+"|"+peerID)
	e.linkMu.Unlock()
	if ch != nil {
		close(ch)
	}
}

// waitForPull waits, after asking a close peer to take this device's save,
// until it has — before the slow devices are asked, so the save reaches the
// close one at full speed and the slow ones can then take it from whichever
// device is closest to them. Bounded by how long the transfer should take,
// and given up when the context ends.
func (e *Engine) waitForPull(ctx context.Context, gameID string, peer Peer, bytes int64) {
	ch := make(chan struct{})
	e.linkMu.Lock()
	if e.pullWaits == nil {
		e.pullWaits = map[string]chan struct{}{}
	}
	e.pullWaits[gameID+"|"+peer.ID] = ch
	e.linkMu.Unlock()
	defer func() {
		e.linkMu.Lock()
		if e.pullWaits[gameID+"|"+peer.ID] == ch {
			delete(e.pullWaits, gameID+"|"+peer.ID)
		}
		e.linkMu.Unlock()
	}()
	wait := 30*time.Second + time.Duration(3*float64(bytes)/e.expectedRate(peer)*float64(time.Second))
	if wait > maxWaitFor {
		wait = maxWaitFor
	}
	select {
	case <-ch:
	case <-time.After(wait):
		e.Log("info", fmt.Sprintf("%s has not finished taking %s yet; going on with the other devices", peer.Name, gameID))
	case <-ctx.Done():
	}
}

// waitsForClose reports whether, having asked peers[i] to take this device's
// save, the sync should wait for it before going on: it is a close device,
// and a much slower one is still to come.
func (e *Engine) waitsForClose(peers []Peer, i int) bool {
	r := e.expectedRate(peers[i])
	if r < fastLink {
		return false
	}
	for _, p := range peers[i+1:] {
		if e.expectedRate(p)*slowerBy <= r {
			return true
		}
	}
	return false
}
