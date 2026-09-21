package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/opensave/opensave/testutil"
)

// Prints what a third device in your relay room actually receives during a
// sync — the demonstration behind the "encrypted" badge, for anyone who
// wants to see it rather than take a test's word for it.
//
// Not run by default: it exists to be watched, not to pass. Run it with
//
//	OPENSAVE_WIRE_DEMO=1 go test ./e2e/ -run TestWireDemo -v
//
// It joins a relay room as an eavesdropper, pairs two devices in that room,
// syncs a save whose contents include a marker string, and prints every
// frame the eavesdropper was handed: which ones are sealed, which are plain,
// and whether the marker appeared anywhere. The eavesdropper test asserts the
// same things; this shows them.
func TestWireDemo(t *testing.T) {
	if os.Getenv("OPENSAVE_WIRE_DEMO") == "" {
		t.Skip("set OPENSAVE_WIRE_DEMO=1 to run this demonstration")
	}
	relayURL := startRelay(t)
	const room = "demo-room"
	const secret = "THE_PLAYER_NAME_IS_SIVA"

	spy := joinAsEavesdropper(t, relayURL, room)

	a := testutil.NewTestDaemon(t, "Desktop")
	b := testutil.NewTestDaemon(t, "Steam Deck")
	pairOverRelay(t, a, b, relayURL, room)

	a.WriteSave("player.sav", "save data containing "+secret)
	gameID := a.TrackGame("Demo Game")
	b.API(http.MethodPost, "/api/games", map[string]string{"name": "Demo Game", "savePath": b.SaveDir}, nil)
	syncTo(a, gameID, b.NodeID())
	if !testutil.WaitFor(60*time.Second, func() bool { return strings.Contains(b.ReadSave("player.sav"), secret) }) {
		t.Fatal("the save never reached the Deck")
	}
	time.Sleep(500 * time.Millisecond)

	type frame struct {
		Type, Route, From      string
		Body, Data             json.RawMessage
		SealedBody, SealedData []byte
	}
	all := spy.snapshot()
	fmt.Printf("\n============ WHAT A THIRD DEVICE IN THE ROOM RECEIVED ============\n")
	fmt.Printf("frames: %d    the save reached the Deck intact: %v    marker: %q\n\n",
		len(all), strings.Contains(b.ReadSave("player.sav"), secret), secret)
	sealed, plain, leaks := 0, 0, 0
	for i, raw := range all {
		var f frame
		_ = json.Unmarshal(raw, &f)
		who := f.From
		if len(who) > 12 {
			who = who[:12] + "…"
		}
		switch {
		case len(f.SealedBody) > 0 || len(f.SealedData) > 0:
			sealed++
			enc := append(append([]byte(nil), f.SealedBody...), f.SealedData...)
			if len(enc) > 12 {
				enc = enc[:12]
			}
			fmt.Printf("  #%02d %-10s %-18s %s  SEALED %5d bytes  %x…\n", i, f.Type, f.Route, who, len(f.SealedBody)+len(f.SealedData), enc)
		case len(f.Body) > 0 || len(f.Data) > 0:
			plain++
			mark := ""
			if strings.Contains(string(raw), secret) {
				leaks++
				mark = "   <<< MARKER VISIBLE"
			}
			p := strings.ReplaceAll(string(f.Body)+string(f.Data), "\n", " ")
			if len(p) > 56 {
				p = p[:56] + "…"
			}
			fmt.Printf("  #%02d %-10s %-18s %s  plain: %s%s\n", i, f.Type, f.Route, who, p, mark)
		default:
			fmt.Printf("  #%02d %-10s %-18s %s  (no payload)\n", i, f.Type, f.Route, who)
		}
	}
	fmt.Printf("\n  sealed payloads: %d    plaintext payloads: %d    frames containing the marker: %d\n", sealed, plain, leaks)
	fmt.Printf("  The %d plaintext payloads are the pairing handshake: a public key and a device name,\n", plain)
	fmt.Printf("  sent before a shared key can exist. Nothing else in the room is readable.\n")
	fmt.Printf("===================================================================\n\n")
}
