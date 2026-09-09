package relay

import (
	"strings"
	"testing"
)

// Reading the account from the unit, because the whole point is to hand the
// secrets file to whoever the service actually runs as — including an
// operator who changed it.
func TestServiceUserFrom(t *testing.T) {
	cases := []struct {
		name string
		unit string
		want string
	}{
		{"the installer's unit", "[Service]\nType=simple\nUser=opensave-relay\nExecStart=/usr/local/bin/opensave-relay\n", "opensave-relay"},
		{"an operator's own account", "[Service]\nUser=relaysvc\n", "relaysvc"},
		{"spaces around the value", "[Service]\n  User =  someone  \n", "someone"},
		{"no User line at all", "[Service]\nType=simple\nExecStart=/x\n", ""},
		{"commented out", "[Service]\n# User=opensave-relay\n; User=other\n", ""},
		{"empty unit", "", ""},
	}
	for _, c := range cases {
		if got := serviceUserFrom(strings.NewReader(c.unit)); got != c.want {
			t.Errorf("%s: serviceUserFrom = %q, want %q", c.name, got, c.want)
		}
	}
}

// A commented-out User= must not be read as the service account: handing the
// secrets file to an account the service does not use would lock the service
// out of it, which is the failure this code exists to prevent.
func TestServiceUserFrom_IgnoresComments(t *testing.T) {
	unit := "[Service]\n# User=wrong-account\nUser=right-account\n"
	if got := serviceUserFrom(strings.NewReader(unit)); got != "right-account" {
		t.Errorf("serviceUserFrom = %q, want right-account", got)
	}
}
