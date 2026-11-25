package e2e

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/opensave/opensave/internal/p2p"
	"github.com/opensave/opensave/internal/p2p/syncengine"
	"github.com/opensave/opensave/testutil"
)

// TestBench_ManySmallFiles times pulling a save of many small files between
// two real daemons, batched and file by file. Opt-in, because it is a
// measurement rather than a check: OPENSAVE_BENCH_FILES=5000 go test ./e2e -run TestBench -v
func TestBench_ManySmallFiles(t *testing.T) {
	n, _ := strconv.Atoi(os.Getenv("OPENSAVE_BENCH_FILES"))
	if n <= 0 {
		t.Skip("set OPENSAVE_BENCH_FILES to run")
	}
	for _, mode := range []struct {
		name  string
		proto int
	}{{"batched", syncengine.ProtoBatchFiles}, {"file-by-file", syncengine.ProtoMultiRoot}} {
		t.Run(mode.name, func(t *testing.T) {
			restore := p2p.SetServedProto(mode.proto)
			defer p2p.SetServedProto(restore)

			a := testutil.NewTestDaemon(t, "Bench-A")
			b := testutil.NewTestDaemon(t, "Bench-B")
			a.PairWith(b)
			chunk := strings.Repeat("x", 3500) // a Project Zomboid map chunk is ~3.5KB
			for i := 0; i < n; i++ {
				a.WriteSave(fmt.Sprintf("map/map_%d_%d.bin", i/100, i%100), chunk+strconv.Itoa(i))
			}
			restoreSyncOnTrack := suppressSyncOnTrack(a, b)
			gameID := a.TrackGame("Bench")
			b.API(http.MethodPost, "/api/games", map[string]string{"name": "Bench", "savePath": b.SaveDir}, nil)
			restoreSyncOnTrack()

			start := time.Now()
			b.API(http.MethodPost, "/api/games/"+gameID+"/sync", nil, nil)
			count := func() int {
				m, _ := filepath.Glob(filepath.Join(b.SaveDir, "map", "*.bin"))
				return len(m)
			}
			if !testutil.WaitFor(60*time.Minute, func() bool { return count() >= n }) {
				t.Fatalf("only %d of %d files arrived", count(), n)
			}
			el := time.Since(start)
			t.Logf("%s: %d files in %s = %.2f ms/file", mode.name, n, el.Round(time.Millisecond), float64(el.Microseconds())/1000/float64(n))
		})
	}
}
