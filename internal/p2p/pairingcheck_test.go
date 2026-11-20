package p2p

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/opensave/opensave/internal/e2ee"
	"github.com/opensave/opensave/internal/p2p/pairing"
	"github.com/opensave/opensave/internal/store"
)

// pairSide is one device with its peer routes served on a real port.
type pairSide struct {
	e    *Engine
	s    *store.Store
	url  string
	port int
	id   string
	key  string
}

func newPairSide(t *testing.T, name string) *pairSide {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "opensave.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.EnsureDefaultSettings(t.TempDir(), t.TempDir()); err != nil {
		t.Fatal(err)
	}
	e := &Engine{Store: s, Log: func(string, string) {}, Pairing: pairing.New()}
	r := chi.NewRouter()
	e.RegisterRoutes(r)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	settings, err := s.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings.Port = portOf(t, srv.Listener.Addr().String())
	settings.DeviceName = name
	if err := s.UpdateSettings(settings); err != nil {
		t.Fatal(err)
	}
	id, err := s.DeviceIdentity()
	if err != nil {
		t.Fatal(err)
	}
	return &pairSide{e: e, s: s, url: srv.URL, port: settings.Port, id: settings.NodeID, key: e2ee.EncodeKey(id.Public)}
}

func portOf(t *testing.T, addr string) int {
	t.Helper()
	_, p, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	n, _ := strconv.Atoi(p)
	return n
}

var fourDigits = regexp.MustCompile(`^\d{4}$`)

func onlyRequest(t *testing.T, side *pairSide) pairing.IncomingRequest {
	t.Helper()
	reqs := side.e.Pairing.PendingRequests()
	if len(reqs) != 1 {
		t.Fatalf("%d pending requests, want 1", len(reqs))
	}
	return reqs[0]
}

func TestPairingCheck_BothDevicesShowTheSameCode(t *testing.T) {
	pc, deck := newPairSide(t, "Gaming PC"), newPairSide(t, "Steam Deck")
	ctx := context.Background()

	code, err := pc.e.InitiatePair(ctx, "127.0.0.1", deck.port)
	if err != nil {
		t.Fatal(err)
	}
	if !fourDigits.MatchString(code) {
		t.Fatalf("the asking device got %q, want four digits", code)
	}
	req := onlyRequest(t, deck)
	if !req.Checks || req.CheckCode != code {
		t.Fatalf("the asked device shows checks=%v code %q, want %q", req.Checks, req.CheckCode, code)
	}

	if err := deck.e.ApprovePairing(ctx, pc.id, code); err != nil {
		t.Fatal(err)
	}
	onPC, err := pc.s.GetPeer(deck.id)
	if err != nil {
		t.Fatalf("the asking device did not store the pairing: %v", err)
	}
	if onPC.PublicKey != deck.key {
		t.Error("the asking device pinned a key that is not the Deck's")
	}
	onDeck, err := deck.s.GetPeer(pc.id)
	if err != nil || onDeck.PublicKey != pc.key {
		t.Errorf("the asked device pinned %q (err %v), want the PC's key", onDeck.PublicKey, err)
	}
}

// The handshake was answered with one key; a confirm carrying another, from
// the same address, is not from the device whose code the user compared.
func TestPairingCheck_ConfirmWithAnotherKeyIsRefused(t *testing.T) {
	pc, deck := newPairSide(t, "Gaming PC"), newPairSide(t, "Steam Deck")
	ctx := context.Background()
	code, err := pc.e.InitiatePair(ctx, "127.0.0.1", deck.port)
	if err != nil {
		t.Fatal(err)
	}

	other, _ := e2ee.GenerateIdentity()
	body, _ := json.Marshal(map[string]any{
		"peerId": deck.id, "deviceName": "Steam Deck", "port": deck.port,
		"publicKey": e2ee.EncodeKey(other.Public),
	})
	resp, err := http.Post(pc.url+"/api/p2p/approve-confirm", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatal("a confirm with a key the handshake was not answered with was accepted")
	}
	if _, err := pc.s.GetPeer(deck.id); err == nil {
		t.Fatal("the refused confirm stored a pairing")
	}

	// And it did not spoil the real one.
	if err := deck.e.ApprovePairing(ctx, pc.id, code); err != nil {
		t.Fatal(err)
	}
	if got, err := pc.s.GetPeer(deck.id); err != nil || got.PublicKey != deck.key {
		t.Fatalf("after the real approval the PC holds %q (err %v), want the Deck's key", got.PublicKey, err)
	}
}

// Asked again, the Deck shows new numbers. Approving with the old ones, still
// on screen from a moment ago, is refused rather than applied to the new
// request.
func TestPairingCheck_ApprovingWithReplacedNumbersIsRefused(t *testing.T) {
	pc, deck := newPairSide(t, "Gaming PC"), newPairSide(t, "Steam Deck")
	ctx := context.Background()
	first, err := pc.e.InitiatePair(ctx, "127.0.0.1", deck.port)
	if err != nil {
		t.Fatal(err)
	}
	second := first
	for i := 0; second == first && i < 5; i++ { // one in 10,000 repeats
		if second, err = pc.e.InitiatePair(ctx, "127.0.0.1", deck.port); err != nil {
			t.Fatal(err)
		}
	}

	if err := deck.e.ApprovePairing(ctx, pc.id, first); !errors.Is(err, pairing.ErrCheckChanged) {
		t.Fatalf("approving with the replaced code: err = %v, want ErrCheckChanged", err)
	}
	if err := deck.e.ApprovePairing(ctx, pc.id, second); err != nil {
		t.Fatalf("approving with the current code: %v", err)
	}
}

// Someone on the network relays the handshake and puts its own keys in both
// directions. Each device works its code out from the keys it was given, so
// the two codes come from different keys and the user sees them differ.
func TestPairingCheck_AKeySwapInTheMiddleShowsDifferentCodes(t *testing.T) {
	pc, deck := newPairSide(t, "Gaming PC"), newPairSide(t, "Steam Deck")
	towardDeck, _ := e2ee.GenerateIdentity()
	towardPC, _ := e2ee.GenerateIdentity()
	var askerNonce, answererNonce string

	relay := func(path string, body map[string]any) map[string]any {
		raw, _ := json.Marshal(body)
		resp, err := http.Post(deck.url+path, "application/json", bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		out := map[string]any{}
		data, _ := io.ReadAll(resp.Body)
		_ = json.Unmarshal(data, &out)
		return out
	}
	mitm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := map[string]any{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch r.URL.Path {
		case "/api/p2p/handshake":
			body["publicKey"] = e2ee.EncodeKey(towardDeck.Public)
			reply := relay(r.URL.Path, body)
			answererNonce, _ = reply["checkNonce"].(string)
			reply["publicKey"] = e2ee.EncodeKey(towardPC.Public)
			_ = json.NewEncoder(w).Encode(reply)
		default:
			if n, ok := body["nonce"].(string); ok {
				askerNonce = n
			}
			_ = json.NewEncoder(w).Encode(relay(r.URL.Path, body))
		}
	}))
	defer mitm.Close()

	pcCode, err := pc.e.InitiatePair(context.Background(), "127.0.0.1", portOf(t, mitm.Listener.Addr().String()))
	if err != nil {
		t.Fatal(err)
	}
	deckCode := onlyRequest(t, deck).CheckCode
	if pcCode == "" || deckCode == "" {
		t.Fatalf("codes %q and %q, want both shown", pcCode, deckCode)
	}

	wantPC := pairing.CheckCode(pc.key, e2ee.EncodeKey(towardPC.Public), askerNonce, answererNonce)
	wantDeck := pairing.CheckCode(e2ee.EncodeKey(towardDeck.Public), deck.key, askerNonce, answererNonce)
	if pcCode != wantPC || deckCode != wantDeck {
		t.Fatalf("codes not from the keys each side holds: PC %q want %q, Deck %q want %q", pcCode, wantPC, deckCode, wantDeck)
	}
	if pcCode == deckCode {
		// Two different key sets: equal only by a one-in-10,000 chance.
		t.Logf("the two codes collided by chance (%s)", pcCode)
	}
}

func TestPairingCheck_AskerWithoutTheCheckShowsNoCode(t *testing.T) {
	deck := newPairSide(t, "Steam Deck")
	other, _ := e2ee.GenerateIdentity()
	body, _ := json.Marshal(map[string]any{
		"peerId": "old-pc", "deviceName": "Old PC", "port": 8383,
		"publicKey": e2ee.EncodeKey(other.Public),
	})
	resp, err := http.Post(deck.url+"/api/p2p/handshake", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var reply handshakeReply
	_ = json.NewDecoder(resp.Body).Decode(&reply)
	if reply.CheckNonce != "" {
		t.Error("a nonce went to a device that never committed to one")
	}
	if reply.PublicKey != deck.key {
		t.Error("the answer did not carry this device's key")
	}
	if req := onlyRequest(t, deck); req.Checks || req.CheckCode != "" {
		t.Fatalf("request from an older version shows checks=%v code %q", req.Checks, req.CheckCode)
	}
}

func TestPairingCheck_AnswererWithoutTheCheckGivesNoCode(t *testing.T) {
	pc := newPairSide(t, "Gaming PC")
	old := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"pending","message":"Pairing request received. Waiting for host approval."}`))
	}))
	defer old.Close()
	port := portOf(t, old.Listener.Addr().String())

	code, err := pc.e.InitiatePair(context.Background(), "127.0.0.1", port)
	if err != nil || code != "" {
		t.Fatalf("InitiatePair = %q, %v; want no code and no error", code, err)
	}
	if pc.e.Pairing.KeyMismatch("old", "127.0.0.1", port, "any-key") {
		t.Fatal("a key was pinned from an answer that carried none")
	}
}
