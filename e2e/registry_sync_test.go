package e2e

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/opensave/opensave/internal/winreg"
	"github.com/opensave/opensave/testutil"
)

// A registry save has to reach the other device, or the feature only protects
// one machine.
//
// The capture is a file in a save location precisely so that it can: every
// mechanism that moves a save between devices is defined over files, and a
// registry key held anywhere else would be invisible to all of them. This
// drives two real daemons and checks the capture arrives.
//
// Both daemons here share one machine, and therefore one registry — so what is
// worth proving is that the CAPTURE crosses, not that a value reappears in a
// registry it never left. Writing the values back is covered by the snapshot
// tests, which restore into a scratch key of their own.
func TestRegistryCaptureReachesThePeer(t *testing.T) {
	a, b, gameID := pairAndTrack(t, "RegSync", map[string]string{"save.sav": "primary v1"})

	// Both devices know a registry location, as the daemon sets up when it
	// tracks a game the manifest says keeps saves there.
	aReg := extraDir(t, a, winreg.LocationName)
	bReg := extraDir(t, b, winreg.LocationName)
	addRoot(t, a, gameID, winreg.LocationName, aReg)
	addRoot(t, b, gameID, winreg.LocationName, bReg)

	// A capture as CaptureToDir writes one. Written directly rather than read
	// from a real key: the point here is the transport, and a fixture keeps
	// the test off whatever this machine happens to have in its registry.
	const captured = `{"requested":["HKEY_CURRENT_USER\\Software\\Test\\Game"],` +
		`"keys":[{"path":"HKEY_CURRENT_USER\\Software\\Test\\Game",` +
		`"values":[{"name":"chapter","type":"DWORD","data":"7"}]}]}`
	writeIn(t, aReg, winreg.FileName, captured)

	time.Sleep(syncSettleWindow)
	a.API(http.MethodPost, "/api/games/"+gameID+"/sync", nil, nil)

	if !testutil.WaitFor(45*time.Second, func() bool {
		return readIn(bReg, winreg.FileName) == captured
	}) {
		t.Fatalf("the registry capture never reached the peer; peer holds %q",
			readIn(bReg, winreg.FileName))
	}
}

// A registry save that changes has to propagate like any other edit. The first
// capture arriving proves the location syncs; this proves an update does, which
// is the case a stale merge base would break.
func TestAChangedRegistryCaptureReachesThePeer(t *testing.T) {
	a, b, gameID := pairAndTrack(t, "RegUpdate", map[string]string{"save.sav": "primary v1"})

	aReg := extraDir(t, a, winreg.LocationName)
	bReg := extraDir(t, b, winreg.LocationName)
	addRoot(t, a, gameID, winreg.LocationName, aReg)
	addRoot(t, b, gameID, winreg.LocationName, bReg)

	first := `{"keys":[{"path":"HKCU\\Software\\Test","values":[{"name":"chapter","type":"DWORD","data":"1"}]}]}`
	writeIn(t, aReg, winreg.FileName, first)
	time.Sleep(syncSettleWindow)
	a.API(http.MethodPost, "/api/games/"+gameID+"/sync", nil, nil)
	if !testutil.WaitFor(45*time.Second, func() bool {
		return readIn(bReg, winreg.FileName) == first
	}) {
		t.Fatal("the first capture never reached the peer")
	}

	// The game is played on, and the next snapshot recaptures.
	second := `{"keys":[{"path":"HKCU\\Software\\Test","values":[{"name":"chapter","type":"DWORD","data":"2"}]}]}`
	writeIn(t, aReg, winreg.FileName, second)
	time.Sleep(syncSettleWindow)
	a.API(http.MethodPost, "/api/games/"+gameID+"/sync", nil, nil)

	if !testutil.WaitFor(45*time.Second, func() bool {
		return readIn(bReg, winreg.FileName) == second
	}) {
		t.Fatalf("the updated capture never reached the peer; peer holds %q",
			readIn(bReg, winreg.FileName))
	}
}

// A device with no folder for the registry location must not be sent the
// capture into its save folder. That would put a registry dump among the
// game's files, where the game would read it as save data.
func TestAPeerWithNoRegistryFolderDoesNotGetTheCaptureInItsSave(t *testing.T) {
	a, b, gameID := pairAndTrack(t, "RegUnmapped", map[string]string{"save.sav": "primary v1"})

	aReg := extraDir(t, a, winreg.LocationName)
	addRoot(t, a, gameID, winreg.LocationName, aReg)
	// b deliberately gets no registry location at all.

	writeIn(t, aReg, winreg.FileName, `{"keys":[]}`)
	time.Sleep(syncSettleWindow)
	a.API(http.MethodPost, "/api/games/"+gameID+"/sync", nil, nil)
	time.Sleep(syncSettleWindow)

	stray := filepath.Join(b.SaveDir, winreg.FileName)
	if _, err := os.Stat(stray); err == nil {
		t.Errorf("the capture landed in the peer's save folder at %q — a game reading "+
			"that folder would see a registry dump as save data", stray)
	}
}
