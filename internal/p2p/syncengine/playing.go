package syncengine

import "strings"

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

// PlayingHere reports whether a game is being played on this device.
func (e *Engine) PlayingHere(gameID string) bool {
	return e.Playing != nil && e.Playing(gameID)
}
