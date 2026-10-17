package syncengine

import "github.com/opensave/opensave/internal/store"

// A choice made on this device that a peer has not acted on yet, and that
// this device's own next sync must not undo (GitHub #81, #85). Since #42 a
// side equal to the agreed base counts as unchanged, so the other side's
// state, which simply hasn't caught up, read as a change to take:
//
//   - "Keep mine" records this device's save as agreed and asks the peer to
//     take it. The peer still held the version declined, and this device's
//     next sync took that version back.
//   - A branch switch made by hand left the peer on the old branch, and this
//     device's next sync followed the peer back to it.
//
// So the choice is noted, per game and peer, kept across restarts
// (store.PeerNote), and while the peer still holds exactly what the choice
// was about, this device asks the peer to act on it instead of taking the
// peer's state. A note never outlives its reason: it is cleared as soon as the
// peer holds anything else (it took this device's save or branch, or moved on
// with a change of its own, which is then taken as usual), on untrack and
// unpair (store.ForgetGameSyncState, store.UnpairPeer), and on a further
// switch made by hand (NoteBranchSwitch).

// stillDeclined reports whether the peer still holds exactly the save "Keep
// mine" declined, and clears the note once it doesn't.
func (e *Engine) stillDeclined(gameID string, peer Peer, localHash, remoteHash string) bool {
	declined := e.Store.PeerNote(gameID, peer.ID, store.NoteDeclined)
	if declined == "" {
		return false
	}
	if remoteHash != declined || remoteHash == localHash {
		e.Store.ClearPeerNote(gameID, peer.ID, store.NoteDeclined)
		return false
	}
	return true
}

// notFollowedYet reports whether the peer is still on the branch this device
// was switched away from by hand, and clears the note once it isn't: on this
// device's branch it has followed, and on any other it has moved on, and is
// followed as before.
func (e *Engine) notFollowedYet(gameID string, peer Peer, peerBranch string) bool {
	left := e.Store.PeerNote(gameID, peer.ID, store.NoteLeftBranch)
	if left == "" {
		return false
	}
	if peerBranch != left {
		e.Store.ClearPeerNote(gameID, peer.ID, store.NoteLeftBranch)
		return false
	}
	return true
}

// NoteBranchSwitch records that a game was switched by hand away from branch
// from: each peer still there is asked to follow, not followed back. It
// replaces every note the game had, a "Keep mine" included: that was about a
// save on the branch left behind.
func (e *Engine) NoteBranchSwitch(gameID, from string) {
	e.Store.ClearGameNotes(gameID, store.NoteDeclined)
	e.Store.ClearGameNotes(gameID, store.NoteLeftBranch)
	if from == "" {
		return
	}
	peers, err := e.Store.ListPeers()
	if err != nil {
		return
	}
	for _, p := range peers {
		_ = e.Store.SetPeerNote(gameID, p.ID, store.NoteLeftBranch, from)
	}
}
