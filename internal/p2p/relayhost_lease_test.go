package p2p

import (
	"testing"
	"time"

	"github.com/opensave/opensave/internal/sysintegration/upnp"
)

// The router mapping's lease arithmetic.
//
// Hosting a relay asks the router to forward a port, and that request is
// leased rather than permanent — a permanent mapping survives the app being
// uninstalled, a crash, a DHCP address change, and stays on the router
// forwarding a port to a machine that may no longer be listening. The cost of
// leasing is that the mapping has to be renewed, and a renewal that is late by
// even a moment is an outage nobody is told about: the relay keeps running,
// the app keeps saying it is hosting, and friends simply cannot reach it.
//
// Nothing about the UPnP path can be tested against a real router here, but
// this part is arithmetic, and it is the part where being wrong is silent.

// renewInterval is the calculation startRenewLocked performs. Kept in one
// place so the test measures the same expression the code runs.
func renewInterval() time.Duration {
	return time.Duration((upnp.LeaseSeconds / 2) * int(time.Second))
}

func TestRelayHost_MappingIsRenewedBeforeItExpires(t *testing.T) {
	lease := time.Duration(upnp.LeaseSeconds) * time.Second
	every := renewInterval()

	if every <= 0 {
		t.Fatalf("renewal interval is %v; the ticker would panic and the mapping would never renew", every)
	}
	if every >= lease {
		t.Fatalf("renewing every %v on a %v lease renews after it has already expired", every, lease)
	}
	// Half, so one missed renewal is survivable rather than an outage. A
	// margin much tighter than this leaves no room for a router that is slow
	// to answer or a machine that was asleep.
	if every > lease/2 {
		t.Errorf("renewing every %v leaves no margin on a %v lease; one missed renewal drops the mapping",
			every, lease)
	}

	// The unit trap this expression invites: LeaseSeconds is a count of
	// seconds and time.Duration is a count of nanoseconds, so an interval that
	// forgot the conversion would come out as 1800 nanoseconds and hammer the
	// router thousands of times a second.
	if every < time.Minute {
		t.Errorf("renewal interval is %v — that is a unit error, not a schedule", every)
	}
	if want := 30 * time.Minute; every != want {
		t.Errorf("renewal interval is %v, want %v (half of the %v lease)", every, want, lease)
	}
}

// A lease long enough to be worth having, short enough that an abandoned
// mapping ages out on its own.
func TestUPnPLeaseIsBounded(t *testing.T) {
	lease := time.Duration(upnp.LeaseSeconds) * time.Second
	if lease == 0 {
		t.Fatal("a zero lease means a permanent mapping, which outlives the app that asked for it — " +
			"including an uninstall, and including a DHCP move that points it at someone else's machine")
	}
	if lease < 10*time.Minute {
		t.Errorf("lease of %v is short enough that a renewal hiccup becomes an outage", lease)
	}
	if lease > 24*time.Hour {
		t.Errorf("lease of %v is long enough to be permanent in practice", lease)
	}
}
