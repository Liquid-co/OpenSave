// Collections in the app: which games are in which, the chips that filter the
// library by them, and the calls that change them. The daemon keeps them
// (internal/store/collections.go); the list arrives with the first state and
// again after every change, from here or from the terminal.
import { writable } from 'svelte/store';
import { api } from './api.js';
import { onCollections, toast } from './stores.js';

export const FAVOURITES = 'favourites';

/** Every collection: [{id, name, builtin, gameIds}], Favourites first. */
export const collections = writable([]);
onCollections((list) => collections.set(list));

/** The collection the library is filtered to, or '' for none. A store, so
 *  the palette and the sidebar can show a collection by name. */
export const collectionFilter = writable('');

/** The collections a game is in. */
export function collectionsOf(all, gameId) {
  return (all ?? []).filter((c) => c.gameIds.includes(gameId));
}

export function isFavourite(all, gameId) {
  return !!(all ?? []).find((c) => c.id === FAVOURITES)?.gameIds.includes(gameId);
}

/**
 * The chips for the filter bar: each collection with how many of the listed
 * games are in it, leaving out the ones with none — an empty chip filters to
 * an empty library. Favourites first, as the list comes.
 */
export function collectionChips(all, rows) {
  const listed = new Set((rows ?? []).map((r) => r.game.id));
  return (all ?? [])
    .map((c) => ({ id: c.id, label: c.name, builtin: !!c.builtin, count: c.gameIds.filter((id) => listed.has(id)).length }))
    .filter((c) => c.count > 0);
}

/** Rows in a collection; every row when none is chosen. */
export function inCollection(rows, all, id) {
  if (!id) return rows;
  const members = new Set((all ?? []).find((c) => c.id === id)?.gameIds ?? []);
  return rows.filter((r) => members.has(r.game.id));
}

async function call(fn, success) {
  try {
    const out = await fn();
    if (success) toast(success, 'success');
    return out;
  } catch (e) {
    toast(e.message, 'error');
    return null;
  }
}

export const setInCollection = (collectionId, gameId, inIt) =>
  call(() => api.post(`/api/collections/${encodeURIComponent(collectionId)}/games`, { gameId, in: inIt }));

export function toggleFavourite(all, game) {
  const on = !isFavourite(all, game.id);
  return call(
    () => api.post(`/api/collections/${FAVOURITES}/games`, { gameId: game.id, in: on }),
    on ? `${game.name} added to Favourites` : `${game.name} removed from Favourites`
  );
}

export const createCollection = (name) => call(() => api.post('/api/collections', { name }));
export const renameCollection = (id, name) => call(() => api.patch(`/api/collections/${encodeURIComponent(id)}`, { name }));
export const deleteCollection = (id) => call(() => api.del(`/api/collections/${encodeURIComponent(id)}`));
