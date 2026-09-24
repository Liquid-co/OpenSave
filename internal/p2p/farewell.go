package p2p

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/opensave/opensave/internal/store"
)

// Goodbyes to devices this one has unpaired.
//
// Unpairing deletes the peer's record and tells the other device, with a
// goodbye signed by the key the two share — derived from the public key in
// the record being deleted. The goodbye used to be sent once and forgotten.
// When it was lost — the other device asleep, the relay socket reconnecting —
// nothing ever sent it again: the fallback, an unsigned notice sent when the
// other device next said hello, is refused by any device that has seen this
// one authenticate, which is every pairing made by a current build. So the
// other device kept this one as paired for good, trying to sync with it and
// being turned away every time.
//
// Now the public key is kept after the record goes (store.UnpairedPeer), the
// goodbye waits for an answer, and until one comes it is signed and sent
// again whenever the other device turns up: any frame from it in the relay
// room, or on a LAN a ping or a request, which it only sends to devices it
// believes it is paired with. Only that device can act on the goodbye — it is
// signed with the key the two of them share — so a third device in the relay
// room still cannot unpair anything, and a copy captured and sent later fails
// the timestamp and nonce checks every request goes through.

const (
	// A retry at most this often per device: the signs that trigger one come
	// every few seconds while the other device is trying to sync. Short
	// enough that a device back from being offline — the usual reason the
	// first goodbye failed moments before — is told almost at once.
	farewellRetryEvery = 10 * time.Second
	// How long to wait for the other device to answer one goodbye.
	farewellTimeout = 20 * time.Second
	// A device not seen for this long is not coming back to be told.
	farewellKeepFor = 30 * 24 * time.Hour
)

type farewell struct {
	peer     store.UnpairedPeer
	lastTry  time.Time
	inFlight bool
}

// loadFarewellsLocked fills the in-memory list from the store the first time
// it is needed. The list is consulted on every frame in the relay room, so it
// lives in memory rather than being read each time.
func (e *Engine) loadFarewellsLocked() {
	if e.farewells != nil {
		return
	}
	e.farewells = map[string]*farewell{}
	cutoff := time.Now().Add(-farewellKeepFor).UnixMilli()
	if n, err := e.Store.ForgetUnpairedBefore(cutoff); err == nil && n > 0 {
		e.Log("info", fmt.Sprintf("gave up telling %d device(s) unpaired over a month ago", n))
	}
	owed, err := e.Store.UnpairedPeers()
	if err != nil {
		e.Log("warn", "could not read which unpaired devices are still to be told: "+err.Error())
		return
	}
	for _, p := range owed {
		e.farewells[p.ID] = &farewell{peer: p}
	}
}

// oweGoodbye records that a device has been unpaired and not yet told.
func (e *Engine) oweGoodbye(p store.UnpairedPeer) {
	e.farewellMu.Lock()
	defer e.farewellMu.Unlock()
	e.loadFarewellsLocked()
	e.farewells[p.ID] = &farewell{peer: p}
}

func (e *Engine) settleGoodbye(peerID string) {
	e.farewellMu.Lock()
	if e.farewells != nil {
		delete(e.farewells, peerID)
	}
	e.farewellMu.Unlock()
	if err := e.Store.ForgetUnpaired(peerID); err != nil {
		e.Log("warn", err.Error())
	}
}

// remindUnpaired sends the goodbye again if peerID is a device this one
// unpaired and has not yet heard back from. Called wherever such a device
// shows it still thinks the two are paired; observedIP is where it spoke
// from, when that is known, and is tried rather than the address it had when
// it was unpaired — a device that has moved is the one most likely to have
// missed the first goodbye.
func (e *Engine) remindUnpaired(peerID, observedIP string) {
	if peerID == "" {
		return
	}
	e.farewellMu.Lock()
	e.loadFarewellsLocked()
	f, owed := e.farewells[peerID]
	if !owed || f.inFlight || time.Since(f.lastTry) < farewellRetryEvery {
		e.farewellMu.Unlock()
		return
	}
	f.inFlight = true
	f.lastTry = time.Now()
	target := f.peer
	e.farewellMu.Unlock()

	// Still paired: either this is the moment inside Unpair between writing
	// the record and deleting the peer — a frame from the device arriving
	// then must not be taken for a pairing that has moved on — or the two
	// were paired again, which drops the stored record. Nothing to send
	// either way, and nothing to settle.
	if _, err := e.Store.GetPeer(peerID); err == nil {
		e.farewellMu.Lock()
		f.inFlight = false
		e.farewellMu.Unlock()
		return
	}
	if observedIP != "" && target.Address != "relay" {
		target.Address = observedIP
	}
	e.GoSync(func(ctx context.Context) {
		ctx, cancel := context.WithTimeout(ctx, farewellTimeout)
		defer cancel()
		e.sayGoodbye(ctx, target)
	})
}

// sayGoodbye sends one signed goodbye and settles the debt if the other
// device answered. Any answer settles it: success; a refusal, which means it
// has already let go of this device or will never accept this key; or
// anything else, such as a build too old to know the route — which is told by
// the unsigned notice it still accepts. Sending again could not change any of
// them, and a debt that never settled would be retried for a month. No answer
// at all leaves it owed.
func (e *Engine) sayGoodbye(ctx context.Context, target store.UnpairedPeer) {
	defer func() {
		e.farewellMu.Lock()
		if f := e.farewells[target.ID]; f != nil {
			f.inFlight = false
		}
		e.farewellMu.Unlock()
	}()

	key, err := e.requestAuthKeyFor(store.Peer{ID: target.ID, PublicKey: target.PublicKey})
	if err != nil {
		e.Log("warn", fmt.Sprintf("cannot sign a goodbye to %q: %v", target.Name, err))
		e.settleGoodbye(target.ID)
		return
	}
	payload := map[string]string{"peerId": e.localNodeID()}

	var status int
	if target.Address == "relay" {
		if e.Wan == nil {
			return
		}
		status, _, err = e.Wan.exchange(ctx, target.ID, "/unpair", http.MethodPost, payload, key)
	} else {
		status, err = e.postGoodbyeLAN(ctx, target, payload, key)
	}
	if err != nil {
		e.Log("info", fmt.Sprintf(
			"could not reach %q to say it is unpaired (%v); it will be told when it is next seen", target.Name, err))
		return
	}
	switch {
	case status >= 200 && status < 300:
		e.Log("info", fmt.Sprintf("%q knows it is no longer paired with this device", target.Name))
	case status == http.StatusUnauthorized:
		e.Log("info", fmt.Sprintf("%q had already let go of this device", target.Name))
	default:
		e.Log("warn", fmt.Sprintf(
			"%q answered the goodbye with HTTP %d; if it still lists this device, unpair it there too",
			target.Name, status))
	}
	e.settleGoodbye(target.ID)
}

// postGoodbyeLAN sends the signed goodbye straight to the device. The status
// is the device's answer; err means there was none.
func (e *Engine) postGoodbyeLAN(ctx context.Context, target store.UnpairedPeer, payload any, key []byte) (int, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}
	url := fmt.Sprintf("http://%s:%d/api/p2p/unpair", target.Address, target.Port)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	e.signLANRequestWith(req, target.ID, body, key)
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return 0, err
	}
	resp.Body.Close()
	return resp.StatusCode, nil
}

// unpairedRecord is what is kept of a peer to sign goodbyes to it later.
func unpairedRecord(p store.Peer, nowMs int64) store.UnpairedPeer {
	return store.UnpairedPeer{
		ID: p.ID, Name: p.Name, Address: p.Address, Port: p.Port,
		PublicKey: strings.TrimSpace(p.PublicKey), UnpairedMs: nowMs,
	}
}
