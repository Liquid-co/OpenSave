package syncengine

import (
	"fmt"
	"strings"
)

// A game being played.
//
// A game autosaves every few minutes, and each autosave is a change of its
// save. Synced as they come, every one of them reached every other device as
// a version of its own, and each of those took a snapshot of it: eleven of
// one game in 26 minutes on a device nobody was sitting at. And this device
// took other devices' versions of a game running here, writing a save under
// a game that was still using it.
//
// So while a game is played here (Playing, from the daemon's sessions), it
// is not synced either way. Another device asking for it is told so, and
// asks again later; nothing is taken for it. When the session ends, the save
// as it was left goes to the other devices in one sync (daemon/sessions.go).

// PlayingMessage is what a device answers when asked for a game being played
// on it. It contains SettlingMessage, so a device on a build that predates
// this takes it as "busy, ask again" too.
const PlayingMessage = "This device is playing this game right now; its save follows when the session ends. " + SettlingMessage

const playingMark = "is playing this game right now"

// isPlaying reports whether an error from a peer is its "playing" answer.
func isPlaying(err error) bool {
	return err != nil && strings.Contains(err.Error(), playingMark)
}

// playingErr is SyncGame declining to sync a game being played here. It
// counts as ErrHeld for callers that only want to know it was not an error.
type playingErr struct{}

func (playingErr) Error() string {
	return "it is being played on this device; it syncs when the session ends"
}

func (playingErr) Is(target error) bool { return target == ErrHeld || target == ErrPlayingHere }

// ErrPlayingHere is SyncGame declining to sync a game being played here.
var ErrPlayingHere error = playingErr{}

// keepRemoteWhilePlaying is a conflict resolution declining to replace the
// save of a game being played here. Keeping the other device's version writes
// it over this one, which is the very thing a sync waits for the session to
// end to do; a choice made in the conflict window is no different. It is not
// ErrHeld: the person asked for it, and is told why it did not happen. The
// conflict stays, to be resolved once the game is closed.
type keepRemoteWhilePlaying struct{}

func (keepRemoteWhilePlaying) Error() string {
	return "this game is being played on this device — close it, then keep the other device's version"
}

func (keepRemoteWhilePlaying) Is(target error) bool { return target == ErrPlayingHere }

// PlayingHere reports whether a game is being played on this device.
func (e *Engine) PlayingHere(gameID string) bool {
	return e.Playing != nil && e.Playing(gameID)
}

// notePeerPlaying says once per session that a peer is playing a game: the
// reconcile asks every minute, and each "playing" answer is the same news.
func (e *Engine) notePeerPlaying(gameID string, peer Peer, gameName string) {
	key := gameID + "\x00" + peer.ID
	e.playingMu.Lock()
	said := e.playingSaid[key]
	if e.playingSaid == nil {
		e.playingSaid = map[string]bool{}
	}
	e.playingSaid[key] = true
	e.playingMu.Unlock()
	if !said {
		e.Log("info", fmt.Sprintf("%s is playing %q right now; its save follows when the session ends", peer.Name, gameName))
	}
}

// peerNotPlaying forgets that a peer was playing a game, once it answers
// normally again: its next session is said again.
func (e *Engine) peerNotPlaying(gameID, peerID string) {
	e.playingMu.Lock()
	delete(e.playingSaid, gameID+"\x00"+peerID)
	e.playingMu.Unlock()
}
