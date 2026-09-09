package relay

import (
	"bufio"
	"io"
	"os"
	"strings"
)

// Who the installed relay service runs as, and making the secrets file
// readable by them.
//
// The problem this exists for: install-relay.sh creates a dedicated
// "opensave-relay" account and runs the service as it, which is the right way
// round for isolation. But `opensave-relay setup` is run with sudo, and writes
// the secrets file 0600 owned by root — so the service cannot read it. The key
// is stored correctly, reported as configured, and never reaches the relay.
//
// That failure is silent in both directions: the relay answers "no key
// configured" and `opensave-relay config`, run as root, reads the file
// perfectly well and says the key is set. Somebody self-hosting has no
// thread to pull.
//
// The environment-variable route does not have this problem, because systemd
// reads EnvironmentFile= as root before dropping privileges. Only the file
// written by setup is affected.

// unitPath is where install-relay.sh puts the service definition.
const unitPath = "/etc/systemd/system/opensave-relay.service"

// ServiceUser reports the account the installed relay service runs as, or ""
// when there is no installed service to read.
//
// Parsed from the unit rather than assumed, because an operator who edited
// User= is exactly the person whose setup would otherwise break in a way
// nothing explains.
func ServiceUser() string {
	f, err := os.Open(unitPath)
	if err != nil {
		return ""
	}
	defer f.Close()
	return serviceUserFrom(f)
}

// serviceUserFrom is the parsing, split out so it can be tested without a
// systemd unit on disk.
func serviceUserFrom(r io.Reader) string {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		name, value, found := strings.Cut(line, "=")
		if !found || !strings.EqualFold(strings.TrimSpace(name), "User") {
			continue
		}
		return strings.TrimSpace(value)
	}
	return ""
}

// HandOverToServiceUser makes path readable by the account the service runs
// as, and reports which account that was.
//
// Returns "" with no error when there is nothing to do: no installed service,
// or the file already belongs to the right account. An error means the file
// was written but the service still cannot read it — which the caller must
// say out loud rather than reporting success.
func HandOverToServiceUser(path string) (string, error) {
	user := ServiceUser()
	if user == "" {
		return "", nil
	}
	changed, err := chownTo(path, user)
	if err != nil {
		return user, err
	}
	if !changed {
		return "", nil
	}
	return user, nil
}
