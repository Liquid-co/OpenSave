// Package syncengine implements the P2P sync state machine ported from
// src/daemon/p2p/sync-engine.js: lineage-based direction/conflict
// classification (never wall-clock alone), block-level concurrent pulls
// with bandwidth throttling, deletion propagation, and the three conflict
// resolutions (keep-local / keep-remote / merge-branch).
//
// This file holds the PURE decision logic — no I/O — so the exact
// classification semantics are locked down by table-driven tests.
package syncengine

import (
	"github.com/opensave/opensave/internal/delta"
)

// clockSkewToleranceMs matches the JS engine's 2-second allowance when
// comparing manifest mtimes against the last sync time.
const clockSkewToleranceMs = 2000

// DetectConflict reports whether both sides changed since the last sync —
// the only situation that needs user intervention.
//
// Rules (ported exactly):
//   - Identical manifests are never a conflict.
//   - Never synced before (lastSyncTimeMs == 0): conflict only if BOTH
//     sides already have files (two pre-existing, diverged save states).
//   - Otherwise: conflict when both manifests' latest mtimes are newer
//     than lastSyncTimeMs + 2s of skew tolerance.
func DetectConflict(local, remote delta.Manifest, lastSyncTimeMs int64, agreedHash string) bool {
	if local.ManifestHash() == remote.ManifestHash() {
		return false
	}
	// The manifest hash covers directories as well as files, so a folder
	// present on one side only makes the hashes differ. That is not a
	// conflict: no save content disagrees, and Compute already resolves it
	// by creating the folder on the other side. Raising one here stops the
	// sync dead over a difference the user cannot act on — the modal lists
	// what differs from the *file* diff, which in this case is empty, so it
	// asks them to choose between two sides it shows as identical.
	if sameFiles(local, remote) {
		return false
	}
	// Content-based detection when a convergence point is known (the
	// manifest hash both sides verifiably held — a merge-base): conflict
	// only when BOTH sides changed relative to it. No clocks involved, so
	// no skew tolerance and no blind window right after a sync.
	if agreedHash != "" {
		return local.ManifestHash() != agreedHash && remote.ManifestHash() != agreedHash
	}
	// Legacy fallback (no convergence recorded yet): mtimes vs last sync.
	if lastSyncTimeMs == 0 {
		return len(local.Files) > 0 && len(remote.Files) > 0
	}
	localModified := int64(local.LatestMtime) > lastSyncTimeMs+clockSkewToleranceMs
	remoteModified := int64(remote.LatestMtime) > lastSyncTimeMs+clockSkewToleranceMs
	return localModified && remoteModified
}

// sameFiles reports whether both manifests hold exactly the same paths with
// exactly the same content hashes — i.e. every difference between them is a
// directory one. Deliberately ignores mtimes: the same bytes written at
// different times are the same save.
func sameFiles(local, remote delta.Manifest) bool {
	if len(local.Files) != len(remote.Files) {
		return false
	}
	for p, lf := range local.Files {
		rf, ok := remote.Files[p]
		if !ok || lf.Hash != rf.Hash {
			return false
		}
	}
	return true
}

// Decision is the complete plan for one game/peer sync run.
type Decision struct {
	FilesToPull          []string // exists (or newer) on remote -> download
	FilesToPush          []string // exists (or newer) locally -> trigger peer pull
	FilesToDeleteOnPeer  []string // we deleted since last sync -> propagate
	FilesToDeleteLocally []string // peer deleted since last sync -> apply
	DirsToPull           []string
	DirsToPush           []string
	DirsToDeleteOnPeer   []string
	DirsToDeleteLocally  []string
}

// HasChanges reports whether anything at all needs to happen.
func (d Decision) HasChanges() bool {
	return len(d.FilesToPull) > 0 || len(d.FilesToPush) > 0 ||
		len(d.FilesToDeleteOnPeer) > 0 || len(d.FilesToDeleteLocally) > 0 ||
		len(d.DirsToPull) > 0 || len(d.DirsToPush) > 0 ||
		len(d.DirsToDeleteOnPeer) > 0 || len(d.DirsToDeleteLocally) > 0
}

// HasPull / HasPush / HasDeletions mirror the JS result classification.
func (d Decision) HasPull() bool { return len(d.FilesToPull) > 0 || len(d.DirsToPull) > 0 }
func (d Decision) HasPush() bool { return len(d.FilesToPush) > 0 || len(d.DirsToPush) > 0 }
func (d Decision) HasDeletions() bool {
	return len(d.FilesToDeleteOnPeer) > 0 || len(d.FilesToDeleteLocally) > 0 ||
		len(d.DirsToDeleteOnPeer) > 0 || len(d.DirsToDeleteLocally) > 0
}

// Compute classifies every file and directory across both manifests using
// the per-peer lineage sets (paths present at the last successful sync).
// The lineage is what distinguishes "new on remote" (pull it) from
// "deleted locally" (propagate the delete) — pure set logic, no clocks.
// Only the modified-both-sides case falls back to mtime comparison, with
// remote winning ties.
func Compute(local, remote delta.Manifest, lastSyncedFiles, lastSyncedDirs map[string]struct{}) Decision {
	return ComputeWithBase(local, remote, lastSyncedFiles, lastSyncedDirs, "")
}

// ComputeWithBase is Compute told which manifest hash both sides last agreed
// on, so an mtime tie can be broken by content rather than by direction. See
// the tie case below for why that matters.
func ComputeWithBase(local, remote delta.Manifest, lastSyncedFiles, lastSyncedDirs map[string]struct{}, baseHash string) Decision {
	return ComputeWithDeletions(local, remote, lastSyncedFiles, lastSyncedDirs, baseHash, nil)
}

// ComputeWithDeletions is ComputeWithBase told which files this device
// recorded deleting, keyed by relative path.
//
// It exists because the lineage cannot be trusted to still hold the evidence.
// The lineage is rebuilt from the intersection of the two devices' current
// manifests, so a rebuild landing after a local deletion removes the very path
// that proved the file was once shared. The deletion then reads as "the peer
// has something we lack", the file is pulled back, and a save the user deleted
// reappears on the machine they deleted it from.
//
// A recorded deletion does not depend on that. It is only ever acted on when
// the peer's copy hashes to exactly what was deleted — if they changed it
// since, their bytes are newer and are taken instead. Content is the substitute
// for the version vectors Syncthing uses to tell a later change from a
// concurrent one; for this single decision it is stricter, because it cannot
// remove content that differs from what was deleted.
//
// A nil map is the old behaviour exactly.
func ComputeWithDeletions(local, remote delta.Manifest, lastSyncedFiles, lastSyncedDirs map[string]struct{}, baseHash string, deleted map[string]DeletedRecord) Decision {
	var d Decision
	localHash, remoteHash := local.ManifestHash(), remote.ManifestHash()

	allFiles := map[string]struct{}{}
	for p := range local.Files {
		allFiles[p] = struct{}{}
	}
	for p := range remote.Files {
		allFiles[p] = struct{}{}
	}

	for relPath := range allFiles {
		localFile, hasLocal := local.Files[relPath]
		remoteFile, hasRemote := remote.Files[relPath]

		switch {
		case hasRemote && !hasLocal:
			if _, synced := lastSyncedFiles[relPath]; synced {
				d.FilesToDeleteOnPeer = append(d.FilesToDeleteOnPeer, relPath)
			} else if rec, recorded := deleted[relPath]; recorded && rec.Hash == remoteFile.Hash {
				// Not in the lineage, but this device wrote down deleting it,
				// and the peer still holds byte-for-byte what was deleted. That
				// is a deletion to propagate, not a file to take back.
				//
				// The hash comparison is the whole safety argument: if the peer
				// had edited the file, its hash would differ and this falls
				// through to a pull, so a recorded deletion can never destroy
				// content that is not exactly what was deleted.
				d.FilesToDeleteOnPeer = append(d.FilesToDeleteOnPeer, relPath)
			} else {
				// Includes the case where a deletion IS recorded but the peer's
				// copy differs: they changed it after this device last saw it,
				// so their version wins and is pulled. Same outcome Syncthing
				// reaches by version vector, arrived at by content.
				d.FilesToPull = append(d.FilesToPull, relPath)
			}

		case hasLocal && !hasRemote:
			if _, synced := lastSyncedFiles[relPath]; synced {
				d.FilesToDeleteLocally = append(d.FilesToDeleteLocally, relPath)
			} else {
				d.FilesToPush = append(d.FilesToPush, relPath)
			}

		case hasLocal && hasRemote && localFile.Hash != remoteFile.Hash:
			switch {
			case remoteFile.MtimeMs > localFile.MtimeMs:
				d.FilesToPull = append(d.FilesToPull, relPath)
			case localFile.MtimeMs > remoteFile.MtimeMs:
				d.FilesToPush = append(d.FilesToPush, relPath)
			default:
				// Equal stamps, different content. Whichever side still
				// matches the base both sides agreed on is the one that has
				// not moved, so the other side holds the edit and wins.
				//
				// Compute only runs when DetectConflict said no, which with a
				// base recorded means at most one side has moved — so this
				// cannot pick the wrong one when it applies at all.
				//
				// Without it the tie went to the remote unconditionally, and a
				// local edit was replaced by the peer's older content with
				// nothing said. Equal stamps are not exotic: they are what a
				// filesystem with coarse timestamps gives you, and an SD card
				// in a Steam Deck is exFAT.
				switch {
				case baseHash != "" && remoteHash == baseHash:
					d.FilesToPush = append(d.FilesToPush, relPath)
				case baseHash != "" && localHash == baseHash:
					d.FilesToPull = append(d.FilesToPull, relPath)
				default:
					d.FilesToPull = append(d.FilesToPull, relPath)
				}
			}
		}
	}

	localDirSet := toSet(local.Dirs)
	remoteDirSet := toSet(remote.Dirs)
	allDirs := map[string]struct{}{}
	for p := range localDirSet {
		allDirs[p] = struct{}{}
	}
	for p := range remoteDirSet {
		allDirs[p] = struct{}{}
	}

	for relDir := range allDirs {
		_, hasLocal := localDirSet[relDir]
		_, hasRemote := remoteDirSet[relDir]

		switch {
		case hasRemote && !hasLocal:
			if _, synced := lastSyncedDirs[relDir]; synced {
				d.DirsToDeleteOnPeer = append(d.DirsToDeleteOnPeer, relDir)
			} else {
				d.DirsToPull = append(d.DirsToPull, relDir)
			}
		case hasLocal && !hasRemote:
			if _, synced := lastSyncedDirs[relDir]; synced {
				d.DirsToDeleteLocally = append(d.DirsToDeleteLocally, relDir)
			} else {
				d.DirsToPush = append(d.DirsToPush, relDir)
			}
		}
	}

	return d
}

// DifferentBlockIndices returns which block indices of a remote file need
// fetching, given the local counterpart (if any): everything when the file
// is new locally or block sizes differ; otherwise indices whose hashes
// differ (including length mismatches in either direction).
func DifferentBlockIndices(localFile *delta.FileEntry, remoteFile delta.FileEntry) []int {
	if localFile == nil || localFile.BlockSize != remoteFile.BlockSize {
		indices := make([]int, len(remoteFile.Blocks))
		for i := range indices {
			indices[i] = i
		}
		return indices
	}

	maxBlocks := len(remoteFile.Blocks)
	if len(localFile.Blocks) > maxBlocks {
		maxBlocks = len(localFile.Blocks)
	}
	var indices []int
	for i := 0; i < maxBlocks; i++ {
		if i >= len(remoteFile.Blocks) {
			break // local-only trailing blocks: nothing to fetch, patch truncates
		}
		if i >= len(localFile.Blocks) || localFile.Blocks[i].Hash != remoteFile.Blocks[i].Hash {
			indices = append(indices, i)
		}
	}
	return indices
}

// BatchIndices splits block indices into fetch batches.
//
// Two constraints set the size. A batch becomes one WebSocket message, so it
// must fit the 16 MB frame limit after base64 inflates it by a third. More
// tightly, `concurrency` of these can be in flight to a single client at once,
// and the relay bounds what it will buffer per client — see
// maxQueuedBytesPerClient in relay/server.go, which has to stay above
// ConcurrencyFor(true) x this x 4/3, or the relay sheds a response and the
// requester pays a full retry for it.
//
// 2 MB is the balance: large enough that a 2 MB-block file doesn't spend a
// round trip per block the way the old 1.5 MB target did, small enough that
// eight can be outstanding without the relay having to hold 40 MB for one
// peer. Throughput comes from the number in flight, not from any one being
// huge, and smaller messages also mean a retry re-sends less.
func BatchIndices(indices []int, blockSize int, isWan bool) [][]int {
	if blockSize <= 0 {
		blockSize = 64 * 1024
	}
	// Relay batches are smaller because they are sealed, and sealing costs a
	// second base64: the block bytes are already base64 inside the request's
	// JSON, and encrypting that JSON produces bytes which JSON encodes as
	// base64 again. 2 MB of blocks would leave ~3.6 MB on the wire instead of
	// ~2.7 MB, and with eight batches outstanding that takes a peer from about
	// 22 MB in flight to about 29 MB — against a relay budget of 32 MB per
	// client, where overflow is dropped messages and a failed sync.
	//
	// 1.5 MB restores the wire size the relay's limits were chosen for, so
	// sealing needs no relay to be upgraded. LAN is unsealed and unchanged.
	targetBatchBytes := 2 << 20 // ~2.7 MB once base64-encoded
	if isWan {
		targetBatchBytes = 3 << 19 // 1.5 MB; ~2.7 MB once sealed and encoded
	}
	calculated := targetBatchBytes / blockSize
	if calculated < 1 {
		calculated = 1
	}
	cap := 32
	if isWan {
		cap = 16
	}
	batchSize := calculated
	if batchSize > cap {
		batchSize = cap
	}

	var batches [][]int
	for i := 0; i < len(indices); i += batchSize {
		end := i + batchSize
		if end > len(indices) {
			end = len(indices)
		}
		batches = append(batches, indices[i:end])
	}
	return batches
}

// ConcurrencyFor returns how many batches to fetch at once.
//
// WAN transfers are latency-bound, not bandwidth-bound: each batch is a full
// round trip out to the relay, across to the peer and back, so throughput is
// roughly (bytes in flight) / RTT. Raising the number of outstanding requests
// is what makes a large file move at a sensible rate over the internet;
// raising the batch size alone doesn't, because the request still can't
// overlap the next one.
func ConcurrencyFor(isWan bool) int {
	if isWan {
		return 8
	}
	return 8
}

func toSet(items []string) map[string]struct{} {
	set := make(map[string]struct{}, len(items))
	for _, item := range items {
		set[item] = struct{}{}
	}
	return set
}

// DeletedRecord is what the decision needs to know about a deletion this
// device made: which content was removed, so a peer holding something else can
// be recognised as having edited it rather than merely lagging behind.
type DeletedRecord struct {
	Hash        string
	DeletedAtMs int64
}
