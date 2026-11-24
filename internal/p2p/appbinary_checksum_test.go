package p2p

import (
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
)

// A peer's build that arrives with no checksum is refused, not installed
// unchecked.
func TestDownloadPeerBinaryLAN_RefusesAMissingChecksum(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-App-Version", "2.4.1")
		_, _ = w.Write([]byte("not really OpenSave"))
	}))
	defer srv.Close()
	host, portStr, _ := net.SplitHostPort(srv.Listener.Addr().String())
	port, _ := strconv.Atoi(portStr)

	e := &Engine{Log: func(string, string) {}}
	if _, err := e.downloadPeerBinaryLAN(t.Context(), host, port, filepath.Join(t.TempDir(), "OpenSave.exe.new"), nil); err == nil {
		t.Error("a build with no checksum was accepted")
	}
}
