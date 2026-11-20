package p2p

// Play sessions shared between paired devices.
//
// Each device records only its own sessions (daemon/sessions.go): a game's
// process seen and then gone. So the Activity page could say how long a game
// was played here, and of another device only that a newer save arrived from
// it. Here each device asks every paired device online for the sessions it
// has ended since the last one kept from it, and keeps them
// (store.AddPeerPlaySessions), so time played can be shown per device.
//
// Asking, not telling: a device answers only for its own sessions, only to a
// paired device that has proved who it is, over the same signed request path
// as everything else (requirePairedPeer on the LAN, routeRequest over the
// relay). Nothing is passed on to a third device. What travels is the game's
// id and two times per session: no names, no paths, nothing of a save.
//
// A device on a build from before this answers 404; it is asked again an
// hour later, not every few minutes, and its sessions read "Not recorded".

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/opensave/opensave/internal/store"
)

const (
	playSessionsRoute = "/play-sessions"
	// playSessionsPage is the most sessions one answer carries.
	playSessionsPage = 500
	// playSessionsMaxPages stops one pull at 10,000 sessions; the next pull
	// carries on from there.
	playSessionsMaxPages = 20
	// playSessionsRetry is how long a device that does not know the route
	// is left before it is asked again.
	playSessionsRetry = time.Hour
)

type playSessionsRequest struct {
	// Since is the end of the newest session kept from the answering device,
	// on its own clock. Sessions ending at or after it are sent; the one at
	// the mark comes again and is kept once.
	Since int64 `json:"since"`
}

type playSessionsReply struct {
	Sessions []store.PlaySession `json:"sessions"`
	// More is set when the page is full and there may be more after it.
	More bool `json:"more"`
}

// playSessionsSince is this device's answer: its own sessions from the mark.
func (e *Engine) playSessionsSince(since int64) (playSessionsReply, error) {
	list, err := e.Store.PlaySessionsEndedSince(since, playSessionsPage)
	if err != nil {
		return playSessionsReply{}, err
	}
	return playSessionsReply{Sessions: list, More: len(list) == playSessionsPage}, nil
}

// handlePlaySessions answers a paired device on the LAN (requirePairedPeer).
func (e *Engine) handlePlaySessions(w http.ResponseWriter, r *http.Request) {
	var body playSessionsRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body); err != nil {
		jsonError(w, http.StatusBadRequest, "since is required")
		return
	}
	reply, err := e.playSessionsSince(body.Since)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "play sessions could not be read")
		return
	}
	jsonOK(w, reply)
}

// errPlaySessionsUnsupported is a device on a build that does not answer.
var errPlaySessionsUnsupported = errors.New("this device's version does not share play sessions")

// fetchPlaySessions asks one paired device for its sessions from the mark,
// over whichever way it is reached: signed on the LAN, sealed and signed
// through the relay (WanClient.Request).
func (e *Engine) fetchPlaySessions(ctx context.Context, peer store.Peer, since int64) (playSessionsReply, error) {
	body := playSessionsRequest{Since: since}
	var out playSessionsReply
	if peer.Address == "relay" {
		if e.Wan == nil {
			return out, errors.New("no relay connection")
		}
		raw, err := e.Wan.Request(ctx, peer.ID, playSessionsRoute, http.MethodPost, body)
		if err != nil {
			if strings.Contains(err.Error(), "not supported over WAN") {
				return out, errPlaySessionsUnsupported
			}
			return out, err
		}
		if err := json.Unmarshal(raw, &out); err != nil {
			return out, fmt.Errorf("unreadable play sessions: %w", err)
		}
		return out, nil
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return out, err
	}
	reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	url := fmt.Sprintf("http://%s:%d/api/p2p%s", peer.Address, peer.Port, playSessionsRoute)
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return out, err
	}
	req.Header.Set("Content-Type", "application/json")
	// Signed like every other request to a peer: an unsigned one is refused
	// by a device that has authenticated before (see TriggerPeerPull).
	e.signLANRequest(req, peer.ID, raw)
	if err := doJSON(req, &out); err != nil {
		if strings.Contains(err.Error(), "returned 404") || strings.Contains(err.Error(), "returned 405") {
			return out, errPlaySessionsUnsupported
		}
		return out, err
	}
	return out, nil
}

// playUnsupported remembers, per device, until when not to ask again.
type playUnsupported struct {
	mu    sync.Mutex
	until map[string]time.Time
}

var playSkips = &playUnsupported{until: map[string]time.Time{}}

func (p *playUnsupported) skip(peerID string, now time.Time) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return now.Before(p.until[peerID])
}

func (p *playUnsupported) mark(peerID string, now time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.until[peerID] = now.Add(playSessionsRetry)
}

// PullPlaySessions asks every paired device that is online for the sessions
// it has ended since the last one kept from it. Run every few minutes by the
// daemon; a device that cannot be reached is simply asked next time.
func (e *Engine) PullPlaySessions(ctx context.Context) int {
	peers, err := e.Store.ListPeers()
	if err != nil {
		return 0
	}
	total := 0
	for _, p := range peers {
		if ctx.Err() != nil {
			break
		}
		if p.Status != "online" || playSkips.skip(p.ID, time.Now()) {
			continue
		}
		total += e.pullPlaySessionsFrom(ctx, p)
	}
	return total
}

func (e *Engine) pullPlaySessionsFrom(ctx context.Context, p store.Peer) int {
	since, err := e.Store.LatestPeerSessionEnd(p.ID)
	if err != nil {
		return 0
	}
	total := 0
	for page := 0; page < playSessionsMaxPages; page++ {
		reply, err := e.fetchPlaySessions(ctx, p, since)
		if errors.Is(err, errPlaySessionsUnsupported) {
			playSkips.mark(p.ID, time.Now())
			break
		}
		if err != nil {
			break
		}
		added, err := e.Store.AddPeerPlaySessions(p.ID, reply.Sessions, time.Now())
		if err != nil {
			e.Log("warn", fmt.Sprintf("could not keep play sessions from %s: %v", p.Name, err))
			break
		}
		total += added
		if !reply.More || len(reply.Sessions) == 0 {
			break
		}
		next := reply.Sessions[len(reply.Sessions)-1].EndedMs
		if next <= since {
			break // a full page all ending at the mark: nothing further to ask for
		}
		since = next
	}
	if total > 0 {
		e.Log("info", fmt.Sprintf("%s shared %d play %s", p.Name, total, plural(total, "session", "sessions")))
	}
	return total
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
