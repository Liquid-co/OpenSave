import { describe, expect, it } from 'vitest';
import { get } from 'svelte/store';
import { collectionChips, collections, collectionsOf, inCollection, isFavourite } from './collections.js';
import { applyMessage } from './stores.js';

const all = [
  { id: 'favourites', name: 'Favourites', builtin: true, gameIds: ['hades'] },
  { id: 'col_1', name: 'Roguelikes', gameIds: ['hades', 'balatro', 'gone'] },
  { id: 'col_2', name: 'Empty', gameIds: [] }
];
const row = (id) => ({ game: { id, name: id } });
const rows = [row('hades'), row('balatro'), row('celeste')];

describe('collections', () => {
  it('knows which a game is in, and whether it is a favourite', () => {
    expect(collectionsOf(all, 'hades').map((c) => c.id)).toEqual(['favourites', 'col_1']);
    expect(isFavourite(all, 'hades')).toBe(true);
    expect(isFavourite(all, 'balatro')).toBe(false);
    expect(isFavourite([], 'hades')).toBe(false);
  });

  it('offers a chip per collection with listed games, counting only those', () => {
    expect(collectionChips(all, rows)).toEqual([
      { id: 'favourites', label: 'Favourites', builtin: true, count: 1 },
      { id: 'col_1', label: 'Roguelikes', builtin: false, count: 2 }
    ]);
    // A search that leaves nothing in a collection drops its chip.
    expect(collectionChips(all, [row('celeste')])).toEqual([]);
  });

  it('filters the library to a collection, or not at all', () => {
    expect(inCollection(rows, all, 'col_1').map((r) => r.game.id)).toEqual(['hades', 'balatro']);
    expect(inCollection(rows, all, '')).toBe(rows);
    expect(inCollection(rows, all, 'no-such')).toEqual([]);
  });

  it('follows the daemon: the first state, then every change', () => {
    applyMessage({ type: 'init', data: { games: {}, collections: all } });
    expect(get(collections)).toHaveLength(3);
    applyMessage({ type: 'collections-update', data: [all[0]] });
    expect(get(collections).map((c) => c.id)).toEqual(['favourites']);
  });
});
