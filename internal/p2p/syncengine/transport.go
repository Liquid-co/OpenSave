package syncengine

import (
	"context"
	"errors"

	"github.com/opensave/opensave/internal/delta"
)

// Peer identifies a sync counterpart. Address "relay" (or IsWan) means the
// peer is reachable only through the WAN relay.
type Peer struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Address string `json:"address"`
	Port    int    `json:"port"`
	IsWan   bool   `json:"isWan"`
}

// Wan reports whether traffic to this peer goes through the relay.
func (p Peer) Wan() bool { return p.IsWan || p.Address == "relay" }

// SnapshotInfo is the lightweight snapshot metadata exchanged in manifest
// responses.
type SnapshotInfo struct {
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	Comment   string `json:"comment"`
}

// ProtoMultiRoot is the protocol revision at which a peer understands games
// with more than one save location.
const ProtoMultiRoot = 1

// ProtoBatchFiles is the revision at which a peer serves the blocks of many
// files in one request (FetchFileBatch). Before it, every file cost its own
// round trip — ~200ms each in practice, which for a save of a quarter-million
// small files (Project Zomboid map chunks) meant half a day for 50MB.
const ProtoBatchFiles = 2

// ProtoVersions is the revision at which a peer keeps save versions
// (version.go). A peer below it decides by the old file comparison, which
// reads whatever its copy lacks as deleted; nothing is taken from such a peer
// or deleted on its word (OldBuildRefused).
const ProtoVersions = 3

// FileBlocksRequest asks for some blocks of one file, as one entry of a batch.
type FileBlocksRequest struct {
	RelPath      string `json:"relPath"`
	BlockIndices []int  `json:"blockIndices"`
	BlockSize    int    `json:"blockSize"`
}

// FileBlocks is one file's answer inside a batch. Error is set, and Blocks
// empty, when the responder could not read that file.
type FileBlocks struct {
	RelPath string      `json:"relPath"`
	Blocks  []BlockData `json:"blocks"`
	Error   string      `json:"error,omitempty"`
}

// BatchFetcher is implemented by transports that can fetch the blocks of
// many files, all in one save location, in a single request. Optional: the
// engine checks for it, and for a peer that advertised ProtoBatchFiles.
type BatchFetcher interface {
	FetchFileBatch(ctx context.Context, peer Peer, gameID, root string, files []FileBlocksRequest) ([]FileBlocks, error)
}

// ErrBatchUnsupported is what a transport returns when it cannot batch for
// this peer (the relay path); the engine then pulls file by file.
var ErrBatchUnsupported = errors.New("batched file fetch is not supported for this peer")

// ManifestResponse is what a peer returns for a manifest request.
type ManifestResponse struct {
	Manifest       delta.Manifest `json:"manifest"`
	ActiveBranch   string         `json:"activeBranch"`
	LatestSnapshot *SnapshotInfo  `json:"latestSnapshot"`

	// Proto is the responder's sync protocol revision, absent (zero) on every
	// build that predates it.
	//
	// This is the capability gate, and it deliberately is not a version
	// string. A version tells you what a peer is; this arrives with the
	// answer itself and tells you what it understood. That matters here
	// because the block and delete requests carry a root name in a field an
	// older peer simply ignores — it would then serve the file from the
	// PRIMARY location instead, and this side would write another location's
	// contents over the save folder. Asking only peers that answered with a
	// proto is what stops that.
	Proto int `json:"proto,omitempty"`

	// DeletionConfirmed says a save location that is empty on the responder
	// was emptied on purpose: someone there answered "delete them on the
	// other devices too", or the emptying came from a device where someone
	// did (see hold.go). Without it an empty location is not taken as every
	// file in it deleted — see emptiedUnconfirmed.
	DeletionConfirmed bool `json:"deletionConfirmed,omitempty"`

	// Unchanged answers a request that carried ManifestQuery.IfHash: the
	// responder's save still hashes to exactly that, so Manifest is left
	// empty. Only ever set when asked; a peer that predates it ignores the
	// question and sends the full manifest, which is handled as before.
	Unchanged    bool   `json:"unchanged,omitempty"`
	ManifestHash string `json:"manifestHash,omitempty"`

	// Version is the responder's version of the save (version.go), absent
	// from builds that predate it and for games it does not version.
	Version *VersionInfo `json:"version,omitempty"`
	// Versions says the responder keeps save versions at all. The relay
	// path sends it instead of a Proto, which there would also claim things
	// that path does not serve.
	Versions bool `json:"versions,omitempty"`
}

// KeepsVersions reports whether the responder keeps save versions.
func (r ManifestResponse) KeepsVersions() bool {
	return r.Versions || r.Proto >= ProtoVersions
}

// FileRef identifies one file inside one of a game's save locations.
//
// Root and RelPath are passed together rather than as adjacent string
// arguments on purpose: this is the code that decides where bytes land on
// disk, and two strings side by side in a parameter list is one transposition
// away from writing a config file into a save folder.
type FileRef struct {
	GameID string
	// Root is the save location's name; empty means the primary one.
	Root    string
	RelPath string
}

// ManifestQuery carries the game metadata that lets the remote side
// auto-track a game it doesn't know yet (same as the JS query params).
type ManifestQuery struct {
	Name     string
	SavePath string
	IsFile   bool
	// AppID and CoverURL let a peer that auto-tracks this game show the same
	// cover art, instead of a blank tile.
	AppID    string
	CoverURL string
	// IfHash asks the peer to answer "unchanged" instead of its manifest when
	// its save still hashes to this — the base both devices last agreed on.
	IfHash string
}

// BlockData is one fetched block.
type BlockData struct {
	Index int    `json:"index"`
	Data  []byte `json:"data"` // JSON-marshals to base64, matching the JS wire format
	// Length is always the block's real, uncompressed size — progress
	// accounting and the patch writer both depend on that.
	Length int `json:"length"`
	// Enc names the encoding applied to Data, empty meaning raw bytes. Only
	// ever set when the requester advertised support for it, so a peer that
	// predates this field never receives one. Transports decode before the
	// engine sees the block.
	Enc string `json:"enc,omitempty"`
}

// Transport moves sync protocol messages to a peer. The LAN implementation
// speaks HTTP to the peer's /api/p2p/* routes; the WAN implementation
// tunnels the same requests through the relay WebSocket.
type Transport interface {
	FetchManifest(ctx context.Context, peer Peer, gameID string, q ManifestQuery) (ManifestResponse, error)
	FetchBlocks(ctx context.Context, peer Peer, ref FileRef, blockIndices []int, blockSize int) ([]BlockData, error)
	DeleteRemote(ctx context.Context, peer Peer, ref FileRef) error
	TriggerPeerPull(peer Peer, gameID string)
	// ReportSyncEvent is fire-and-forget progress reporting to the peer.
	ReportSyncEvent(peer Peer, gameID, eventType string, data map[string]any)
}

// ProgressEvent feeds both the local dashboard and the remote peer's UI.
type ProgressEvent struct {
	PeerName         string  `json:"peerName"`
	Direction        string  `json:"direction,omitempty"`
	BytesTransferred int64   `json:"bytesTransferred,omitempty"`
	TotalBytes       int64   `json:"totalBytes,omitempty"`
	SpeedBytesPerSec float64 `json:"speedBytesPerSec,omitempty"`
	Percentage       int     `json:"percentage,omitempty"`
	Error            string  `json:"error,omitempty"`
}

// ProgressCallbacks are optional hooks into the dashboard WS hub.
type ProgressCallbacks struct {
	OnSyncStart    func(gameID string, ev ProgressEvent)
	OnSyncProgress func(gameID string, ev ProgressEvent)
	OnSyncComplete func(gameID string, ev ProgressEvent)
	OnSyncError    func(gameID string, ev ProgressEvent)
	OnConflict     func(gameID string)
	// OnSyncConfirmed fires when a game's sync state changed without a sync
	// running here: a peer reported that it finished, and this side verified
	// it. The dashboard shows a last-synced time per game, and this is the
	// one path that moves it with no OnSyncComplete to carry the news.
	OnSyncConfirmed func(gameID string)
}
