package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// GitHub #39: signing in to a cloud provider, the browser came back to
// http://localhost/callback and found nothing listening.

func startSignIn(t *testing.T, ts *testServer) {
	t.Helper()
	resp, raw := ts.do(t, http.MethodPost, "/api/auth/start", map[string]string{"provider": "dropbox"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("auth/start: %d", resp.StatusCode)
	}
	var auto bool
	_ = json.Unmarshal(raw["autoCallback"], &auto)
	if !auto {
		t.Skip("port 80 cannot be used here (on Linux it needs root): the listener is not tested")
	}
}

func getCallback(t *testing.T, query string) (int, string, error) {
	t.Helper()
	client := &http.Client{Timeout: 3 * time.Second}
	r, err := client.Get("http://127.0.0.1/callback" + query)
	if err != nil {
		return 0, "", err
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	return r.StatusCode, string(b), nil
}

// refusingTokenEndpoint stands in for Dropbox's token endpoint and refuses
// every code, as Dropbox does a made-up one. A callback with a code exchanges
// it there before answering the browser: against the real endpoint, a slow
// answer from Dropbox outlasted getCallback's 3 seconds and failed the test.
// It returns how many exchanges reached it.
func refusingTokenEndpoint(t *testing.T, ts *testServer) *atomic.Int32 {
	t.Helper()
	var hits atomic.Int32
	token := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":"invalid_grant"}`)
	}))
	t.Cleanup(token.Close)
	ts.daemon.Cloud.Endpoints.DropboxToken = token.URL
	return &hits
}

func pending() string {
	pendingPKCE.Lock()
	defer pendingPKCE.Unlock()
	return pendingPKCE.provider
}

// An abandoned attempt's timer must not close a newer attempt's listener.
func TestSignIn_AnOldAttemptsTimerLeavesTheNewListener(t *testing.T) {
	saved := authCallbackTimeout
	authCallbackTimeout = 600 * time.Millisecond
	t.Cleanup(func() { authCallbackTimeout = saved; activeAuthListener.stop() })
	ts := startTestServer(t)

	startSignIn(t, ts)                 // abandoned
	time.Sleep(400 * time.Millisecond) // and tried again
	startSignIn(t, ts)
	time.Sleep(400 * time.Millisecond) // the first attempt's timer has fired

	if _, _, err := getCallback(t, ""); err != nil {
		t.Fatalf("the new attempt's listener was closed by the old attempt's timer: %v", err)
	}
	time.Sleep(500 * time.Millisecond) // and its own timer has fired
	if _, _, err := getCallback(t, ""); err == nil {
		t.Error("the listener was still up after its own timeout")
	}
}

// A request with no code in it — a reload, a second tab — neither closes the
// listener nor uses up the sign-in the real redirect needs.
func TestSignIn_AStrayRequestUsesNothingUp(t *testing.T) {
	t.Cleanup(func() { activeAuthListener.stop() })
	ts := startTestServer(t)
	exchanges := refusingTokenEndpoint(t, ts)
	startSignIn(t, ts)

	if _, _, err := getCallback(t, ""); err != nil {
		t.Fatal(err)
	}
	if pending() == "" {
		t.Error("a request with no code used up the pending sign-in")
	}
	if _, _, err := getCallback(t, "?code=not-a-real-code"); err != nil {
		t.Errorf("the listener closed after a stray request: %v", err)
	}
	// A failed exchange uses the flow up, but leaves the listener: it closes
	// on success or at its timeout.
	if _, _, err := getCallback(t, "?code=again"); err != nil {
		t.Errorf("the listener closed after a failed exchange: %v", err)
	}
	if n := exchanges.Load(); n != 1 {
		t.Errorf("%d exchanges reached the stand-in token endpoint, want 1: the first code is exchanged, the used-up flow sends nothing", n)
	}
}

// What the address says is shown as text on the page, never as markup.
func TestSignIn_ThePageEscapesWhatTheAddressSays(t *testing.T) {
	t.Cleanup(func() { activeAuthListener.stop() })
	ts := startTestServer(t)
	startSignIn(t, ts)

	_, body, err := getCallback(t, "?error=%3Cscript%3Ealert(1)%3C%2Fscript%3E")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(body, "<script>") {
		t.Error("the provider's error text was written into the page as markup")
	}
}

func TestPastedAuthCode(t *testing.T) {
	for pasted, want := range map[string]string{
		"4/0AXlqoi41_wLN4":    "4/0AXlqoi41_wLN4",
		"  4/0AXlqoi41_wLN4 ": "4/0AXlqoi41_wLN4",
		"http://localhost/callback?iss=https://accounts.google.com&code=4/0AXlqoi41_wLN4&scope=email%20openid&authuser=1": "4/0AXlqoi41_wLN4",
		"localhost/callback?code=abc123&state=x":   "abc123",
		"code=abc123":                              "abc123",
		"http://localhost/callback?code=a%2Fb#top": "a/b",
	} {
		got, err := pastedAuthCode(pasted)
		if err != nil || got != want {
			t.Errorf("pastedAuthCode(%q) = %q, %v; want %q", pasted, got, err, want)
		}
	}
	for _, pasted := range []string{
		"http://localhost/callback?error=access_denied",
		"http://localhost/callback?state=x&code=",
	} {
		if got, err := pastedAuthCode(pasted); err == nil {
			t.Errorf("pastedAuthCode(%q) = %q, want an error", pasted, got)
		}
	}
}
