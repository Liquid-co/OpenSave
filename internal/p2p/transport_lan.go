package p2p

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/opensave/opensave/internal/e2ee"
	"github.com/opensave/opensave/internal/p2p/syncengine"
	"github.com/opensave/opensave/internal/store"
)

// lanTransport speaks the /api/p2p/* HTTP protocol directly to a peer.
// WAN peers get their own relay-tunnel transport in Phase 3.
//
// engine is held so requests can be authenticated with the key pinned when
// the two devices paired. Without it a peer was identified by its source
// address alone, which a device on the same network can take — by ARP
// spoofing, or simply by being handed that address after the real peer's DHCP
// lease expired. The relay path was given proof-of-key; this is the same
// treatment for the path most syncs actually use.
type lanTransport struct{ engine *Engine }

var lanClient = &http.Client{Timeout: 30 * time.Second}

func peerURL(peer syncengine.Peer, route string) string {
	return fmt.Sprintf("http://%s:%d/api/p2p%s", peer.Address, peer.Port, route)
}

func (t *lanTransport) FetchManifest(ctx context.Context, peer syncengine.Peer, gameID string, q syncengine.ManifestQuery) (syncengine.ManifestResponse, error) {
	params := url.Values{}
	if q.Name != "" {
		params.Set("name", q.Name)
	}
	if q.SavePath != "" {
		params.Set("savePath", q.SavePath)
	}
	params.Set("isFile", fmt.Sprintf("%t", q.IsFile))
	if q.AppID != "" {
		params.Set("appId", q.AppID)
	}
	if q.CoverURL != "" {
		params.Set("coverUrl", q.CoverURL)
	}

	var resp syncengine.ManifestResponse
	err := t.getJSON(ctx, peer, peerURL(peer, "/manifest/"+gameID)+"?"+params.Encode(), &resp)
	return resp, err
}

func (t *lanTransport) FetchBlocks(ctx context.Context, peer syncengine.Peer, ref syncengine.FileRef, blockIndices []int, blockSize int) ([]syncengine.BlockData, error) {
	var resp struct {
		Blocks []syncengine.BlockData `json:"blocks"`
	}
	// No encodings advertised: on a LAN the wire is typically faster than the
	// compressor, so the bytes saved cost more than they're worth. Responses
	// are still decoded, so a peer that compresses anyway is handled.
	err := t.postJSON(ctx, peer, peerURL(peer, "/blocks/"+ref.GameID), map[string]any{
		"relPath": ref.RelPath, "root": ref.Root, "blockIndices": blockIndices, "blockSize": blockSize,
	}, &resp)
	if err != nil {
		return nil, err
	}
	return decodeBlocks(resp.Blocks)
}

func (t *lanTransport) DeleteRemote(ctx context.Context, peer syncengine.Peer, ref syncengine.FileRef) error {
	return t.postJSON(ctx, peer, peerURL(peer, "/delete-file/"+ref.GameID), map[string]any{"relPath": ref.RelPath, "root": ref.Root}, nil)
}

// TriggerPeerPull tells a peer that this device holds newer content.
//
// Signed like every other request, which it was not: being fire-and-forget it
// built its own http.Request instead of going through getJSON, and so skipped
// t.sign. The receiving side refuses an unsigned request from a peer that has
// authenticated before, so this was dropped on arrival and the peer only
// noticed on its next periodic reconcile — up to a minute later, with nothing
// reported anywhere. The identical mistake existed on the WAN side; see
// wanTransport.TriggerPeerPull.
func (t *lanTransport) TriggerPeerPull(peer syncengine.Peer, gameID string) {
	// Fire-and-forget, 5s cap, same as the JS fetch().catch(() => {}).
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		url := fmt.Sprintf("http://%s:%d/api/sync/trigger/%s", peer.Address, peer.Port, gameID)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return
		}
		t.sign(req, peer, nil)
		resp, err := lanClient.Do(req)
		if err == nil {
			resp.Body.Close()
		}
	}()
}

func (t *lanTransport) ReportSyncEvent(peer syncengine.Peer, gameID, eventType string, data map[string]any) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = t.postJSON(ctx, peer, peerURL(peer, "/sync-event/"+gameID), map[string]any{
			"eventType": eventType, "data": data,
		}, nil)
	}()
}

func (t *lanTransport) getJSON(ctx context.Context, peer syncengine.Peer, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	t.sign(req, peer, nil)
	return doJSON(req, out)
}

func (t *lanTransport) postJSON(ctx context.Context, peer syncengine.Peer, url string, body any, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	t.sign(req, peer, raw)
	return doJSON(req, out)
}

// sign attaches proof that this request came from the device it claims to.
//
// The body is covered, and it is cheap to cover: LAN request bodies are small
// — a list of block indices, a path — because the bulk of a sync travels in
// the RESPONSES. Buffering a request body here costs nothing worth measuring,
// and leaving it uncovered would let a captured request be replayed with its
// contents swapped.
//
// Silent when the pair has no key, which is a pairing made before key
// exchange existed. The receiving side knows that and does not demand proof
// that could never have been sent.
func (t *lanTransport) sign(req *http.Request, peer syncengine.Peer, body []byte) {
	if t.engine == nil {
		return
	}
	key, err := t.engine.requestAuthKey(peer.ID)
	if err != nil {
		return
	}
	nonce, err := e2ee.NewNonce()
	if err != nil {
		return
	}
	from := t.engine.localNodeID()
	at := time.Now().UnixMilli()
	route := req.URL.RequestURI()
	req.Header.Set(lanAuthPeerHeader, from)
	req.Header.Set(lanAuthNonceHeader, nonce)
	req.Header.Set(lanAuthTimeHeader, strconv.FormatInt(at, 10))
	req.Header.Set(lanAuthHeader, e2ee.RequestMAC(key, from, peer.ID, route, req.Method, body, nonce, at))
}

func doJSON(req *http.Request, out any) error {
	resp, err := lanClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("peer returned %d: %s", resp.StatusCode, string(raw))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// pingPeer probes a paired peer's /api/p2p/ping. Reports reachability plus
// the peer's app build info (for the peer-update flow) when present.
func pingPeer(ctx context.Context, p store.Peer, fromNodeID string) (pingInfo, bool) {
	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	url := fmt.Sprintf("http://%s:%d/api/p2p/ping?from=%s", p.Address, p.Port, url.QueryEscape(fromNodeID))
	req, err := http.NewRequestWithContext(pingCtx, http.MethodGet, url, nil)
	if err != nil {
		return pingInfo{}, false
	}
	resp, err := lanClient.Do(req)
	if err != nil {
		return pingInfo{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return pingInfo{}, false
	}
	var info pingInfo
	_ = json.NewDecoder(resp.Body).Decode(&info)
	return info, true
}

// pingInfo is the subset of the ping response the caller records.
type pingInfo struct {
	AppVersion  string `json:"appVersion"`
	BuildTimeMs int64  `json:"buildTimeMs"`
}

func postHandshake(ctx context.Context, address string, port int, body map[string]any) error {
	return postPeerJSON(ctx, address, port, "/handshake", body)
}

func postApproveConfirm(ctx context.Context, address string, port int, body map[string]any) error {
	return postPeerJSON(ctx, address, port, "/approve-confirm", body)
}

func postPeerJSON(ctx context.Context, address string, port int, route string, body map[string]any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	url := fmt.Sprintf("http://%s:%d/api/p2p%s", address, port, route)
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return doJSON(req, nil)
}
