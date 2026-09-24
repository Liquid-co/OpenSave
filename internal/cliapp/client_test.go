package cliapp

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A daemon that is running but slow to answer must not be reported as not
// running. The message used to tell people to start a daemon that was, at
// that moment, busy doing what they had asked.
func TestSlowDaemonIsNotReportedAsMissing(t *testing.T) {
	release := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	defer slow.Close()
	defer close(release)

	home := t.TempDir()
	for _, v := range []string{"HOME", "USERPROFILE"} {
		t.Setenv(v, home)
	}
	dataDir := filepath.Join(home, ".opensave")
	if err := os.MkdirAll(dataDir, 0o777); err != nil {
		t.Fatal(err)
	}
	addr := strings.TrimPrefix(slow.URL, "http://")
	if err := os.WriteFile(filepath.Join(dataDir, "daemon.addr"), []byte(addr), 0o666); err != nil {
		t.Fatal(err)
	}

	_, err := daemonRequestWith(&http.Client{Timeout: 100 * time.Millisecond}, "POST", "/api/cloud/check", nil)
	if err == nil {
		t.Fatal("a request that timed out reported success")
	}
	if strings.Contains(err.Error(), "start it") {
		t.Errorf("a slow daemon was reported as missing: %v", err)
	}
	if !strings.Contains(err.Error(), "did not answer") {
		t.Errorf("error = %v, want it to say the daemon did not answer in time", err)
	}
}
