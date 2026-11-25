package syncengine

import (
	"context"
	"fmt"

	"github.com/opensave/opensave/internal/ignore"
)

// A sync judges the peer's save as it was when its manifest was fetched,
// against a base and a lineage read afterwards. Those can be written in
// between by something else this device does for the same peer: the peer
// reporting it finished pulling from here records the files it took as
// shared at once (peerFinishedPulling, RefreshLineage), while a sync of this
// device's own, started earlier, is still running.
//
// The sync then reads a lineage newer than the save it compares: a file the
// peer had just taken from here, missing from the manifest fetched before it
// did, reads as one the peer deleted — and this device deletes its own copy.
// The peer never deleted anything.
//
// So before deleting anything here because the peer no longer has it, the
// peer is asked once more. If it holds any of those files now, it has moved
// since this sync read it, and nothing is decided on that reading: the sync
// stops before changing anything and is run again at once, as for a peer
// still writing ("peer_busy"), on a manifest and a lineage read in order.

// peerRegainedDeletions reports whether the peer holds now any of the files
// this sync is about to delete here because the peer did not. A peer that
// cannot be asked counts as having moved: nothing is deleted on a reading
// that could not be confirmed, and the next sync asks again.
func (e *Engine) peerRegainedDeletions(ctx context.Context, gameID string, peer Peer, q ManifestQuery, rules ignore.Rules, decision Decision) bool {
	if len(decision.FilesToDeleteLocally) == 0 {
		return false
	}
	again, err := e.Transport.FetchManifest(ctx, peer, gameID, q)
	if err != nil {
		return true
	}
	now := filterManifest(again.Manifest, rules)
	for _, p := range decision.FilesToDeleteLocally {
		if _, ok := now.Files[p]; ok {
			e.Log("info", fmt.Sprintf("%s changed while this device was comparing with it (it holds %s again) — comparing again before deleting anything here", peer.Name, p))
			return true
		}
	}
	return false
}
