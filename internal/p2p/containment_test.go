package p2p

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/opensave/opensave/internal/store"
)

// makeDirLink makes link a link to the folder target: a junction on Windows,
// which needs no special rights there, and a symbolic link elsewhere.
func makeDirLink(t *testing.T, link, target string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
			t.Skipf("cannot make a junction here: %v %s", err, out)
		}
		return
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot make a symbolic link here: %v", err)
	}
}

// A paired device asking for the blocks of a file behind a link in the save
// folder is not served what is wherever the link points.
func TestBlocks_NotServedThroughALink(t *testing.T) {
	c := newCVEFixture(t)
	root := t.TempDir()
	save := filepath.Join(root, "save")
	outside := filepath.Join(root, "outside")
	for _, d := range []string{save, outside} {
		if err := os.MkdirAll(d, 0o777); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("NOT-A-SAVE-SECRET"), 0o666); err != nil {
		t.Fatal(err)
	}
	makeDirLink(t, filepath.Join(save, "link"), outside)
	if err := c.store.CreateGame(store.Game{ID: "linked", Name: "Linked", SavePath: save, ActiveBranch: "main", AutoSync: true}); err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(map[string]any{"relPath": "link/secret.txt", "blockIndices": []int{0}, "blockSize": 65536})
	req := c.lanRequest(t, http.MethodPost, "/api/p2p/blocks/linked", body)
	req.RemoteAddr = "192.0.2.10:40000"
	rec := httptest.NewRecorder()
	c.router.ServeHTTP(rec, req)

	got, _ := io.ReadAll(rec.Body)
	if rec.Code == http.StatusOK || strings.Contains(string(got), "NOT-A-SAVE-SECRET") ||
		strings.Contains(string(got), "Tk9ULUEtU0FWRS1TRUNSRVQ") { // the secret, base64
		t.Fatalf("a file outside the save folder was served through a link: HTTP %d %s", rec.Code, got)
	}
}
