import { describe, expect, it } from 'vitest';
import { emptiedWhere, newlyEmptied, waitsOnAnswer } from './emptied.js';
import { gameStatus, librarySummary } from './gamestatus.js';

const game = (id, emptied = null) => ({ id, name: id.toUpperCase(), emptied, branches: {} });

describe('emptied saves', () => {
  it('waits on an answer only while held, not while the files come back', () => {
    expect(waitsOnAnswer(game('a', { state: 'held' }))).toBe(true);
    expect(waitsOnAnswer(game('a', { state: 'fetching' }))).toBe(false);
    expect(waitsOnAnswer(game('a'))).toBe(false);
  });

  it('reports a newly emptied save once, and none on the first look', () => {
    const first = newlyEmptied(null, { a: game('a', { state: 'held' }) });
    expect(first.fresh).toEqual([]);
    const second = newlyEmptied(first.now, { a: game('a', { state: 'held' }), b: game('b', { state: 'held' }) });
    expect(second.fresh.map((g) => g.id)).toEqual(['b']);
    const third = newlyEmptied(second.now, { a: game('a', { state: 'held' }), b: game('b', { state: 'held' }) });
    expect(third.fresh).toEqual([]);
  });

  it('names where the files went from', () => {
    expect(emptiedWhere([''])).toBe('its save folder');
    expect(emptiedWhere(['', 'Config'])).toBe('its save folder and its “Config” location');
  });

  it('comes first on the tile, and in the line above the library', () => {
    const held = game('a', { state: 'held' });
    expect(gameStatus(held)).toMatchObject({ state: 'emptied', tone: 'warn' });
    expect(gameStatus({ ...held, emptied: { state: 'fetching' } })).toMatchObject({ state: 'restoring', tone: 'busy' });
    const rows = [
      { game: held, status: gameStatus(held) },
      { game: game('b'), status: { state: 'conflict' } }
    ];
    expect(librarySummary(rows).headline).toBe("A's save files were all deleted here");
  });
});
