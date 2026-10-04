package syncengine

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/opensave/opensave/internal/delta"
	"github.com/opensave/opensave/internal/snapshot"
)

// batchTransport is fakeTransport plus batched fetches, answering manifests
// with a chosen protocol revision.
type batchTransport struct {
	*fakeTransport
	proto int

	mu         sync.Mutex
	batchCalls int
	blockCalls int
	failPath   string // answered with a per-file error
	// maxFiles, when set, refuses bigger batches the way a responder that
	// counts differently does.
	maxFiles int
}

func (b *batchTransport) FetchManifest(ctx context.Context, peer Peer, gameID string, q ManifestQuery) (ManifestResponse, error) {
	resp, err := b.fakeTransport.FetchManifest(ctx, peer, gameID, q)
	resp.Proto = b.proto
	return resp, err
}

func (b *batchTransport) FetchBlocks(ctx context.Context, peer Peer, ref FileRef, idx []int, bs int) ([]BlockData, error) {
	b.mu.Lock()
	b.blockCalls++
	b.mu.Unlock()
	return b.fakeTransport.FetchBlocks(ctx, peer, ref, idx, bs)
}

func (b *batchTransport) FetchFileBatch(ctx context.Context, peer Peer, gameID, root string, files []FileBlocksRequest) ([]FileBlocks, error) {
	b.mu.Lock()
	b.batchCalls++
	b.mu.Unlock()
	if len(files) > batchMaxFiles {
		return nil, fmt.Errorf("batch of %d files is over the limit", len(files))
	}
	if b.maxFiles > 0 && len(files) > b.maxFiles {
		return nil, fmt.Errorf(`peer returned 400: {"error":"batch too large"}`)
	}
	out := make([]FileBlocks, 0, len(files))
	for _, f := range files {
		if f.RelPath == b.failPath {
			out = append(out, FileBlocks{RelPath: f.RelPath, Error: "read blocks failed: locked"})
			continue
		}
		blocks, err := b.fakeTransport.FetchBlocks(ctx, peer, FileRef{GameID: gameID, Root: root, RelPath: f.RelPath}, f.BlockIndices, f.BlockSize)
		if err != nil {
			out = append(out, FileBlocks{RelPath: f.RelPath, Error: err.Error()})
			continue
		}
		out = append(out, FileBlocks{RelPath: f.RelPath, Blocks: blocks})
	}
	return out, nil
}

func setupBatchEngine(t *testing.T, proto int) (*engineEnv, *batchTransport) {
	t.Helper()
	env := setupEngine(t)
	bt := &batchTransport{fakeTransport: env.transport, proto: proto}
	env.engine = New(env.store, snapshot.New(env.store), bt)
	return env, bt
}

// A save of many small files, plus one big one, in two folders.
func writeManyFiles(t *testing.T, dir string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		write(t, dir, fmt.Sprintf("map/chunk_%04d.bin", i), strings.Repeat(fmt.Sprintf("chunk %d;", i), 20))
	}
	big := bytes.Repeat([]byte("large save data "), (512<<10)/16) // 512KB: per-file path
	if err := os.WriteFile(filepath.Join(dir, "big.sav"), big, 0o666); err != nil {
		t.Fatal(err)
	}
}

func assertSameTree(t *testing.T, want, got string) {
	t.Helper()
	wm, err := delta.BuildManifest(want)
	if err != nil {
		t.Fatal(err)
	}
	gm, err := delta.BuildManifest(got)
	if err != nil {
		t.Fatal(err)
	}
	if wm.ManifestHash() != gm.ManifestHash() {
		t.Fatalf("trees differ: %d files remote, %d local", len(wm.Files), len(gm.Files))
	}
	for p, f := range wm.Files {
		if gm.Files[p].MtimeMs != f.MtimeMs {
			t.Errorf("%s: mtime %d, want the peer's %d", p, gm.Files[p].MtimeMs, f.MtimeMs)
			break
		}
	}
}

func TestPull_BatchesSmallFiles(t *testing.T) {
	env, bt := setupBatchEngine(t, ProtoBatchFiles)
	writeManyFiles(t, env.remoteDir, 600)

	res, err := env.engine.SyncWithPeer(context.Background(), "game1", env.peer)
	if err != nil {
		t.Fatalf("SyncWithPeer: %v", err)
	}
	if res.Status != "updated" || res.Direction != "pull" {
		t.Fatalf("result = %+v, want updated/pull", res)
	}
	assertSameTree(t, env.remoteDir, env.localDir)

	// 600 one-block files: each claims a 64 KB block as the responder counts,
	// so 192 fit a request (batchMaxClaimed) -> 4 requests; the big file alone
	// goes block by block.
	if bt.batchCalls != 4 {
		t.Errorf("batch requests = %d, want 4", bt.batchCalls)
	}
	if bt.blockCalls != 1 {
		t.Errorf("per-file block requests = %d, want 1 (the big file)", bt.blockCalls)
	}
	if len(env.transport.syncEvents) == 0 || env.transport.syncEvents[len(env.transport.syncEvents)-1] != "sync-complete" {
		t.Errorf("sync did not complete: %v", env.transport.syncEvents)
	}
}

// A peer that predates batching is asked file by file, as before.
func TestPull_OldPeerIsNotBatched(t *testing.T) {
	env, bt := setupBatchEngine(t, ProtoMultiRoot)
	writeManyFiles(t, env.remoteDir, 50)

	if _, err := env.engine.SyncWithPeer(context.Background(), "game1", env.peer); err != nil {
		t.Fatalf("SyncWithPeer: %v", err)
	}
	assertSameTree(t, env.remoteDir, env.localDir)
	if bt.batchCalls != 0 {
		t.Errorf("batch requests to an old peer = %d, want 0", bt.batchCalls)
	}
	if bt.blockCalls != 51 {
		t.Errorf("per-file requests = %d, want 51", bt.blockCalls)
	}
}

// One file the peer cannot read fails the pull and names the file, like the
// per-file path does; nothing half-written is left in its place.
func TestPull_BatchFileErrorNamesTheFile(t *testing.T) {
	env, bt := setupBatchEngine(t, ProtoBatchFiles)
	writeManyFiles(t, env.remoteDir, 10)
	bt.failPath = "map/chunk_0003.bin"

	_, err := env.engine.SyncWithPeer(context.Background(), "game1", env.peer)
	if err == nil || !strings.Contains(err.Error(), "map/chunk_0003.bin") {
		t.Fatalf("err = %v, want one naming map/chunk_0003.bin", err)
	}
	if _, statErr := os.Stat(filepath.Join(env.localDir, "map", "chunk_0003.bin")); statErr == nil {
		t.Error("the file the peer could not serve was written anyway")
	}
	leftovers, _ := filepath.Glob(filepath.Join(env.localDir, "map", "*"+delta.TmpSuffix))
	if len(leftovers) > 0 {
		t.Errorf("temp files left behind: %v", leftovers)
	}
}

func TestGroupBatches_RespectsLimits(t *testing.T) {
	var jobs []batchJob
	for i := 0; i < 600; i++ {
		jobs = append(jobs, batchJob{relPath: fmt.Sprint(i),
			remote:  delta.FileEntry{Blocks: []delta.Block{{Index: 0, Length: 32 << 10}}},
			indices: []int{0}})
	}
	for _, b := range groupBatches(jobs) {
		var n int64
		for _, j := range b {
			n += changedBytes(j.remote, j.indices)
		}
		if len(b) > batchMaxFiles || n > batchMaxBytes {
			t.Errorf("batch of %d files / %d bytes is over the limit", len(b), n)
		}
	}
}

// Many small files, some of several blocks: what the responder counts —
// every block at full block size — must stay under its limit too. 256 files
// of one 64 KB block already claim 16 MB, and each file of more blocks tipped
// a batch over it: "batch too large", and the whole pull failed.
func TestGroupBatches_StaysUnderWhatTheResponderCounts(t *testing.T) {
	var jobs []batchJob
	for i := 0; i < 2000; i++ {
		blocks := []delta.Block{{Index: 0, Length: 4 << 10}}
		indices := []int{0}
		if i%10 == 0 {
			blocks = []delta.Block{{Index: 0, Length: 64 << 10}, {Index: 1, Length: 64 << 10}, {Index: 2, Length: 10 << 10}}
			indices = []int{0, 1, 2}
		}
		jobs = append(jobs, batchJob{relPath: fmt.Sprint(i),
			remote: delta.FileEntry{BlockSize: 64 << 10, Blocks: blocks}, indices: indices})
	}
	for _, b := range groupBatches(jobs) {
		var claimed int64
		for _, j := range b {
			claimed += claimedBytes(j.remote, j.indices)
		}
		if claimed > 16<<20 {
			t.Errorf("a batch of %d files claims %d bytes, over the responder's limit", len(b), claimed)
		}
	}
}

// A responder that refuses a batch as too large gets it again in halves,
// rather than the pull failing.
func TestPull_ABatchRefusedAsTooLargeIsSplit(t *testing.T) {
	env, bt := setupBatchEngine(t, ProtoBatchFiles)
	bt.maxFiles = 10
	for i := 0; i < 40; i++ {
		write(t, env.remoteDir, fmt.Sprintf("chunks/c%02d.bin", i), fmt.Sprint("chunk ", i))
	}
	if _, err := env.engine.SyncWithPeer(context.Background(), "game1", env.peer); err != nil {
		t.Fatalf("sync failed though the batches could be split: %v", err)
	}
	for i := 0; i < 40; i++ {
		if _, err := os.Stat(filepath.Join(env.localDir, "chunks", fmt.Sprintf("c%02d.bin", i))); err != nil {
			t.Fatalf("c%02d.bin did not arrive", i)
		}
	}
}
