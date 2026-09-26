package p2p

import (
	"fmt"

	"github.com/opensave/opensave/internal/store"
	"github.com/opensave/opensave/internal/switchtitle"
)

// A Switch game is the same game on every device whatever id each tracks it
// under. The ids differ as a matter of course: a game tracked before version
// 2.4 has an id made from the name the scan gave it, which named the emulator
// ("citron-switch-emulator-title-id-0100…"), so the same game in Eden on a
// Steam Deck and in Citron on a PC never met; and one tracked since has
// "switch-0100…". Every one of them carries the title id — in the id, or in
// the save folder's path — and the title id is what identifies the game.
//
// Unlike App-ID matching this needs no setting. App IDs are off by default
// because a cracked and a bought copy of a PC game can keep incompatible
// saves; a Switch game's save is the game's own save data, the same files in
// every emulator of the yuzu family.

// matchSwitchTitle resolves a peer's id for a Switch game to the one game
// tracked here with the same title id, and records the match as a link so
// everything the peer sends under that id resolves the same way. peerSavePath
// is the peer's save path, when the request carries one.
func (e *Engine) matchSwitchTitle(peerGameID, peerSavePath string) (store.Game, bool) {
	titleID := switchtitle.FromGameID(peerGameID)
	if titleID == "" {
		titleID = switchtitle.FromSavePath(peerSavePath)
	}
	if titleID == "" {
		return store.Game{}, false
	}
	games, err := e.Store.ListGames()
	if err != nil {
		return store.Game{}, false
	}
	var matches []store.Game
	for _, g := range games {
		if switchtitle.Of(g.ID, g.SavePath) == titleID {
			matches = append(matches, g)
		}
	}
	switch {
	case len(matches) == 0:
		return store.Game{}, false
	case len(matches) > 1:
		// The same game tracked here more than once, in two emulators say.
		// Which of them the peer's save belongs in is the person's call.
		e.Log("warn", fmt.Sprintf(
			"the Switch game %s is tracked here %d times — link the one to sync from its Manage tab",
			titleID, len(matches)))
		return store.Game{}, false
	}
	game := matches[0]
	if err := e.Store.AddGameAlias(peerGameID, game.ID); err != nil {
		e.Log("warn", fmt.Sprintf("could not record the Switch title match %s -> %s: %v", peerGameID, game.ID, err))
	} else {
		e.Log("info", fmt.Sprintf("matched peer's %q to local %q by Switch title id %s", peerGameID, game.ID, titleID))
	}
	return game, true
}
