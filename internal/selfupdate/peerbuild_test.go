package selfupdate

import (
	"strings"
	"testing"
)

func TestPeerBuildAssetURL(t *testing.T) {
	got, err := PeerBuildAssetURL("Liquid-co/OpenSave", "windows", "2.4.1")
	if err != nil || got != "https://github.com/Liquid-co/OpenSave/releases/download/v2.4.1/OpenSave.exe" {
		t.Errorf("windows 2.4.1 = %q, %v", got, err)
	}
	if got, err := PeerBuildAssetURL("Liquid-co/OpenSave", "windows", "v2.5.0-beta.1"); err != nil || !strings.HasSuffix(got, "/v2.5.0-beta.1/OpenSave.exe") {
		t.Errorf("a v-prefixed beta = %q, %v", got, err)
	}
	// Fails closed: nothing published to check the binary against, or a
	// version that is the peer's to write and could steer the URL.
	for _, c := range []struct{ goos, version string }{
		{"linux", "2.4.1"},
		{"darwin", "2.4.1"},
		{"windows", ""},
		{"windows", "2.4.1/../../../evil/releases/download/v1"},
		{"windows", "2.4.1?x=y"},
	} {
		if got, err := PeerBuildAssetURL("Liquid-co/OpenSave", c.goos, c.version); err == nil {
			t.Errorf("%s %q = %q, want refused", c.goos, c.version, got)
		}
	}
}
