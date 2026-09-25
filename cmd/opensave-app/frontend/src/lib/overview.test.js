import { describe, expect, it } from 'vitest';
import { recentEvents, spaceUsed } from './overview.js';

const now = new Date(2026, 8, 25, 12, 0); // 25 Sep 2026, noon, local
const at = (daysAgo, hour = 10) => new Date(2026, 8, 25 - daysAgo, hour).toISOString();
const snap = (timestamp, extra = {}) => ({ timestamp, isSystemAuto: true, comment: 'Auto backup', sizeBytes: 100, ...extra });

const games = {
  hades: {
    id: 'hades',
    name: 'Hades',
    branches: {
      main: { snapshots: [snap(at(0, 9)), snap(at(0, 9.5)), snap(at(0, 10)), snap(at(1), { isSystemAuto: false, comment: 'Before the boss', sizeBytes: 250 })] },
      side: { snapshots: [snap(at(20))] }
    },
    lastSyncedWith: { deck: at(0, 11), gone: at(0, 11.5) }
  },
  celeste: { id: 'celeste', name: 'Celeste', branches: { main: { snapshots: [snap(at(1, 8))] } } }
};
const peers = { deck: { name: 'Steam Deck' } };

describe('recentEvents', () => {
  it('lists snapshots and syncs newest first, a run of the same as one line', () => {
    const events = recentEvents(games, peers, { now });
    expect(events.map((e) => [e.game.id, e.title, e.count])).toEqual([
      ['hades', 'Synced with Steam Deck', 1],
      ['hades', 'Save changed', 3],
      ['hades', 'Before the boss', 1],
      ['celeste', 'Save changed', 1],
      ['hades', 'Save changed', 1]
    ]);
    expect(events[1].at).toBe(new Date(at(0, 10)).getTime());
  });

  it('leaves out syncs with a device that is no longer paired', () => {
    expect(recentEvents(games, {}, { now }).some((e) => e.kind === 'sync')).toBe(false);
  });

  it('stops at the limit', () => {
    expect(recentEvents(games, peers, { now, limit: 2 })).toHaveLength(2);
    expect(recentEvents({}, peers, { now })).toEqual([]);
  });
});

describe('spaceUsed', () => {
  it('adds up every snapshot on every branch', () => {
    expect(spaceUsed(games)).toBe(100 * 5 + 250);
    expect(spaceUsed({})).toBe(0);
  });
});
