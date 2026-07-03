// Package p2p wires peer discovery, pairing, the sync engine, and the
// /api/p2p/* peer protocol routes into one engine — the Go counterpart of
// src/daemon/p2p/index.js.
package p2p

import (
	"context"
	"fmt"
	"time"

	"github.com/opensave/opensave/internal/delta"
	"github.com/opensave/opensave/internal/p2p/discovery"
	"github.com/opensave/opensave/internal/p2p/pairing"
	"github.com/opensave/opensave/internal/p2p/syncengine"
	"github.com/opensave/opensave/internal/snapshot"
	"github.com/opensave/opensave/internal/store"
)

// GameState is the lightweight per-game summary exchanged in pings/hellos
// so peers can see what each other has without a full manifest fetch.
type GameState struct {
	LatestSnapshotID   string `json:"latestSnapshotId"`
	LatestSnapshotTime int64  `json:"latestSnapshotTime"`
	ActiveBranch       string `json:"activeBranch"`
	ManifestHash       string `json:"manifestHash"`
}

// Engine owns all P2P state for one daemon.
type Engine struct {
	Store     *store.Store
	Snapshots *snapshot.Manager
	Sync      *syncengine.Engine
	Pairing   *pairing.Manager
	Discovery *discovery.Manager
	Wan       *WanClient
	Log       func(level, msg string)

	// OnPeerUpdate fires whenever peer/pairing state changes (dashboard
	// broadcast hook). May be nil.
	OnPeerUpdate func()
}

// StartDiscovery begins UDP LAN presence broadcasting. Paired peers seen
// on the LAN flip online (triggering auto-sync when they were offline);
// unseen peers age out to offline.
func (e *Engine) StartDiscovery() error {
	identity := func() discovery.Ping {
		settings, err := e.Store.GetSettings()
		if err != nil {
			return discovery.Ping{}
		}
		return discovery.Ping{
			NodeID:     settings.NodeID,
			DeviceName: settings.DeviceName,
			DeviceType: settings.DeviceType,
			Port:       settings.Port,
		}
	}

	e.Discovery = discovery.New(identity, discovery.Callbacks{
		OnPeerSeen: func(d discovery.Discovered, isNew bool) {
			peer, err := e.Store.GetPeer(d.ID)
			if err == nil {
				wasOffline := peer.Status != "online"
				peer.Address = d.Address
				peer.Port = d.Port
				peer.DeviceType = d.DeviceType
				peer.Status = "online"
				peer.LastSeenMs = d.LastSeen
				_ = e.Store.UpsertPeer(peer)
				if wasOffline {
					e.Log("info", fmt.Sprintf("paired peer %q appeared on LAN; auto-syncing", peer.Name))
					go e.SyncAllGames(context.Background())
					e.notifyPeerUpdate()
				}
			} else if isNew {
				e.notifyPeerUpdate()
			}
		},
		OnExpired: func(d discovery.Discovered) {
			peer, err := e.Store.GetPeer(d.ID)
			if err == nil && peer.Status == "online" && peer.Address != "relay" {
				peer.Status = "offline"
				_ = e.Store.UpsertPeer(peer)
			}
			e.notifyPeerUpdate()
		},
	})
	return e.Discovery.Start()
}

// Stop shuts down discovery and the WAN client.
func (e *Engine) Stop() {
	if e.Discovery != nil {
		e.Discovery.Stop()
	}
	if e.Wan != nil {
		e.Wan.Disconnect()
	}
}

// New assembles a P2P engine with LAN + WAN transports routed per peer.
func New(s *store.Store, snaps *snapshot.Manager, logf func(level, msg string)) *Engine {
	e := &Engine{
		Store:     s,
		Snapshots: snaps,
		Pairing:   pairing.New(),
		Log:       logf,
	}
	e.Wan = newWanClient(e)
	e.Sync = syncengine.New(s, snaps, &routingTransport{
		lan: &lanTransport{},
		wan: &wanTransport{wan: e.Wan},
	})
	e.Sync.Log = logf
	return e
}

// LocalGamesState builds the per-game summary for ping/hello responses.
func (e *Engine) LocalGamesState() map[string]GameState {
	out := map[string]GameState{}
	games, err := e.Store.ListGames()
	if err != nil {
		return out
	}
	for _, g := range games {
		state := GameState{ActiveBranch: g.ActiveBranch}
		if latest, err := e.Snapshots.LatestSnapshot(g.ID, ""); err == nil {
			state.LatestSnapshotID = latest.ID
			if t, err := time.Parse("2006-01-02T15:04:05.000Z", latest.Timestamp); err == nil {
				state.LatestSnapshotTime = t.UnixMilli()
			}
		}
		if m, err := delta.BuildManifest(g.SavePath); err == nil {
			state.ManifestHash = m.ManifestHash()
		}
		out[g.ID] = state
	}
	return out
}

// OnlinePeers returns paired peers currently marked online, as sync-engine
// peer descriptors.
func (e *Engine) OnlinePeers() []syncengine.Peer {
	peers, err := e.Store.ListPeers()
	if err != nil {
		return nil
	}
	var online []syncengine.Peer
	for _, p := range peers {
		if p.Status == "online" {
			online = append(online, syncengine.Peer{
				ID: p.ID, Name: p.Name, Address: p.Address, Port: p.Port,
				IsWan: p.Address == "relay",
			})
		}
	}
	return online
}

// PingPairedPeers probes every paired LAN peer and updates their
// online/offline status.
func (e *Engine) PingPairedPeers(ctx context.Context) {
	peers, err := e.Store.ListPeers()
	if err != nil {
		return
	}
	settings, err := e.Store.GetSettings()
	if err != nil {
		return
	}

	changed := false
	for _, p := range peers {
		if p.Address == "relay" {
			continue // WAN presence is heartbeat-driven (Phase 3)
		}
		ok := pingPeer(ctx, p, settings.NodeID)
		newStatus := "offline"
		if ok {
			newStatus = "online"
		}
		if p.Status != newStatus {
			p.Status = newStatus
			p.LastSeenMs = time.Now().UnixMilli()
			_ = e.Store.UpsertPeer(p)
			changed = true
		}
	}
	if changed {
		e.notifyPeerUpdate()
	}
}

// SyncGame pings peers and then syncs one game with everyone online.
func (e *Engine) SyncGame(ctx context.Context, gameID string) (map[string]syncengine.Result, error) {
	e.PingPairedPeers(ctx)
	online := e.OnlinePeers()
	if len(online) == 0 {
		return nil, fmt.Errorf("no online peers available")
	}
	return e.Sync.SyncGame(ctx, gameID, online)
}

// SyncAllGames syncs every tracked game (used when a peer comes online).
func (e *Engine) SyncAllGames(ctx context.Context) {
	games, err := e.Store.ListGames()
	if err != nil {
		return
	}
	online := e.OnlinePeers()
	if len(online) == 0 {
		return
	}
	for _, g := range games {
		if !g.AutoSync {
			continue
		}
		if _, err := e.Sync.SyncGame(ctx, g.ID, online); err != nil {
			e.Log("warn", fmt.Sprintf("auto-sync %s: %v", g.ID, err))
		}
	}
}

// InitiatePair sends a handshake to a device at address:port and opens the
// approve-confirm grace window.
func (e *Engine) InitiatePair(ctx context.Context, address string, port int) error {
	settings, err := e.Store.GetSettings()
	if err != nil {
		return err
	}

	e.Pairing.RecordSent(address, fmt.Sprintf("%s:%d", address, port))

	err = postHandshake(ctx, address, port, map[string]any{
		"peerId":     settings.NodeID,
		"deviceName": settings.DeviceName,
		"deviceType": settings.DeviceType,
		"port":       settings.Port,
	})
	if err != nil {
		return fmt.Errorf("handshake to %s:%d: %w", address, port, err)
	}
	e.Log("info", fmt.Sprintf("pairing request sent to %s:%d — waiting for their approval", address, port))
	return nil
}

// InitiatePairWan sends a handshake to a room member through the relay.
func (e *Engine) InitiatePairWan(ctx context.Context, peerID string) error {
	settings, err := e.Store.GetSettings()
	if err != nil {
		return err
	}
	// "relay" is the JS grace-window key for WAN-initiated handshakes.
	e.Pairing.RecordSent(peerID, "relay")

	_, err = e.Wan.Request(ctx, peerID, "/handshake", "POST", map[string]any{
		"peerId":     settings.NodeID,
		"deviceName": settings.DeviceName,
		"deviceType": settings.DeviceType,
		"port":       settings.Port,
	})
	if err != nil {
		return fmt.Errorf("WAN handshake to %s: %w", peerID, err)
	}
	e.Log("info", fmt.Sprintf("WAN pairing request sent to %s — waiting for their approval", peerID))
	return nil
}

// ApprovePairing accepts a pending incoming handshake: persists the peer
// and sends approve-confirm back to them.
func (e *Engine) ApprovePairing(ctx context.Context, peerID string) error {
	req, ok := e.Pairing.TakeIncoming(peerID)
	if !ok {
		return fmt.Errorf("no pending pairing request from %q", peerID)
	}
	settings, err := e.Store.GetSettings()
	if err != nil {
		return err
	}

	if err := e.Store.UpsertPeer(store.Peer{
		ID: req.PeerID, Name: req.DeviceName, DeviceType: orDefault(req.DeviceType, "desktop"),
		Address: req.Address, Port: req.Port, Status: "online", LastSeenMs: time.Now().UnixMilli(),
	}); err != nil {
		return err
	}

	confirmBody := map[string]any{
		"peerId":     settings.NodeID,
		"deviceName": settings.DeviceName,
		"deviceType": settings.DeviceType,
		"port":       settings.Port,
	}
	if req.IsWan || req.Address == "relay" {
		if _, err := e.Wan.Request(ctx, req.PeerID, "/approve-confirm", "POST", confirmBody); err != nil {
			e.Log("warn", fmt.Sprintf("WAN approve-confirm to %s failed (peer saved anyway): %v", req.DeviceName, err))
		}
	} else if err := postApproveConfirm(ctx, req.Address, req.Port, confirmBody); err != nil {
		e.Log("warn", fmt.Sprintf("approve-confirm to %s failed (peer saved anyway): %v", req.DeviceName, err))
	}

	e.Log("success", fmt.Sprintf("paired with %q (%s:%d)", req.DeviceName, req.Address, req.Port))
	e.notifyPeerUpdate()
	return nil
}

// RejectPairing discards a pending incoming handshake.
func (e *Engine) RejectPairing(peerID string) {
	e.Pairing.TakeIncoming(peerID)
	e.notifyPeerUpdate()
}

// Unpair removes a paired peer.
func (e *Engine) Unpair(peerID string) error {
	if err := e.Store.UnpairPeer(peerID); err != nil {
		return err
	}
	e.notifyPeerUpdate()
	return nil
}

func (e *Engine) notifyPeerUpdate() {
	if e.OnPeerUpdate != nil {
		e.OnPeerUpdate()
	}
}

func orDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
