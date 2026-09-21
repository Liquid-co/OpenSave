package e2e

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/opensave/opensave/testutil"
)

// The terminal shows the same protection state the app does, from a real
// daemon, through the real binary.
//
// The unit tests pin the rule; this pins the wiring. A column that is
// computed correctly and never reaches the table is the failure the app's
// badge already had once, and a terminal user on a Steam Deck in Game Mode
// may have no other way to see it.
func TestCLIPeers_ShowsProtectionPerDevice(t *testing.T) {
	relayURL := startRelay(t)
	a := testutil.NewTestDaemon(t, "CLIProt-A")
	b := testutil.NewTestDaemon(t, "CLIProt-B")
	pairOverRelay(t, a, b, relayURL, "cli-prot-room")
	c := testutil.NewTestDaemon(t, "CLIProt-C")
	a.PairWith(c) // one LAN pairing beside the relay one

	// A sync, so the relay peer proves it holds the key and the badge can
	// honestly say "encrypted" rather than "encrypting shortly".
	a.WriteSave("s.sav", "x")
	gameID := a.TrackGame("CLI Prot Game")
	b.API(http.MethodPost, "/api/games", map[string]string{"name": "CLI Prot Game", "savePath": b.SaveDir}, nil)
	syncTo(a, gameID, b.NodeID())
	if !testutil.WaitFor(30*time.Second, func() bool {
		p, err := a.Daemon.Store.GetPeer(b.NodeID())
		return err == nil && p.AuthVerifiedMs > 0
	}) {
		t.Fatal("setup: the relay peer never authenticated")
	}

	out := runCLIAgainst(t, a, "peers")

	// One row per device, and each names its own state.
	for _, want := range []string{
		"CLIProt-B", "internet relay", "encrypted",
		"CLIProt-C", "127.0.0.1", "direct",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("`opensave peers` output lacks %q:\n%s", want, out)
		}
	}
	// A relay member is not "on this network", and the relay's presence port
	// is not an address anyone can use.
	if strings.Contains(out, "relay:") {
		t.Errorf("a relay peer is printed with a port, which means nothing to a person:\n%s", out)
	}
	// Nothing needs re-pairing here, so no one must be told to.
	if strings.Contains(out, "Unpair and pair again") {
		t.Errorf("an encrypted pairing was told to re-pair:\n%s", out)
	}
	// And the JSON form carries the raw fields for scripts — parsed, not
	// string-matched, since the CLI pretty-prints it.
	var js struct {
		Peers map[string]struct {
			Encrypted   bool   `json:"encrypted"`
			OverRelay   bool   `json:"overRelay"`
			Fingerprint string `json:"fingerprint"`
		} `json:"peers"`
	}
	if err := json.Unmarshal([]byte(runCLIAgainst(t, a, "peers", "--json")), &js); err != nil {
		t.Fatalf("`opensave peers --json` is not JSON: %v", err)
	}
	relayPeer := js.Peers[b.NodeID()]
	if !relayPeer.Encrypted || !relayPeer.OverRelay || relayPeer.Fingerprint == "" {
		t.Errorf("`opensave peers --json` for the relay peer: %+v; want encrypted, overRelay and a fingerprint", relayPeer)
	}
	if js.Peers[c.NodeID()].OverRelay {
		t.Error("`opensave peers --json` reports the LAN peer as reached over a relay")
	}
}

// runCLIAgainst runs the built CLI with its home pointed at a test daemon.
//
// The CLI finds its daemon through $HOME/.opensave/daemon.addr, and a test
// daemon's home IS the .opensave directory, so the CLI gets a HOME whose
// .opensave links to it.
func runCLIAgainst(t *testing.T, d *testutil.TestDaemon, args ...string) string {
	t.Helper()
	fakeHome := filepath.Join(filepath.Dir(d.Daemon.Paths.HomeDir), "cli-home")
	if err := os.MkdirAll(fakeHome, 0o777); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(fakeHome, ".opensave")
	if _, err := os.Lstat(link); err != nil {
		if err := os.Symlink(d.Daemon.Paths.HomeDir, link); err != nil {
			t.Skipf("symlinks unavailable here (%v); this needs Developer Mode on Windows, or Linux", err)
		}
	}
	cmd := exec.Command(cliBin, args...)
	cmd.Env = append(os.Environ(), "HOME="+fakeHome, "USERPROFILE="+fakeHome, "NO_COLOR=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("opensave %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// A relay pairing with no key is the one state a person can act on, and the
// terminal has to say so and say how — it is the state every internet
// pairing made before 2.4.0 is in.
func TestCLIPeers_TellsYouToRepairAnUnkeyedRelayPairing(t *testing.T) {
	relayURL := startRelay(t)
	a := testutil.NewTestDaemon(t, "CLIRepair-A")
	b := testutil.NewTestDaemon(t, "CLIRepair-B")
	pairOverRelay(t, a, b, relayURL, "cli-repair-room")

	// Put the pairing in the state an older version left it: no key. The
	// store is the thing pairing would have written, so writing it back is
	// an exact model.
	// Neither SetPeerPublicKey nor UpsertPeer will drop a pinned key — by
	// design, a key survives everything but an unpair — so the test hook
	// writes the row the way an older version left it.
	if err := a.Daemon.Store.ForgetPeerKeyForTest(b.NodeID()); err != nil {
		t.Fatal(err)
	}
	if got, _ := a.Daemon.Store.GetPeer(b.NodeID()); got.PublicKey != "" {
		t.Fatal("setup: the key was not cleared")
	}

	out := runCLIAgainst(t, a, "peers")
	t.Logf("what a person sees:\n%s", out)
	if !strings.Contains(out, "not encrypted") {
		t.Errorf("an unkeyed relay pairing was not reported as unencrypted:\n%s", out)
	}
	if !strings.Contains(out, "Unpair and pair again") || !strings.Contains(out, "CLIRepair-B") {
		t.Errorf("no re-pair instruction naming the device:\n%s", out)
	}
}
