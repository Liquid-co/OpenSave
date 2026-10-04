package syncengine

import (
	"context"
	"fmt"

	"github.com/opensave/opensave/internal/store"
)

// quickInSync settles the common case of a sync in one small exchange: this
// device's save still hashes to the base both devices last agreed on, and
// the peer, asked whether it does too (ManifestQuery.IfHash), says
// "unchanged". Then there is nothing to decide, and neither manifest needs
// to cross the wire — for a save of a quarter-million files that is tens of
// MB of JSON per peer, which the minute-long reconcile used to fetch every
// minute to learn that nothing had changed.
//
// It only ever answers "in sync" when both sides provably hold the agreed
// state. In every other case it reports not done and the full sync runs; a
// full manifest that came back anyway (a peer that predates the question)
// is handed over in prefetched so it is not fetched twice.
//
// Deliberately narrow:
//   - no exclusion rules: the agreed base is taken over the filtered
//     manifest, the peer hashes its unfiltered one;
//   - no extra save locations: those carry bases of their own (RootHash);
//   - no conflict waiting, and the same branch on both sides.
func (e *Engine) quickInSync(ctx context.Context, gameID string, game store.Game, peer Peer, q ManifestQuery) (res Result, done bool, prefetched *ManifestResponse) {
	agreed := e.Store.GetAgreedHash(gameID, peer.ID)
	if agreed == "" || !e.rulesFor(gameID).Empty() {
		return Result{}, false, nil
	}
	if roots, err := e.Store.GameRootPaths(gameID); err != nil || len(roots) > 0 {
		return Result{}, false, nil
	}
	e.mu.Lock()
	conflicted := e.activeConflicts[gameID] != nil
	e.mu.Unlock()
	if conflicted {
		return Result{}, false, nil
	}
	local, err := e.ReadManifest(ctx, gameID, game.SavePath)
	if err != nil || len(local.Extra) > 0 || local.ManifestHash() != agreed {
		return Result{}, false, nil
	}

	q.IfHash = agreed
	resp, err := e.Transport.FetchManifest(ctx, peer, gameID, q)
	if err != nil {
		// Let the full path fetch again and classify the error as it always has.
		return Result{}, false, nil
	}
	if !resp.Unchanged {
		return Result{}, false, &resp
	}
	if resp.ManifestHash != agreed || (resp.ActiveBranch != "" && resp.ActiveBranch != game.ActiveBranch) {
		// Unchanged, but not in a way this shortcut may act on: the full
		// exchange decides.
		return Result{}, false, nil
	}
	// The same files; the versions still have to say the same (version.go).
	if resp.Version != nil && game.AutoSync {
		res, final := e.quickVersionCheck(game, peer, local, *resp.Version)
		if !final {
			return Result{}, false, nil
		}
		if res.Status == versionStatusWaiting {
			return res, true, nil
		}
	}

	// Same as the in-sync outcome of a full sync, minus what is already
	// recorded: the base is unchanged and the lineage with it.
	e.Transport.ReportSyncEvent(peer, gameID, "in-sync", map[string]any{
		"peerName":     e.deviceName(),
		"manifestHash": agreed,
	})
	if resp.LatestSnapshot != nil && len(local.Files) > 0 {
		e.recordMirrorSnapshot(gameID, game, peer, *resp.LatestSnapshot,
			fmt.Sprintf("Synced from peer: %s (%s)", peer.Name, resp.LatestSnapshot.Comment))
	}
	return Result{Status: "in_sync", Direction: "none"}, true, nil
}
