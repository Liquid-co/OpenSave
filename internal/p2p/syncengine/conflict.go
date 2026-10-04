package syncengine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/opensave/opensave/internal/delta"
	"github.com/opensave/opensave/internal/owntouch"
	"github.com/opensave/opensave/internal/snapshot"
)

// ResolveConflict applies the user's chosen resolution to an active
// conflict. Every resolution affects ONLY this device's save — it never
// reaches over and rewrites the peer's save without the peer's own user
// choosing. The peer resolves its own copy of the conflict independently.
//
//   - keep-local:   keep this device's version as-is (peer is untouched).
//   - keep-remote:  adopt the peer's current state on this device, taking a
//     safety snapshot of the local version first.
//   - merge-branch: keep this device's version, as keep-local does, and
//     keep the peer's state beside it on a new "conflict-<peer>-<id>"
//     branch, so both survive; switch between them later.
func (e *Engine) ResolveConflict(ctx context.Context, gameID, peerID, resolution string) (branchName string, err error) {
	e.mu.Lock()
	conflict := e.activeConflicts[gameID]
	e.mu.Unlock()
	if conflict == nil || conflict.Peer.ID != peerID {
		return "", fmt.Errorf("no active conflict for game %q and peer %q", gameID, peerID)
	}
	peer := conflict.Peer

	switch resolution {
	case "keep-local":
		// Keep our version as the resolved state. We record it as the agreed
		// merge-base (so this exact divergence never re-prompts) and ask the
		// peer to pull our version — its own engine applies it with a safety
		// snapshot, or re-raises the conflict if the peer has NEW unsynced
		// work of its own. Without recording the base, the next sync would
		// see both sides still differing from the stale base and re-conflict
		// immediately — the "keep looping" bug.
		e.Log("info", fmt.Sprintf("conflict on %q resolved: keep LOCAL — our version becomes the shared state", gameID))
		if err := e.markResolvedLocal(ctx, gameID, peer); err != nil {
			return "", err
		}
		// This device's version becomes newer than both, so the peer — and any
		// device holding either side's — takes it whole (version.go).
		e.versionAfterResolution(gameID, conflict.remoteVersion, true)
		e.clearConflict(gameID)
		e.Transport.TriggerPeerPull(peer, gameID)
		return "", nil

	case "keep-remote":
		e.Log("info", fmt.Sprintf("conflict on %q resolved: keep REMOTE — overwriting local", gameID))
		remoteData, err := e.peerStateForResolution(ctx, gameID, peer)
		if err != nil {
			return "", err
		}
		// From here this device's save is being replaced (settle.go).
		defer e.Writing(gameID)()
		// Non-destructive: snapshot the local version first so "keep theirs"
		// can always be undone from the Snapshots tab. (merge-branch never
		// replaces this device's save, so needs no such copy.)
		if _, err := e.Snapshots.CreateBeforeReplacing(gameID, fmt.Sprintf("This device's version (before keeping %s's)", peer.Name)); err != nil {
			e.Log("warn", fmt.Sprintf("safety snapshot before keep-remote failed: %v", err))
		}
		if err := e.overwriteLocalWithRemote(ctx, gameID, peer, remoteData, "Resolved conflict: Overwrite with remote"); err != nil {
			// This device's own version is given up either way; what is still
			// missing comes from the peer like any outdated device's would.
			e.discardLocalVersion(gameID, remoteData.Version)
			return "", err
		}
		if game, err := e.Store.GetGame(gameID); err == nil {
			if now, err := delta.BuildManifest(game.SavePath); err == nil && sameFiles(now, remoteData.Manifest) {
				e.versionAfterResolution(gameID, remoteData.Version, false)
			} else {
				e.discardLocalVersion(gameID, remoteData.Version)
			}
		}
		e.markResolvedConverged(gameID, peer)
		e.clearConflict(gameID)
		return "", nil

	case "merge-branch":
		// Keep both: this device goes on playing its own version, and the
		// peer's is kept beside it on a branch of its own, there to switch to.
		//
		// It used to do the opposite of that: switch this device onto a new
		// branch and fill it with the peer's version, leaving its own behind on
		// the old one. The peer, answering the same way, did likewise, so the
		// two ended on branches with different names; each then followed the
		// other's branch by creating it empty, and read its empty folder
		// against their old record of shared files as every file deleted —
		// which it passed on. Both saves were emptied by the recommended
		// answer. Now the save folder is never touched, and no branch changes.
		branchName = fmt.Sprintf("conflict-%s-%s",
			snapshot.CleanBranchName(peer.Name),
			lastN(fmt.Sprintf("%d", time.Now().UnixMilli()), 4))
		remoteData, err := e.peerStateForResolution(ctx, gameID, peer)
		if err != nil {
			return "", err
		}
		created, err := e.Snapshots.CreateBranch(gameID, branchName, false)
		if err != nil {
			return "", err
		}
		if err := e.keepPeersVersion(ctx, gameID, peer, remoteData, created); err != nil {
			// Not left behind empty: switching to an empty branch clears the
			// save folder.
			_, _ = e.Snapshots.DeleteBranch(gameID, created)
			return "", fmt.Errorf("keep %s's version: %w", peer.Name, err)
		}
		e.Log("info", fmt.Sprintf("conflict on %q resolved: keep BOTH — this device's version stays, and %s's is kept on branch %q",
			gameID, peer.Name, created))
		// From here it is "keep mine": this device's version becomes the
		// shared one, and the peer is asked to take it — which, if it has
		// changes of its own, asks the person there in turn.
		if err := e.markResolvedLocal(ctx, gameID, peer); err != nil {
			return "", err
		}
		e.versionAfterResolution(gameID, conflict.remoteVersion, true)
		e.clearConflict(gameID)
		e.Transport.TriggerPeerPull(peer, gameID)
		return created, nil

	default:
		return "", fmt.Errorf("invalid conflict resolution %q", resolution)
	}
}

// markResolvedConverged records the post-resolution convergence for the
// keep-remote path, where the local save was just overwritten to match the
// peer. It captures the CURRENT local manifest as the agreed
// merge-base and lineage, and tells the peer we are now in sync at that
// hash — the peer, already holding that exact state, re-confirms identity
// on its own clock (ConfirmInSync) and records the same base. Both sides
// end at one merge-base, so the resolved divergence can never re-trigger.
//
// Capturing the manifest AFTER the overwrite (rather than assuming it
// equals the remote) means even a partially-applied overwrite is safe: the
// base matches our real files, so the next sync finishes converging via a
// normal pull instead of re-raising a conflict.
func (e *Engine) markResolvedConverged(gameID string, peer Peer) {
	game, err := e.Store.GetGame(gameID)
	if err != nil {
		return
	}
	local, err := delta.BuildManifest(game.SavePath)
	if err != nil {
		e.Log("warn", fmt.Sprintf("record resolution: build manifest failed: %v", err))
		return
	}
	e.persistLineage(gameID, peer.ID, local, local)
	_ = e.Store.SetAgreedHash(gameID, peer.ID, local.ManifestHash())
	e.recordSynced(gameID, peer.ID)
	_ = e.Store.SetLastManifestHash(gameID, e.contentHashOf(gameID, local, game.SavePath))
	e.Transport.ReportSyncEvent(peer, gameID, "in-sync", map[string]any{
		"peerName":     e.deviceName(),
		"manifestHash": local.ManifestHash(),
	})
}

// markResolvedLocal records our local version as the agreed merge-base for
// the keep-local path (we did NOT overwrite anything) and makes it
// authoritative so it propagates to the peer instead of being pulled back.
//
// Subtlety: recording the merge-base alone is not enough. On the next
// sync the file still differs on both sides, so Compute falls to its
// mtime tiebreak — and if the peer's copy happens to carry a newer mtime,
// OUR version would be pulled away, silently undoing the user's "keep
// mine" choice. So we refresh our save files' mtimes to now: the content
// (and thus the merge-base hash) is unchanged, but our side is now
// unambiguously the newest, so the sync direction is a push. The peer
// then receives our version (its own engine snapshots first, or re-raises
// its own conflict if it has newer unsynced work of its own).
//
// The lineage, though, must say only what both sides verifiably hold. It used
// to record every local file as shared (persistLineage(local, local)) before
// the peer had pulled any of it. When that pull did not finish — a large save,
// a request that timed out, a device that went away — the next sync found
// those files "shared before and missing on the peer now", read that as the
// peer having deleted them, and deleted them HERE: on the device whose version
// the person had just chosen to keep. A save of a quarter-million files lost
// tens of thousands that way, and the folders differing again re-raised the
// conflict, inviting the same answer. Now the record is the intersection of
// the two saves as they stand; a file only this device has is new to the
// peer, not deleted by it, and goes over on the next sync. If the peer cannot
// be asked, nothing is recorded as shared, so nothing can be read as deleted.
func (e *Engine) markResolvedLocal(ctx context.Context, gameID string, peer Peer) error {
	game, err := e.Store.GetGame(gameID)
	if err != nil {
		return err
	}
	e.touchSaveMtimes(gameID, game.SavePath)
	local, err := delta.BuildManifest(game.SavePath)
	if err != nil {
		return err
	}
	var files, dirs []string
	if remote, err := e.peerStateForResolution(ctx, gameID, peer); err == nil {
		files, dirs = IntersectLineage(local, remote.Manifest)
	} else {
		e.Log("info", fmt.Sprintf("could not read %s's save while resolving %q (%v) — nothing is recorded as shared, so nothing it lacks is taken as deleted", peer.Name, gameID, err))
	}
	if rules := e.rulesFor(gameID); !rules.Empty() {
		files = filterPathList(files, rules)
		dirs = filterPathList(dirs, rules)
	}
	if err := e.Store.SetSyncState(gameID, peer.ID, files, dirs); err != nil {
		e.Log("warn", fmt.Sprintf("persist sync lineage failed: %v", err))
	}
	_ = e.Store.SetAgreedHash(gameID, peer.ID, local.ManifestHash())
	_ = e.Store.SetLastManifestHash(gameID, e.contentHashOf(gameID, local, game.SavePath))
	e.recordSynced(gameID, peer.ID)
	return nil
}

// touchSaveMtimes bumps every file's mtime under root to now without
// changing its content, marking this side as the most recent version.
func (e *Engine) touchSaveMtimes(gameID, root string) {
	// A save half re-stamped reads as newer in some files and not others, so
	// nothing reads it for a sync until it is done (settle.go). Taken first so
	// it is let go after the invalidation.
	defer e.Writing(gameID)()
	// Defence in depth. This moves every mtime to now, which a size+mtime
	// cache notices by itself — the entries miss and are re-read to the same
	// hashes, since the content is untouched. Kept so the rule stays "every
	// writer invalidates", and so that changing this to set any time other
	// than now cannot quietly become a correctness bug.
	defer delta.InvalidateRoot(root)

	now := time.Now()
	info, err := os.Stat(root)
	if err != nil {
		return
	}
	if !info.IsDir() {
		owntouch.Mark(root)
		_ = os.Chtimes(root, now, now)
		owntouch.Settled(root)
		return
	}
	_ = filepath.Walk(root, func(path string, fi os.FileInfo, walkErr error) error {
		if walkErr != nil || fi.IsDir() {
			return nil
		}
		owntouch.Mark(path)
		_ = os.Chtimes(path, now, now)
		owntouch.Settled(path)
		return nil
	})
}

func (e *Engine) clearConflict(gameID string) {
	e.mu.Lock()
	delete(e.activeConflicts, gameID)
	e.mu.Unlock()
	if e.Progress.OnConflict != nil {
		e.Progress.OnConflict(gameID) // state changed; let the UI refresh
	}
}

// peerStateForResolution asks the peer for the save a resolution is about to
// take. Asked before this device writes anything: if the peer is itself
// part-way through a write it makes this wait, and this device's save should
// not sit half-replaced meanwhile — nor, if both devices resolve at once,
// should each be holding its own save while waiting on the other's.
func (e *Engine) peerStateForResolution(ctx context.Context, gameID string, peer Peer) (ManifestResponse, error) {
	game, err := e.Store.GetGame(gameID)
	if err != nil {
		return ManifestResponse{}, err
	}
	remoteData, err := e.Transport.FetchManifest(ctx, peer, gameID, ManifestQuery{Name: game.Name, SavePath: game.SavePath, AppID: game.AppID, CoverURL: game.CoverURL})
	if isSettling(err) {
		return ManifestResponse{}, fmt.Errorf("%s is still finishing a sync of this game — try again in a moment", peer.Name)
	}
	if err != nil {
		return ManifestResponse{}, fmt.Errorf("fetch remote manifest: %w", err)
	}
	return remoteData, nil
}

// keepPeersVersion stores the peer's save, as remoteData describes it, as the
// first snapshot on branch. It is fetched into a folder of its own and
// archived from there, so this device's save folder is not touched.
func (e *Engine) keepPeersVersion(ctx context.Context, gameID string, peer Peer, remoteData ManifestResponse, branch string) error {
	game, err := e.Store.GetGame(gameID)
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "opensave-keep-both-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	files := remoteData.Manifest.Files
	if len(files) == 0 {
		return fmt.Errorf("%s holds no save files for this game", peer.Name)
	}
	// A save that is one file is archived as that file, under this device's
	// own name for it, which is what switching to the branch restores to.
	isFile, _ := delta.ResolveLocalSaveFilePath(game.SavePath)
	src := tmp
	if isFile {
		if len(files) != 1 {
			return fmt.Errorf("%s holds %d files for a save that is one file here", peer.Name, len(files))
		}
		src = filepath.Join(tmp, filepath.Base(game.SavePath))
	} else {
		for _, dir := range remoteData.Manifest.Dirs {
			if delta.IsSafePath(tmp, dir) {
				_ = os.MkdirAll(filepath.Join(tmp, filepath.FromSlash(dir)), 0o777)
			}
		}
	}

	var total int64
	for _, f := range files {
		total += f.Size
	}
	tracker := newProgressTracker(total)
	throttle := e.throttleFor(peer.Wan())
	for rel, file := range files {
		if !delta.IsSafePath(tmp, rel) {
			return fmt.Errorf("path traversal attempt on %s", rel)
		}
		if reason := delta.UnrepresentableName(rel); reason != "" {
			e.Log("warn", fmt.Sprintf("not keeping %q from %s: %s", rel, peer.Name, reason))
			continue
		}
		local := filepath.Join(tmp, filepath.FromSlash(rel))
		if isFile {
			local = src
		}
		if err := os.MkdirAll(filepath.Dir(local), 0o777); err != nil {
			return err
		}
		ref := FileRef{GameID: gameID, Root: delta.PrimaryRoot, RelPath: rel}
		if err := e.pullFile(ctx, peer, ref, local, file, DifferentBlockIndices(nil, file), throttle, tracker, func(bool) {}); err != nil {
			return fmt.Errorf("fetch %s: %w", rel, err)
		}
		if file.MtimeMs > 0 {
			mtime := time.UnixMilli(int64(file.MtimeMs))
			_ = os.Chtimes(local, mtime, mtime)
		}
	}
	_, err = e.Snapshots.CreateOnBranchFrom(gameID, branch, src,
		fmt.Sprintf("%s's version, kept when a conflict was resolved", peer.Name))
	return err
}

// overwriteLocalWithRemote makes the local save byte-identical to the
// peer's state as remoteData describes it: delete local-only files, pull
// every added/changed file, then mirror the peer's latest snapshot. The
// caller holds the write gate.
func (e *Engine) overwriteLocalWithRemote(ctx context.Context, gameID string, peer Peer, remoteData ManifestResponse, mirrorComment string) error {
	game, err := e.Store.GetGame(gameID)
	if err != nil {
		return err
	}
	// This replaces local content wholesale, via helpers that each invalidate
	// on their own. Repeated here so the guarantee does not depend on which
	// path through them a given conflict resolution happens to take.
	defer delta.InvalidateRoot(game.SavePath)

	localManifest, err := delta.BuildManifest(game.SavePath)
	if err != nil {
		return err
	}

	// Local-only files are deleted (remote is the source of truth here).
	for relPath := range localManifest.Files {
		if _, onRemote := remoteData.Manifest.Files[relPath]; onRemote {
			continue
		}
		if !delta.IsSafePath(game.SavePath, relPath) {
			continue
		}
		full := filepath.Join(game.SavePath, filepath.FromSlash(relPath))
		_ = os.Chmod(full, 0o666)
		owntouch.MarkRemoved(full)
		_ = os.Remove(full)
	}

	// Pull everything that's new or different.
	var filesToPull []string
	for relPath, remoteFile := range remoteData.Manifest.Files {
		if localFile, ok := localManifest.Files[relPath]; ok && localFile.Hash == remoteFile.Hash {
			continue
		}
		filesToPull = append(filesToPull, relPath)
	}

	if len(filesToPull) > 0 {
		if err := e.pullFiles(ctx, peer, gameID, game, primaryRootOf(game), localManifest, remoteData, filesToPull); err != nil {
			return err
		}
	} else if remoteData.LatestSnapshot != nil {
		// Nothing to transfer but still record the shared history entry.
		e.recordMirrorSnapshot(gameID, game, peer, *remoteData.LatestSnapshot,
			fmt.Sprintf("Synced from peer: %s (%s)", peer.Name, mirrorComment))
	}
	return nil
}

func lastN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
