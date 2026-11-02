package p2p

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/opensave/opensave/internal/store"
)

// CVE-2026-103398: a paired device — or anyone able to pose as one — named a
// folder in a manifest request, and this device tracked it as a game and
// served its files. Here the folder holds an SSH key.
//
// The attacker signs its requests properly: what is tested is the path, not
// the authentication (see the unsigned tests below for that).

type cveFixture struct {
	*authFixture
	router http.Handler
	secret string
	home   string
}

func newCVEFixture(t *testing.T) *cveFixture {
	t.Helper()
	// Auto-tracking creates folders under the home a path translates to.
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)

	f := newAuthFixture(t, true)
	peer := f.peer
	peer.Address = "192.0.2.10"
	if err := f.store.UpdatePeer(peer); err != nil {
		t.Fatal(err)
	}
	f.peer = peer

	secret := filepath.Join(t.TempDir(), "ssh")
	if err := os.MkdirAll(secret, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(secret, "id_ed25519"), []byte("-----BEGIN OPENSSH PRIVATE KEY-----"), 0o600); err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	f.engine.RegisterRoutes(r)
	return &cveFixture{authFixture: f, router: r, secret: secret, home: home}
}

// askManifest is the attacker's request: a game it makes up, at the folder it
// wants.
func (c *cveFixture) askManifest(t *testing.T, gameID, savePath string) *httptest.ResponseRecorder {
	t.Helper()
	q := url.Values{"name": {"Totally A Game"}, "savePath": {savePath}}
	req := c.lanRequest(t, http.MethodGet, "/api/p2p/manifest/"+gameID+"?"+q.Encode(), nil)
	req.RemoteAddr = "192.0.2.10:40000"
	rec := httptest.NewRecorder()
	c.router.ServeHTTP(rec, req)
	return rec
}

func TestCVE_2026_103398_APeerCannotHaveAnyFolderTracked(t *testing.T) {
	c := newCVEFixture(t)

	rec := c.askManifest(t, "totally-a-game", c.secret)

	body, _ := io.ReadAll(rec.Body)
	if strings.Contains(string(body), "id_ed25519") {
		t.Fatalf("the folder's files were listed to the peer: HTTP %d %s", rec.Code, body)
	}
	if rec.Code == http.StatusOK {
		t.Fatalf("the manifest was served: HTTP %d %s", rec.Code, body)
	}
	if g, err := c.store.GetGame("totally-a-game"); err == nil {
		t.Fatalf("the folder was tracked as a game at %s", g.SavePath)
	}
	// Not dropped: offered, for the user to place on this device if it is
	// a real game.
	offered, err := c.store.ListOfferedGames()
	if err != nil {
		t.Fatal(err)
	}
	if len(offered) != 1 || offered[0].GameID != "totally-a-game" {
		t.Errorf("offered games = %+v, want the one asked for", offered)
	}
}

// What this device's own scanner knows as a save folder still syncs by
// itself, as before.
func TestCVE_2026_103398_AKnownSaveFolderStillAutoTracks(t *testing.T) {
	c := newCVEFixture(t)
	// Where Hollow Knight keeps its save under this device's home: the folder
	// the other PC's path names, once translated.
	save := filepath.Join(c.home, "AppData", "LocalLow", "Team Cherry", "Hollow Knight")
	if err := os.MkdirAll(save, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(save, "user1.dat"), []byte("save"), 0o666); err != nil {
		t.Fatal(err)
	}
	same := func(a, b string) bool { return strings.EqualFold(filepath.Clean(a), filepath.Clean(b)) }
	c.engine.KnownSaveLocation = func(_, _, p string) bool { return same(p, save) }
	peerPath := `C:\Users\alice\AppData\LocalLow\Team Cherry\Hollow Knight`

	// The tracking step itself: serving the manifest needs a whole sync
	// engine, which this fixture does not build.
	g, err := c.engine.ensureManifestGame("hollow-knight", manifestGameQuery{Name: "Hollow Knight", SavePath: peerPath}, c.peer.ID, c.peer.ID)
	if err != nil {
		t.Fatalf("a known save folder was refused: %v", err)
	}
	if !same(g.SavePath, save) {
		t.Errorf("tracked at %s, want the known folder %s", g.SavePath, save)
	}
}

// The second half of the report: requests that were never signed with the
// pairing's key are not served, over the network or the relay.
func TestCVE_2026_103398_UnsignedLANRequestIsRefused(t *testing.T) {
	c := newCVEFixture(t)
	// A device that has not signed anything yet, so the old latch had not
	// closed: the window the advisory's spoofing walked through.
	if err := c.store.MarkPeerAuthVerified(c.peer.ID, 0); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/p2p/games", nil)
	req.RemoteAddr = "192.0.2.10:40000"
	rec := httptest.NewRecorder()
	c.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("an unsigned request from the paired address was served: HTTP %d", rec.Code)
	}
}

func TestCVE_2026_103398_UnsignedRelayRequestFromAKeylessPairingIsRefused(t *testing.T) {
	f := newAuthFixture(t, false) // paired over the relay before 2.4.0: no key
	w := &WanClient{engine: f.engine}
	for _, route := range []string{"/games", "/manifest/g1", "/blocks/g1", "/delete-file/g1"} {
		status, _ := w.routeRequest(t.Context(), RelayMessage{Type: "request", From: f.peer.ID, Route: route, Method: http.MethodGet})
		if status != http.StatusUnauthorized {
			t.Errorf("%s from a keyless pairing: HTTP %d, want 401", route, status)
		}
	}
	// Unpairing still works, so such a pair can be cleared up.
	if status, _ := w.routeRequest(t.Context(), RelayMessage{Type: "request", From: f.peer.ID, Route: "/unpair", Method: http.MethodPost}); status == http.StatusUnauthorized {
		t.Error("unpairing a keyless pairing was refused")
	}
}

var _ = store.Peer{}

// A device the user lets choose folders — their own — has its game tracked at
// the folder it names, as before 2.4.1; any other device's is offered.
func TestADeviceLetChooseFoldersHasItsFolderTracked(t *testing.T) {
	c := newCVEFixture(t)
	// The folder the other PC names, and where it lands here: under this
	// device's home, translated as it always was.
	q := manifestGameQuery{Name: "Homebrew Game", SavePath: `C:\Users\alice\My Custom Saves`}
	custom := filepath.Join(c.home, "My Custom Saves")

	if _, err := c.engine.ensureManifestGame("homebrew-game", q, c.peer.ID, c.peer.ID); err == nil {
		t.Fatal("an unvouched folder was tracked for a device not let choose folders")
	}
	if err := c.store.SetPeerChoosesFolders(c.peer.ID, true); err != nil {
		t.Fatal(err)
	}
	g, err := c.engine.ensureManifestGame("homebrew-game", q, c.peer.ID, c.peer.ID)
	if err != nil {
		t.Fatalf("the trusted device's folder was refused: %v", err)
	}
	if !strings.EqualFold(filepath.Clean(g.SavePath), filepath.Clean(custom)) {
		t.Errorf("tracked at %s, want the folder it named %s", g.SavePath, custom)
	}
	// It was offered on the first attempt; tracked now, the offer is gone.
	if offered, _ := c.store.ListOfferedGames(); len(offered) != 0 {
		t.Errorf("the offer outlived the game being tracked: %+v", offered)
	}
}

// Trust is the device's that proved itself, never whoever asks: another
// paired device is not trusted because one is.
func TestTrustIsPerDevice(t *testing.T) {
	c := newCVEFixture(t)
	if err := c.store.UpsertPeer(store.Peer{ID: "peer-other", Name: "Other", Address: "192.0.2.99"}); err != nil {
		t.Fatal(err)
	}
	if err := c.store.SetPeerChoosesFolders("peer-other", true); err != nil {
		t.Fatal(err)
	}
	q := manifestGameQuery{Name: "Homebrew Game", SavePath: filepath.Join(t.TempDir(), "x")}
	if _, err := c.engine.ensureManifestGame("homebrew-game", q, c.peer.ID, c.peer.ID); err == nil {
		t.Fatal("one device's trust let another choose a folder")
	}
	if _, err := c.engine.ensureManifestGame("homebrew-game", q, "", ""); err == nil {
		t.Fatal("a request from no device was trusted")
	}
}

// An offer answers itself once this device can vouch for the folder: the game
// is tracked on the peer's next attempt and the offer disappears.
func TestAnOfferClearsOnceTheFolderIsKnown(t *testing.T) {
	c := newCVEFixture(t)
	folder := filepath.Join(t.TempDir(), "Hollow Knight")
	known := false
	c.engine.KnownSaveLocation = func(_, _, p string) bool { return known }
	offers := 0
	c.engine.OnGameOffered = func() { offers++ }
	q := manifestGameQuery{Name: "Hollow Knight", SavePath: folder}

	if _, err := c.engine.ensureManifestGame("hollow-knight", q, c.peer.ID, c.peer.ID); err == nil {
		t.Fatal("tracked before the folder was known")
	}
	if offers != 1 {
		t.Errorf("the offer asked for %d scans, want 1", offers)
	}
	known = true // the scan found it
	if _, err := c.engine.ensureManifestGame("hollow-knight", q, c.peer.ID, c.peer.ID); err != nil {
		t.Fatalf("not tracked once known: %v", err)
	}
	if offered, _ := c.store.ListOfferedGames(); len(offered) != 0 {
		t.Errorf("the offer is still shown for a game now syncing: %+v", offered)
	}
}

// Trust follows the device the signature proved, not the one the request is
// attributed to: a request whose sender was only inferred, from an address,
// may not choose a folder even when that address belongs to a trusted device.
func TestTrustNeedsTheProvenDevice(t *testing.T) {
	c := newCVEFixture(t)
	if err := c.store.SetPeerChoosesFolders(c.peer.ID, true); err != nil {
		t.Fatal(err)
	}
	q := manifestGameQuery{Name: "Homebrew Game", SavePath: `C:\Users\alice\Homebrew`}
	if _, err := c.engine.ensureManifestGame("homebrew-game", q, c.peer.ID, ""); err == nil {
		t.Fatal("a folder was trusted for a request no signature proved")
	}
}
