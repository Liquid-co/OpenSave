package presets

import (
	"os"
	"path/filepath"
	"testing"
)

// While detection holds a file open to judge it, the game can still delete
// it: the read goes through fsx.OpenShared, not os.Open, whose handle
// refuses deletion on Windows.
func TestOpenForJudging_LetsTheGameDeleteTheFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "video.cfg")
	if err := os.WriteFile(p, []byte("setting.fullscreen 1"), 0o666); err != nil {
		t.Fatal(err)
	}
	f, err := openForJudging(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := os.Remove(p); err != nil {
		t.Errorf("the game could not delete a file being judged: %v", err)
	}
}
