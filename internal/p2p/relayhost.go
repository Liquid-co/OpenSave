package p2p

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/opensave/opensave/internal/sysintegration/upnp"
	"github.com/opensave/opensave/relay"
)

// upnpTimeout bounds the router conversation. A gateway that does not answer
// promptly is one that is not going to, and hosting must not appear to hang
// on a setting the user just ticked.
const upnpTimeout = 12 * time.Second

// RelayHost runs an OpenSave relay server inside this process when the
// user enables "host a relay" — so friends can connect directly to this
// machine without a third-party relay. Safe for concurrent use.
type RelayHost struct {
	mu     sync.Mutex
	server *relay.Server
	port   int
	addr   string
	logf   func(level, msg string)

	// stopRenew ends the lease-renewal loop for the current mapping.
	stopRenew context.CancelFunc
	// mapped is the port currently forwarded on the router, 0 when none.
	// Tracked so the mapping is withdrawn on the way out rather than left
	// behind: a hole punched in someone's router by a checkbox they have
	// since unticked is the kind of thing they will never find again.
	mapped int
	// ExternalIP is the address the gateway reports, when it reported one.
	// Read by the API so the app can show an address that is actually
	// reachable instead of one the user has to work out.
	ExternalIP string
}

// NewRelayHost creates an idle relay host.
func NewRelayHost(logf func(level, msg string)) *RelayHost {
	return &RelayHost{logf: logf}
}

// Apply starts, stops, or restarts the hosted relay to match the desired
// state. Idempotent: calling it repeatedly with the same args is a no-op.
func (h *RelayHost) Apply(enabled bool, port int) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !enabled {
		h.stopLocked()
		return
	}
	if h.server != nil && h.port == port {
		return // already running on this port
	}
	h.stopLocked()

	srv := relay.New(relay.Config{Port: port})
	addr, err := srv.Start()
	if err != nil {
		h.logf("error", fmt.Sprintf("could not host relay on port %d: %v", port, err))
		return
	}
	h.server = srv
	h.port = port
	h.addr = addr
	h.logf("success", fmt.Sprintf("hosting WAN relay on port %d", port))

	// Ask the router to open the port.
	//
	// This is the step that decided whether self-hosting was practical: the
	// code to do it has existed in internal/sysintegration/upnp since it was
	// ported from the JS app, reachable only from a CLI verb, while the app
	// told people to configure their router by hand. Most never will.
	//
	// Best-effort on purpose. Plenty of routers have UPnP switched off, and a
	// refusal is not a failure to host — the relay is already running and is
	// reachable on the LAN either way. So it is reported, not raised, and the
	// wording says what to do next rather than only what went wrong.
	h.forwardLocked(port)
}

// forwardLocked asks the gateway to open the port, recording the mapping so
// it can be withdrawn later.
func (h *RelayHost) forwardLocked(port int) {
	ctx, cancel := context.WithTimeout(context.Background(), upnpTimeout)
	defer cancel()

	externalIP, err := upnp.Forward(ctx, port)
	if err != nil {
		h.logf("warn", fmt.Sprintf(
			"could not open port %d on your router automatically (%v) — friends on the "+
				"internet will not reach you until you forward it by hand, though this "+
				"relay already works for devices on your own network", port, err))
		return
	}
	h.mapped = port
	h.ExternalIP = externalIP
	h.startRenewLocked(port)
	if externalIP != "" {
		h.logf("success", fmt.Sprintf(
			"opened port %d on your router — friends can reach you at %s:%d", port, externalIP, port))
		return
	}
	h.logf("success", fmt.Sprintf("opened port %d on your router", port))
}

// Stop shuts the hosted relay down.
func (h *RelayHost) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.stopLocked()
}

func (h *RelayHost) stopLocked() {
	if h.stopRenew != nil {
		h.stopRenew()
		h.stopRenew = nil
	}
	// Withdraw the router mapping first, and regardless of whether a server
	// is still running — the two can get out of step if hosting failed to
	// start after a mapping was made, and the mapping is the part that
	// outlives this process.
	if h.mapped != 0 {
		ctx, cancel := context.WithTimeout(context.Background(), upnpTimeout)
		if err := upnp.Remove(ctx, h.mapped); err != nil {
			h.logf("warn", fmt.Sprintf(
				"could not close port %d on your router (%v) — remove the "+
					"\"OpenSave Relay\" forward by hand if you want it gone", h.mapped, err))
		} else {
			h.logf("info", fmt.Sprintf("closed port %d on your router", h.mapped))
		}
		cancel()
		h.mapped = 0
		h.ExternalIP = ""
	}
	if h.server != nil {
		h.server.Stop()
		h.logf("info", "stopped hosted WAN relay")
		h.server = nil
		h.addr = ""
		h.port = 0
	}
}

// Running reports whether a relay is currently hosted and on what port.
func (h *RelayHost) Running() (bool, int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.server != nil, h.port
}

// startRenewLocked keeps the router mapping alive while hosting continues.
//
// The mapping is leased rather than permanent, so it now has to be renewed or
// it lapses mid-session and friends quietly stop being able to reach this
// machine. Renewed at half the lease, which means one missed renewal is
// survivable rather than an outage.
func (h *RelayHost) startRenewLocked(port int) {
	ctx, cancel := context.WithCancel(context.Background())
	h.stopRenew = cancel
	every := (upnp.LeaseSeconds / 2) * int(time.Second)
	go func() {
		ticker := time.NewTicker(time.Duration(every))
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				renewCtx, done := context.WithTimeout(ctx, upnpTimeout)
				if _, err := upnp.Forward(renewCtx, port); err != nil {
					h.logf("warn", fmt.Sprintf(
						"could not renew the router mapping for port %d (%v) — friends may "+
							"stop reaching you when it lapses", port, err))
				}
				done()
			}
		}
	}()
}
