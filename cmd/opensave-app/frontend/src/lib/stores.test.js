import { describe, expect, it } from 'vitest';
import { get } from 'svelte/store';
import { applyMessage, gameList, stateLoaded } from './stores.js';

describe('stateLoaded', () => {
  it('stays false until the first full state, so an empty list is not yet "no games"', () => {
    expect(get(stateLoaded)).toBe(false);
    applyMessage({ type: 'games-update', data: {} });
    expect(get(stateLoaded)).toBe(false);

    applyMessage({ type: 'init', data: { games: {} } });
    expect(get(stateLoaded)).toBe(true);
    expect(get(gameList)).toEqual([]);
  });
});
