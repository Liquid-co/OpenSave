import { describe as suite, expect, it } from 'vitest';
import { byDay, describe, filterItems, playedWhere, runs } from './timeline.js';

const now = new Date(2026, 8, 25, 18, 0);
const at = (h, m = 0, day = 25) => new Date(2026, 8, day, h, m).getTime();

suite('timeline', () => {
  it('says each kind of event plainly', () => {
    expect(describe({ kind: 'received', device: 'Steam Deck', files: 3, bytes: 2048 }, now)).toMatchObject({
      title: 'Got 3 files from Steam Deck',
      detail: '2.0 KB'
    });
    expect(describe({ kind: 'sent', device: 'Laptop', files: 1 }, now).title).toBe('Laptop took 1 file from here');
    expect(describe({ kind: 'played', durationMs: 72 * 60_000 }, now).title).toBe('Played for 1 h 12 min');
    expect(describe({ kind: 'snapshot', comment: 'Synced from peer: Deck (Save changed)', device: 'Deck', auto: true }, now).title).toBe(
      "Kept a copy of Deck's save"
    );
    expect(describe({ kind: 'snapshot', comment: 'Before the boss', auto: false }, now)).toMatchObject({
      title: 'Snapshot: Before the boss',
      detail: 'Taken by you'
    });
    expect(describe({ kind: 'emptied', files: 4 }, now).tone).toBe('warn');
  });

  it('folds a run of the same thing into one line with a count', () => {
    const items = [
      { kind: 'snapshot', gameId: 'hades', comment: '', auto: true, atMs: at(17) },
      { kind: 'snapshot', gameId: 'hades', comment: '', auto: true, atMs: at(16) },
      { kind: 'snapshot', gameId: 'celeste', comment: '', auto: true, atMs: at(15) },
      { kind: 'played', gameId: 'hades', durationMs: 60_000, atMs: at(14) },
      { kind: 'played', gameId: 'hades', durationMs: 60_000, atMs: at(13) }
    ];
    const r = runs(items, now);
    expect(r.map((x) => [x.item.gameId, x.count])).toEqual([
      ['hades', 2],
      ['celeste', 1],
      ['hades', 1],
      ['hades', 1]
    ]);
  });

  it('filters by kind and by game, and groups by day', () => {
    const items = [
      { kind: 'received', gameId: 'a', device: 'D', files: 1, atMs: at(10) },
      { kind: 'snapshot', gameId: 'b', comment: '', auto: true, atMs: at(9, 0, 24) }
    ];
    expect(filterItems(items, 'syncs').map((i) => i.kind)).toEqual(['received']);
    expect(filterItems([{ kind: 'conflict', gameId: 'a' }, { kind: 'emptied', gameId: 'a' }], 'syncs')).toHaveLength(2);
    expect(filterItems(items, 'all', 'b').map((i) => i.gameId)).toEqual(['b']);
    expect(byDay(runs(items, now), now).map((d) => d.day)).toEqual(['Today', 'Yesterday']);
  });

  it('says where a game was last played', () => {
    expect(playedWhere({ lastPlayedAt: 1, lastPlayedOn: 'Steam Deck' }, 'Gaming PC')).toBe('Steam Deck');
    expect(playedWhere({ lastPlayedAt: 1 }, 'Gaming PC')).toBe('Gaming PC (here)');
    expect(playedWhere({}, 'Gaming PC')).toBe('');
  });
});
