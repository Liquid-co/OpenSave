// Games taken out of the Library's Recently played row, on this device: each
// game's id and the play (ms) that was removed. A later play shows it again
// (lib/libraryhome.js recentlyPlayed), so nothing here needs clearing by hand.
import { writable } from 'svelte/store';

const KEY = 'opensave.recentHidden';
// More than the row could ever need; the oldest go first.
const MAX = 200;

/** Only well-formed entries, the newest MAX of them. */
export function sanitizeHidden(v) {
  const entries = Object.entries(v && typeof v === 'object' && !Array.isArray(v) ? v : {}).filter(
    ([id, at]) => id && typeof at === 'number' && Number.isFinite(at) && at > 0
  );
  entries.sort((a, b) => b[1] - a[1]);
  return Object.fromEntries(entries.slice(0, MAX));
}

export function loadHidden(storage = globalThis.localStorage) {
  try {
    return sanitizeHidden(JSON.parse(storage?.getItem(KEY) ?? '{}'));
  } catch {
    return {};
  }
}

export function saveHidden(v, storage = globalThis.localStorage) {
  try {
    storage?.setItem(KEY, JSON.stringify(sanitizeHidden(v)));
  } catch {}
}

/** gameId -> the play (ms) taken out of the row. */
export const recentHidden = writable(loadHidden());
recentHidden.subscribe((v) => saveHidden(v));

/** Takes one play of a game out of the row. */
export const hideRecent = (gameId, at) => recentHidden.update((v) => ({ ...v, [gameId]: at }));

/** Puts a game back, as it was before it was taken out (Undo). */
export const unhideRecent = (gameId, before) =>
  recentHidden.update((v) => {
    const next = { ...v };
    if (before) next[gameId] = before;
    else delete next[gameId];
    return next;
  });
