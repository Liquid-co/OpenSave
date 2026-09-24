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

// A backup takes as long as the saves in it. Export and import were given the
// same few seconds as a status call, so a large library was reported as the
// daemon not answering — while it went on writing the file — and a test
// importing two small saves failed that way under load.
func TestBackupCommandsWaitForTheDaemon(t *testing.T) {
	const answerAfter = 300 * time.Millisecond
	slowDaemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(answerAfter)
		switch r.URL.Path {
		case "/api/backup/export":
			_, _ = w.Write([]byte(`{"exported":1}`))
		case "/api/backup/restore":
			_, _ = w.Write([]byte(`{"restored":1}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer slowDaemon.Close()

	home := t.TempDir()
	for _, v := range []string{"HOME", "USERPROFILE"} {
		t.Setenv(v, home)
	}
	dataDir := filepath.Join(home, ".opensave")
	if err := os.MkdirAll(dataDir, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "daemon.addr"), []byte(strings.TrimPrefix(slowDaemon.URL, "http://")), 0o666); err != nil {
		t.Fatal(err)
	}

	// A quick call gives up well before the answer; a slow one waits for it.
	quick, slow := httpClient, slowClient
	httpClient = &http.Client{Timeout: answerAfter / 3}
	slowClient = &http.Client{Timeout: 10 * answerAfter}
	defer func() { httpClient, slowClient = quick, slow }()

	archive := filepath.Join(home, "all.sscb")
	if code := cmdBackup([]string{"export", archive, "some-game"}); code != 0 {
		t.Errorf("backup export gave up on a daemon that was still working (exit %d)", code)
	}
	if err := os.WriteFile(archive, []byte("x"), 0o666); err != nil {
		t.Fatal(err)
	}
	if code := cmdBackup([]string{"import", archive, "--overwrite"}); code != 0 {
		t.Errorf("backup import gave up on a daemon that was still working (exit %d)", code)
	}
}
