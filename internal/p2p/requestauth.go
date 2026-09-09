package p2p

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/opensave/opensave/internal/e2ee"
	"github.com/opensave/opensave/internal/store"
)

// Verification of relay request authentication.
//
// Over the relay a request's "from" is whatever the sender wrote — the relay
// forwards messages within a room and never stamps identity — and the room
// broadcasts every device's paired peer IDs, so the value needed to
// impersonate a paired device is available to anyone in the room. The room
// code was the only thing preventing it.
//
// Requests now carry a MAC derived from the X25519 keys pairing already pins.
// See internal/e2ee/auth.go for the construction and why a MAC rather than a
// signature.

// seenNonces remembers recently accepted nonces so a captured request cannot
// be replayed inside the timestamp window.
//
// The timestamp bound alone is not enough: it stops a request being useful
// tomorrow, not being sent twice in the next minute. For a route like
// /delete-file/ that difference matters.
type nonceCache struct {
	mu      sync.Mutex
	expires map[string]int64 // nonce -> unix ms after which it may be forgotten
	lastGC  int64
}

func newNonceCache() *nonceCache {
	return &nonceCache{expires: map[string]int64{}}
}

// remember records a nonce and reports whether it was new. A repeat is a
// replay and must be refused.
func (n *nonceCache) remember(nonce string, nowMs int64) bool {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Sweep on use rather than on a timer: entries only accumulate while
	// requests arrive, so that is exactly when clearing them is worth doing,
	// and it means no goroutine to own and shut down.
	if nowMs-n.lastGC > 60_000 {
		for k, exp := range n.expires {
			if exp < nowMs {
				delete(n.expires, k)
			}
		}
		n.lastGC = nowMs
	}

	if exp, seen := n.expires[nonce]; seen && exp >= nowMs {
		return false
	}
	// Held for the full skew window: any earlier and a request could be
	// replayed while its timestamp was still considered fresh.
	n.expires[nonce] = nowMs + e2ee.MaxAuthSkew
	return true
}

// authOutcome is what verification concluded, so the caller can log the
// difference between "refused" and "allowed, but this pair is not protected
// yet" without re-deriving it.
type authOutcome int

const (
	authVerified    authOutcome = iota // MAC present and correct
	authNotPossible                    // no pinned key on either side; legacy pairing
	authRefused                        // MAC missing when required, or wrong
)

// verifyRequestAuth checks one relay request against the peer it claims to be
// from.
//
// The rule it implements, and why it is not simply "always require a MAC":
// both devices must be running a build that sends one, and they are not
// upgraded at the same moment. Demanding it outright would break syncing for
// anyone mid-upgrade, which in a program that moves save files looks exactly
// like the failure people fear most. Never demanding it would be no fix at
// all, since an attacker just omits the field.
//
// So it latches. A peer that has ever sent a valid MAC must always send one;
// a peer that never has is treated as it was before. The exposure is the
// window between the two devices upgrading, and it closes by itself the first
// time the upgraded peer sends a request.
func (e *Engine) verifyRequestAuth(peer store.Peer, msg RelayMessage) (authOutcome, error) {
	key, keyErr := e.requestAuthKeyFor(peer)
	hasMAC := strings.TrimSpace(msg.Auth) != ""

	if keyErr != nil {
		// No pinned key, so no MAC could have been produced by either side.
		// A MAC arriving anyway cannot be checked against anything and is
		// treated as absent rather than trusted.
		if peer.AuthVerifiedMs > 0 {
			// This peer has authenticated before, so its key was readable
			// then. Something has changed underneath — a corrupted or cleared
			// key — and quietly dropping to unauthenticated is the one
			// response that would hand an attacker the downgrade.
			return authRefused, fmt.Errorf(
				"peer %s authenticated before but its key is unusable now: %v", peer.ID, keyErr)
		}
		return authNotPossible, nil
	}

	if !hasMAC {
		if peer.AuthVerifiedMs > 0 {
			// Usually a downgrade or a reinstall rather than an attack; the
			// recovery is re-pairing, and nobody will guess that from a bare
			// refusal.
			return authRefused, fmt.Errorf(
				"peer %s has authenticated before, so an unsigned request from it is refused — "+
					"if that device was downgraded or reinstalled, unpair and pair the two again", peer.ID)
		}
		return authNotPossible, nil
	}

	now := time.Now().UnixMilli()
	skew := now - msg.AuthMs
	if skew < 0 {
		skew = -skew
	}
	if msg.AuthMs == 0 || skew > e2ee.MaxAuthSkew {
		return authRefused, fmt.Errorf(
			"request from %s is timestamped %dms away from now", peer.ID, skew)
	}
	if strings.TrimSpace(msg.Nonce) == "" {
		return authRefused, fmt.Errorf("request from %s carries a MAC but no nonce", peer.ID)
	}

	if !e2ee.CheckRequestMAC(key, msg.From, msg.To, msg.Route, msg.Method,
		wireBody(msg.SealedBody, msg.Body), msg.Nonce, msg.AuthMs, msg.Auth) {
		return authRefused, fmt.Errorf("request from %s failed its authentication check", peer.ID)
	}

	// Only after the MAC verifies. Recording the nonce first would let an
	// attacker burn nonces belonging to a real peer, so its next genuine
	// request looked like a replay.
	if !e.nonces().remember(msg.Nonce, now) {
		return authRefused, fmt.Errorf("request from %s reuses a nonce (replay)", peer.ID)
	}

	// Latch, so this pair can no longer be talked down to unauthenticated.
	//
	// Written once, on the transition. Stamping it on every verified message
	// would put a database write in front of every block of every transfer —
	// which it briefly did, and a sync moves thousands of them. The value's
	// only job is "has this peer ever authenticated", so refreshing it buys
	// nothing and costs the hot path.
	if peer.AuthVerifiedMs == 0 {
		e.Log("info", fmt.Sprintf(
			"%q now authenticates its requests; unsigned requests claiming to be it will be refused from here on",
			peer.Name))
		if err := e.Store.MarkPeerAuthVerified(peer.ID, now); err != nil {
			// The request itself is fine — it verified. Failing to persist the
			// latch only means the protection is not yet sticky, which is worth
			// a warning and not worth refusing a legitimate sync over.
			e.Log("warn", "could not record that "+peer.Name+" authenticated: "+err.Error())
		}
	}
	return authVerified, nil
}

// latchIfAuthentic re-checks a message that had to be accepted before its
// sender's key was known, and records the peer as authenticated if it holds up.
//
// Only the pairing confirmation needs this. That message is the one carrying
// the key everything else is checked with, so when it arrives there is nothing
// to check it against and it is let through on that basis. Once the key is
// pinned the same bytes can be verified properly, and doing so immediately is
// what lets encryption start on the very next request instead of the one after
// that.
//
// A failure here does not undo the pairing — the user approved it, and the
// message did what it was sent to do. It is logged because a confirmation that
// cannot be verified against the key it just delivered is worth seeing.
func (e *Engine) latchIfAuthentic(peerID string, msg RelayMessage) {
	peer, err := e.Store.GetPeer(peerID)
	if err != nil {
		return
	}
	if outcome, vErr := e.verifyRequestAuth(peer, msg); outcome == authRefused {
		e.Log("warn", fmt.Sprintf(
			"pairing confirmation from %q did not verify against the key it supplied: %v", peer.Name, vErr))
	}
}

func (e *Engine) nonces() *nonceCache {
	e.nonceOnce.Do(func() { e.nonceCache = newNonceCache() })
	return e.nonceCache
}

// verifyResponseAuth checks a reply from the peer a request was sent to.
//
// Same latch as requests, and the same flag: a peer that has proved it can
// authenticate must keep doing so, in either direction. Returns true when the
// reply may be used.
func (e *Engine) verifyResponseAuth(peerID string, msg RelayMessage) bool {
	peer, err := e.Store.GetPeer(peerID)
	if err != nil {
		// Not a stored peer, so there is no pinned key and nothing to check
		// against. This is the pairing handshake: /handshake and /ping are
		// answered before either side has recorded the other, and refusing
		// their replies here would make pairing itself impossible — which is
		// exactly what it did, breaking every relay test in the suite.
		//
		// Safe because it grants nothing: the routes that read or destroy
		// save data all require pairing on the request side, so an unpaired
		// peer's reply can only ever be to a request that carried no
		// authority in the first place.
		return true
	}
	key, keyErr := e.requestAuthKeyFor(peer)
	if keyErr != nil {
		// Nothing to check against. Refuse only if this peer has authenticated
		// before, which would mean its key went missing rather than never
		// existing.
		return peer.AuthVerifiedMs == 0
	}
	if strings.TrimSpace(msg.Auth) == "" {
		return peer.AuthVerifiedMs == 0
	}

	now := time.Now().UnixMilli()
	skew := now - msg.AuthMs
	if skew < 0 {
		skew = -skew
	}
	if msg.AuthMs == 0 || skew > e2ee.MaxAuthSkew || strings.TrimSpace(msg.Nonce) == "" {
		e.Log("warn", fmt.Sprintf("discarded a reply from %q: stale or missing nonce", peer.Name))
		return false
	}
	if !e2ee.CheckResponseMAC(key, msg.From, msg.To, msg.MsgID, msg.Status,
		wireBody(msg.SealedData, msg.Data), msg.Nonce, msg.AuthMs, msg.Auth) {
		e.Log("warn", fmt.Sprintf("discarded a reply from %q: it failed authentication", peer.Name))
		return false
	}
	if !e.nonces().remember(msg.Nonce, now) {
		e.Log("warn", fmt.Sprintf("discarded a reply from %q: nonce reused (replay)", peer.Name))
		return false
	}
	// Once only — see verifyRequestAuth for why this must not run per message.
	if peer.AuthVerifiedMs == 0 {
		if err := e.Store.MarkPeerAuthVerified(peer.ID, now); err != nil {
			e.Log("warn", "could not record that "+peer.Name+" authenticated: "+err.Error())
		}
	}
	return true
}

// Headers carrying request authentication on the LAN path.
//
// Separate from the relay envelope's JSON fields because the LAN protocol is
// plain HTTP — but the MAC underneath is the same construction, so a device
// proves itself the same way whichever route a sync takes.
const (
	lanAuthPeerHeader  = "X-Opensave-Peer"
	lanAuthNonceHeader = "X-Opensave-Nonce"
	lanAuthTimeHeader  = "X-Opensave-Auth-Ms"
	lanAuthHeader      = "X-Opensave-Auth"
)

// localNodeID is this device's own id, as peers know it.
func (e *Engine) localNodeID() string {
	settings, err := e.Store.GetSettings()
	if err != nil {
		return ""
	}
	return settings.NodeID
}

// verifyLANRequest checks the proof-of-key headers on a LAN request, and
// reports the peer it proves the request came from.
//
// Returns ok=false only when a request is positively bad — a MAC that does not
// verify, a replay, a stale timestamp. A request with no headers at all
// returns ("", true): that is a peer on a build that does not sign yet, and
// the caller falls back to matching the source address as it always did.
//
// The body is read and handed back, because a MAC that did not cover it would
// let a captured request be replayed with different contents. LAN request
// bodies are small — the bulk of a sync is in the responses — so buffering
// costs little.
func (e *Engine) verifyLANRequest(r *http.Request) (peerID string, body []byte, ok bool) {
	claimed := strings.TrimSpace(r.Header.Get(lanAuthPeerHeader))
	mac := strings.TrimSpace(r.Header.Get(lanAuthHeader))

	if r.Body != nil {
		raw, err := io.ReadAll(r.Body)
		r.Body.Close()
		if err != nil {
			return "", nil, false
		}
		body = raw
		r.Body = io.NopCloser(bytes.NewReader(raw))
	}
	if claimed == "" || mac == "" {
		return "", body, true // unsigned: the caller decides
	}

	peer, err := e.Store.GetPeer(claimed)
	if err != nil {
		e.Log("warn", "LAN request claims to be from "+claimed+", which is not a paired device")
		return "", body, false
	}
	key, keyErr := e.requestAuthKeyFor(peer)
	if keyErr != nil {
		// Signed by a peer we hold no key for: nothing to check it against,
		// so it proves nothing. Treated as unsigned rather than trusted.
		return "", body, true
	}

	at, err := strconv.ParseInt(strings.TrimSpace(r.Header.Get(lanAuthTimeHeader)), 10, 64)
	if err != nil {
		return "", body, false
	}
	now := time.Now().UnixMilli()
	skew := now - at
	if skew < 0 {
		skew = -skew
	}
	if skew > e2ee.MaxAuthSkew {
		e.Log("warn", fmt.Sprintf("LAN request from %q is timestamped %dms away from now", peer.Name, skew))
		return "", body, false
	}

	nonce := strings.TrimSpace(r.Header.Get(lanAuthNonceHeader))
	if nonce == "" {
		return "", body, false
	}
	if !e2ee.CheckRequestMAC(key, claimed, e.localNodeID(), r.URL.RequestURI(), r.Method, body, nonce, at, mac) {
		e.Log("warn", fmt.Sprintf("LAN request claiming to be %q failed its authentication check", peer.Name))
		return "", body, false
	}
	// After the MAC verifies, never before — otherwise anyone could burn a
	// real peer's nonces and make its genuine requests look like replays.
	if !e.nonces().remember(nonce, now) {
		e.Log("warn", fmt.Sprintf("LAN request from %q reuses a nonce (replay)", peer.Name))
		return "", body, false
	}
	if peer.AuthVerifiedMs == 0 {
		if err := e.Store.MarkPeerAuthVerified(peer.ID, now); err != nil {
			e.Log("warn", "could not record that "+peer.Name+" authenticated: "+err.Error())
		}
	}
	return peer.ID, body, true
}
