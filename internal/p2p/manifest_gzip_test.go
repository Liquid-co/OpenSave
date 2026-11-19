package p2p

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/opensave/opensave/internal/delta"
	"github.com/opensave/opensave/internal/p2p/syncengine"
)

// A large manifest goes over the wire gzipped, and an ordinary Go client —
// which is what every OpenSave version uses — reads it without knowing.
func TestManifestIsCompressedAndReadable(t *testing.T) {
	m := delta.Manifest{Files: map[string]delta.FileEntry{}}
	for i := 0; i < 5000; i++ {
		sum := sha256.Sum256([]byte(fmt.Sprint(i))) // random-looking, like real hashes
		h := hex.EncodeToString(sum[:])
		m.Files[fmt.Sprintf("Saves/world/map_%d_%d.bin", i/100, i%100)] = delta.FileEntry{
			Size: 3500, Hash: h, BlockSize: 65536, Blocks: []delta.Block{{Index: 0, Hash: h, Length: 3500}},
		}
	}
	resp := syncengine.ManifestResponse{Manifest: m, ActiveBranch: "main"}

	var wire atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cw := &countingResponse{ResponseWriter: w}
		jsonOKCompressed(cw, r, resp)
		wire.Store(cw.n)
	}))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	var got syncengine.ManifestResponse
	if err := doJSONWith(lanBulkClient, req, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Manifest.Files) != len(m.Files) || got.Manifest.ManifestHash() != m.ManifestHash() {
		t.Fatalf("manifest came back with %d files and a different hash", len(got.Manifest.Files))
	}

	plain := httptest.NewRecorder()
	jsonOK(plain, resp)
	if wire.Load()*2 > int64(plain.Body.Len()) {
		t.Errorf("compressed %d bytes vs %d plain — expected at least 2x smaller", wire.Load(), plain.Body.Len())
	}
	t.Logf("manifest of %d files: %d bytes plain, %d on the wire", len(m.Files), plain.Body.Len(), wire.Load())
}

type countingResponse struct {
	http.ResponseWriter
	n int64
}

func (c *countingResponse) Write(b []byte) (int, error) {
	n, err := c.ResponseWriter.Write(b)
	c.n += int64(n)
	return n, err
}
