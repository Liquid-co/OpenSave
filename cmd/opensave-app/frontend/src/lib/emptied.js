// Saves emptied on this device and held back from the others until someone
// says whether that was meant (internal/p2p/syncengine/hold.go). The game
// carries `emptied` while it waits: {state: 'held' | 'fetching', files,
// locations, putBackFrom, putBackFromTime}.

/** Whether a game waits on an answer about its emptied save. */
export const waitsOnAnswer = (game) => game?.emptied?.state === 'held';

/**
 * The games that started waiting since the last look, and the ids waiting
 * now, to pass back next time. The first look reports none: a save emptied
 * before the app opened is on screen already, and is not news.
 * @param {Set<string>|null} before
 * @param {Record<string, object>} games
 */
export function newlyEmptied(before, games) {
  const now = new Set(Object.values(games ?? {}).filter(waitsOnAnswer).map((g) => g.id));
  const fresh = before === null ? [] : Object.values(games).filter((g) => now.has(g.id) && !before.has(g.id));
  return { now, fresh };
}

/** Where the files went from, in words: "its save folder", or a location. */
export function emptiedWhere(locations = []) {
  const names = locations.map((n) => (n === '' ? 'its save folder' : `its “${n}” location`));
  return names.length ? names.join(' and ') : 'its save folder';
}
