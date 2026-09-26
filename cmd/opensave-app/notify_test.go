package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// Where a click goes survives the trip through Windows, which carries it in
// an XML attribute that nothing escapes.
func TestNotificationTargetSurvivesTheTrip(t *testing.T) {
	open := `{"view":"game","params":{"gameId":"hades & co \"x\" <y>"}}`
	enc := encodeOpen(open)
	if strings.ContainsAny(enc, `"<>&'`) {
		t.Errorf("encoded target %q still has characters XML would need escaped", enc)
	}
	if got := decodeOpen(enc); got != open {
		t.Errorf("decoded %q, want %q", got, open)
	}
	if decodeOpen("something else") != "" || encodeOpen("") != "" {
		t.Error("an empty or foreign target should come back empty")
	}
	if got := plainText("a ]]> b"); strings.Contains(got, "]]>") {
		t.Errorf("plainText left %q", got)
	}
}

// A notification's picture is fetched from the daemon's covers and nowhere
// else: the URL comes from the page.
func TestNotificationImageOnlyFromTheDaemon(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("jpeg"))
	}))
	defer srv.Close()
	a := &App{addr: strings.TrimPrefix(srv.URL, "http://")}

	path := a.noteImage(srv.URL + "/api/cover?appId=1145360")
	if path == "" {
		t.Fatal("the daemon's cover was not fetched")
	}
	defer os.Remove(path)
	if b, _ := os.ReadFile(path); string(b) != "jpeg" {
		t.Errorf("fetched %q", b)
	}
	for _, url := range []string{
		"https://example.com/cover.jpg",
		srv.URL + "/api/games",
		"http://127.0.0.1:1/api/cover?appId=1",
		"",
	} {
		if got := a.noteImage(url); got != "" {
			t.Errorf("noteImage(%q) fetched %s", url, got)
		}
	}
}
