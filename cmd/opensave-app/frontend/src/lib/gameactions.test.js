import { describe, expect, it } from 'vitest';
import { gameMenuItems, latestSnapshot } from './gameactions.js';

const game = (extra = {}) => ({
  id: 'g1',
  name: 'Hades',
  activeBranch: 'main',
  branches: {
    main: { snapshots: [{ id: 's1', timestamp: '2026-09-20T10:00:00Z' }, { id: 's2', timestamp: '2026-09-22T10:00:00Z' }] },
    old: { snapshots: [{ id: 's9', timestamp: '2026-09-23T10:00:00Z' }] }
  },
  ...extra
});
const ids = (items) => items.map((i) => i?.id ?? '—');

describe('latestSnapshot', () => {
  it('is the newest on the branch being played, not the newest anywhere', () => {
    expect(latestSnapshot(game()).id).toBe('s2');
  });
  it('is null with nothing to restore', () => {
    expect(latestSnapshot(game({ branches: {} }))).toBe(null);
    expect(latestSnapshot(null)).toBe(null);
  });
});

describe('gameMenuItems', () => {
  it('offers Launch only for a game that can be launched', () => {
    expect(ids(gameMenuItems(game()))).not.toContain('launch');
    expect(ids(gameMenuItems(game({ appId: '1145360' })))).toContain('launch');
    expect(ids(gameMenuItems(game({ exePath: 'C:/Games/Hades.exe' })))).toContain('launch');
  });

  it('greys out restoring when there is no snapshot, and says when the latest is from', () => {
    const none = gameMenuItems(game({ branches: {} })).find((i) => i?.id === 'restore-latest');
    expect(none).toMatchObject({ disabled: true, hint: 'none yet' });
    const some = gameMenuItems(game(), { now: new Date('2026-09-22T12:00:00Z') }).find((i) => i?.id === 'restore-latest');
    expect(some.disabled).toBe(false);
    expect(some.hint).toBeTruthy();
  });

  it('offers to add to Favourites, or to remove when it is one', () => {
    expect(gameMenuItems(game()).find((i) => i?.id === 'favourite').label).toBe('Add to Favourites');
    expect(gameMenuItems(game(), { favourite: true }).find((i) => i?.id === 'favourite').label).toBe('Remove from Favourites');
  });

  it('puts the destructive action last, apart and marked', () => {
    const items = gameMenuItems(game());
    expect(items.at(-1)).toMatchObject({ id: 'untrack', danger: true });
    expect(items.at(-2)).toBe(null);
  });
});
