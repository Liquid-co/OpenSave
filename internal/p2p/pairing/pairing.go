// Package pairing implements the P2P pairing handshake state:
// incoming requests awaiting user approval, and a 2-minute grace window
// guarding /approve-confirm against unsolicited spoofing — ported from the
// handshake logic in src/daemon/p2p/routes.js.
package pairing

import (
	"errors"
	"sync"
	"time"
)

// graceWindow is how long an initiated handshake stays valid for the
// counterpart's approve-confirm.
const graceWindow = 2 * time.Minute

// IncomingRequest is a handshake from another device awaiting the local
// user's approval.
type IncomingRequest struct {
	PeerID     string `json:"peerId"`
	DeviceName string `json:"deviceName"`
	DeviceType string `json:"deviceType"`
	Address    string `json:"address"`
	Port       int    `json:"port"`
	ReceivedAt int64  `json:"receivedAt"` // unix ms
	IsWan      bool   `json:"isWan"`
	// PublicKey is the asking device's X25519 public half, held until the
	// user approves and it can be pinned. Empty from a build without
	// end-to-end encryption, which pairs exactly as before.
	PublicKey string `json:"-"`

	// Checks says the asking device shows the four-digit check (see
	// check.go), and CheckCode is the code once its nonce has arrived. A
	// request from an older version has neither.
	Checks    bool   `json:"checks"`
	CheckCode string `json:"checkCode,omitempty"`
	// Commit is the asking device's commitment to its nonce, Nonce is ours.
	Commit string `json:"-"`
	Nonce  string `json:"-"`
	// OwnKey is the key this device answered with, kept so the code is
	// worked out from the same one.
	OwnKey string `json:"-"`
}

// ErrNoRequest is returned when no handshake from the peer is waiting.
var ErrNoRequest = errors.New("no pending pairing request")

// ErrCheckChanged is returned when the user approves a request whose code is
// not the one they were shown: a newer handshake replaced it.
var ErrCheckChanged = errors.New("the pairing request changed, check the numbers again")

// Manager is safe for concurrent use.
type Manager struct {
	mu       sync.Mutex
	incoming map[string]IncomingRequest // keyed by peerId
	sent     map[string]time.Time       // keys: peerId, ip, "ip:port"
	// expected holds the key the device we asked answered the handshake
	// with, under the same keys as sent. Its approve-confirm must carry
	// the same key, or the code the user compared was for a different one.
	expected map[string]string
	now      func() time.Time
}

// New creates a Manager.
func New() *Manager {
	return &Manager{
		incoming: map[string]IncomingRequest{},
		sent:     map[string]time.Time{},
		expected: map[string]string{},
		now:      time.Now,
	}
}

// RecordIncoming stores (or refreshes) a handshake awaiting approval.
func (m *Manager) RecordIncoming(req IncomingRequest) {
	m.mu.Lock()
	defer m.mu.Unlock()
	req.ReceivedAt = m.now().UnixMilli()
	m.incoming[req.PeerID] = req
}

// TakeIncoming removes and returns a pending request (approval or
// rejection consumes it).
func (m *Manager) TakeIncoming(peerID string) (IncomingRequest, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	req, ok := m.incoming[peerID]
	if ok {
		delete(m.incoming, peerID)
	}
	return req, ok
}

// TakeChecked is TakeIncoming for an approval. shownCode is the code the user
// saw when they approved; if the request carries a different one, a newer
// handshake replaced the one they checked, and it is left waiting. An empty
// shownCode (an approval that showed no code, such as the CLI's) is accepted.
func (m *Manager) TakeChecked(peerID, shownCode string) (IncomingRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	req, ok := m.incoming[peerID]
	if !ok {
		return IncomingRequest{}, ErrNoRequest
	}
	if shownCode != "" && req.CheckCode != shownCode {
		return IncomingRequest{}, ErrCheckChanged
	}
	delete(m.incoming, peerID)
	return req, nil
}

// Reveal takes the asking device's nonce. It must match the commitment its
// handshake carried and come from the same address; then the request gets
// its code. A second reveal cannot change a code already set.
func (m *Manager) Reveal(peerID, address, nonce string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	req, ok := m.incoming[peerID]
	if !ok || !req.Checks || req.Address != address || nonce == "" || Commit(nonce) != req.Commit {
		return "", false
	}
	if req.CheckCode == "" {
		req.CheckCode = CheckCode(req.PublicKey, req.OwnKey, nonce, req.Nonce)
		m.incoming[peerID] = req
	}
	return req.CheckCode, true
}

// HasIncoming reports whether a handshake from this peer is pending.
func (m *Manager) HasIncoming(peerID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.incoming[peerID]
	return ok
}

// PendingRequests lists handshakes awaiting user approval.
func (m *Manager) PendingRequests() []IncomingRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]IncomingRequest, 0, len(m.incoming))
	for _, req := range m.incoming {
		out = append(out, req)
	}
	return out
}

// RecordSent marks that we initiated pairing toward the given keys
// (peer id and/or address forms), opening the approve-confirm grace
// window.
func (m *Manager) RecordSent(keys ...string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	for _, k := range keys {
		if k != "" {
			m.sent[k] = now
			// A new attempt: whatever the last one was answered with is
			// not what this one will be.
			delete(m.expected, k)
		}
	}
}

// ExpectKey records the key the asked device answered the handshake with,
// under the same keys RecordSent took.
func (m *Manager) ExpectKey(publicKey string, keys ...string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, k := range keys {
		if k != "" && publicKey != "" {
			m.expected[k] = publicKey
		}
	}
}

// KeyMismatch reports whether an approve-confirm's key differs from the one
// the device answered our handshake with. Call it before ValidateConfirm,
// which consumes the records.
func (m *Manager) KeyMismatch(peerID, clientIP string, port int, publicKey string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	for _, k := range candidateKeys(peerID, clientIP, port) {
		want, ok := m.expected[k]
		if !ok {
			continue
		}
		if t, sent := m.sent[k]; !sent || now.Sub(t) >= graceWindow {
			continue
		}
		if want != publicKey {
			return true
		}
	}
	return false
}

// ValidateConfirm decides whether an /approve-confirm may be accepted:
// we initiated a handshake to this peer/address within the grace window,
// OR they have an active incoming handshake here (simultaneous initiation),
// OR they're already paired (idempotent re-confirm). alreadyPaired is
// supplied by the caller from the store. Valid confirms consume the sent
// entries.
//
// The approve-confirm handlers pass alreadyPaired=false: being paired is
// not, on its own, grounds to apply a confirm (see
// reconfirmWithoutHandshake in package p2p).
func (m *Manager) ValidateConfirm(peerID, clientIP string, port int, alreadyPaired bool) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if alreadyPaired {
		return true
	}
	if _, ok := m.incoming[peerID]; ok {
		return true
	}

	now := m.now()
	fresh := func(key string) bool {
		t, ok := m.sent[key]
		return ok && now.Sub(t) < graceWindow
	}

	keys := candidateKeys(peerID, clientIP, port)
	for _, k := range keys {
		if fresh(k) {
			// Consume every candidate entry to prevent replay.
			for _, kk := range keys {
				delete(m.sent, kk)
				delete(m.expected, kk)
			}
			return true
		}
	}
	return false
}

// candidateKeys covers peer-id, plain-ip, ip:port, and the
// localhost/127.0.0.1 alias cross-checks from the JS implementation.
func candidateKeys(peerID, clientIP string, port int) []string {
	keys := []string{
		peerID,
		clientIP,
		hostPort(clientIP, port),
	}
	switch clientIP {
	case "localhost":
		keys = append(keys, "127.0.0.1", hostPort("127.0.0.1", port))
	case "127.0.0.1", "::1":
		keys = append(keys, "localhost", hostPort("localhost", port))
	}
	return keys
}

func hostPort(host string, port int) string {
	if port <= 0 {
		return host
	}
	return host + ":" + itoa(port)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
