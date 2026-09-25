import { describe, expect, it } from 'vitest';
import { SORTS, conflictedIds, gameStatus, latestSnapshotAt, librarySummary, sortRows } from './gamestatus.js';

const now = Date.parse('2026-09-24T12:00:00Z');
const minsAgo = (m) => new Date(now - m * 60000).toISOString();
const game = (over = {}) => ({
  id: 'g',
  name: 'Game',
  autoSync: true,
  lastSyncedWith: {},
  branches: { main: { name: 'main', snapshots: [] } },
  ...over
});
const snaps = (...times) => ({ main: { name: 'main', snapshots: times.map((t, i) => ({ id: `s${i}`, timestamp: t })) } });

describe('gameStatus', () => {
  it('puts a waiting decision before everything else', () => {
    const s = gameStatus(game(), { conflicted: true, activity: { state: 'running', percentage: 50 }, now });
    expect(s).toMatchObject({ state: 'conflict', tone: 'warn' });
  });

  it('shows a sync in progress with its percentage', () => {
    expect(gameStatus(game(), { activity: { state: 'running', percentage: 42 }, now }).label).toBe('Syncing 42%');
    expect(gameStatus(game(), { activity: { state: 'running' }, now }).label).toBe('Syncing 0%');
  });

  it('says so when the last sync failed', () => {
    expect(gameStatus(game(), { activity: { state: 'error' }, now }).state).toBe('error');
  });

  it('says auto-sync is off rather than reporting a stale sync time', () => {
    const g = game({ autoSync: false, lastSyncedWith: { p1: minsAgo(3) } });
    expect(gameStatus(g, { peers: { p1: {} }, now }).state).toBe('paused');
  });

  it('reports the most recent paired device, ignoring devices since unpaired', () => {
    const g = game({ lastSyncedWith: { p1: minsAgo(30), gone: minsAgo(1) } });
    expect(gameStatus(g, { peers: { p1: {} }, now })).toMatchObject({ state: 'synced', label: 'Synced 30 min ago' });
  });

  it('says a game has not synced when devices are paired but none has it', () => {
    const g = game({ lastSyncedWith: { gone: minsAgo(1) } });
    expect(gameStatus(g, { peers: { p1: {} }, now })).toMatchObject({ state: 'unsynced', tone: 'muted' });
  });

  it('falls back to the newest snapshot when nothing is paired', () => {
    const g = game({ branches: snaps(minsAgo(120), minsAgo(4)) });
    expect(gameStatus(g, { now })).toMatchObject({ state: 'local', label: 'Snapshot 4 min ago', tone: 'ok' });
  });

  it('says there is nothing yet for a game with no snapshots and no devices', () => {
    expect(gameStatus(game(), { now })).toMatchObject({ state: 'empty', tone: 'muted' });
  });
});

describe('latestSnapshotAt', () => {
  it('looks across every branch', () => {
    const g = game({
      branches: {
        main: { snapshots: [{ timestamp: minsAgo(10) }] },
        ng: { snapshots: [{ timestamp: minsAgo(2) }] }
      }
    });
    expect(latestSnapshotAt(g)).toBe(minsAgo(2));
    expect(latestSnapshotAt(game())).toBe(null);
  });
});

describe('conflictedIds', () => {
  it('joins whole-game and per-folder conflicts', () => {
    const ids = conflictedIds({ a: {} }, [{ gameId: 'b', root: 'config' }, { gameId: 'a', root: 'x' }]);
    expect([...ids].sort()).toEqual(['a', 'b']);
    expect(conflictedIds(null, null).size).toBe(0);
  });
});

describe('a damaged snapshot', () => {
  it('is said, after a decision or a failure but before the quiet cases', () => {
    const rows = [
      { game: { name: 'Hades', branches: { main: { snapshots: [{ timestamp: minsAgo(5), problem: 'slot1.sav no longer matches its checksum' }] } } }, status: { state: 'local' } },
      { game: { name: 'Celeste', branches: { main: { snapshots: [{ timestamp: minsAgo(5) }] } } }, status: { state: 'local' } }
    ];
    expect(librarySummary(rows)).toEqual({ tone: 'warn', headline: "A snapshot of Hades can't be restored" });
  });
});

describe('a game being played', () => {
  it('says so, and the recently played order puts it first', () => {
    expect(gameStatus(game({ id: 'h', name: 'Hades', playingSince: '2026-09-25T18:00:00Z' }))).toMatchObject({
      state: 'playing',
      label: 'Playing now'
    });
    const rowOf = (g) => ({ game: g, status: { state: 'local' } });
    const rows = [
      rowOf(game({ id: 'a', name: 'Long ago', lastPlayedAt: '2026-09-01T00:00:00Z' })),
      rowOf(game({ id: 'n', name: 'Never' })),
      rowOf(game({ id: 'p', name: 'Playing', playingSince: '2026-09-25T18:00:00Z' })),
      rowOf(game({ id: 'y', name: 'Yesterday', lastPlayedAt: '2026-09-24T00:00:00Z' }))
    ];
    expect(sortRows(rows, 'played').map((r) => r.game.name)).toEqual(['Playing', 'Yesterday', 'Long ago', 'Never']);
  });
});

describe('a missing save folder', () => {
  it('comes before everything else, for the game and for the library', () => {
    const g = game({ id: 'h', name: 'Hades', savePathMissing: true });
    expect(gameStatus(g, { conflicted: true })).toMatchObject({ state: 'missing', tone: 'warn', label: 'Save folder missing' });
    const rows = [
      { game: g, status: { state: 'missing' } },
      { game: game({ id: 'c', name: 'Celeste' }), status: { state: 'conflict' } }
    ];
    expect(librarySummary(rows)).toEqual({ tone: 'warn', headline: "Hades's save folder is missing" });
    expect(sortRows(rows, 'attention').map((r) => r.game.name)).toEqual(['Hades', 'Celeste']);
  });
});

describe('SORTS', () => {
  const rowOf = (g, state = 'local') => ({ game: g, status: { state } });
  const names = (rows) => rows.map((r) => r.game.name);

  it('recent: the most recently changed first, games without snapshots last, by name', () => {
    const rows = [
      rowOf(game({ id: 'z', name: 'Zed' })),
      rowOf(game({ id: 'o', name: 'Old', branches: snaps(minsAgo(600)) })),
      rowOf(game({ id: 'a', name: 'Alpha' })),
      rowOf(game({ id: 'n', name: 'New', branches: snaps(minsAgo(5)) }))
    ];
    expect(names(sortRows(rows, 'recent'))).toEqual(['New', 'Old', 'Alpha', 'Zed']);
    // Turned around, the whole order is.
    expect(names(sortRows(rows, 'recent', true))).toEqual(['Zed', 'Alpha', 'Old', 'New']);
  });

  it('synced: by the latest sync with any device', () => {
    const rows = [
      rowOf(game({ id: 'a', name: 'A', lastSyncedWith: { deck: minsAgo(60) } })),
      rowOf(game({ id: 'b', name: 'B', lastSyncedWith: { deck: minsAgo(90), laptop: minsAgo(2) } })),
      rowOf(game({ id: 'c', name: 'C' }))
    ];
    expect(names(sortRows(rows, 'synced'))).toEqual(['B', 'A', 'C']);
  });

  it('attention: a decision, then a failure, then a sync running, then the quiet ones', () => {
    const rows = [
      rowOf(game({ id: 'ok', name: 'Fine' }), 'local'),
      rowOf(game({ id: 'x', name: 'Failed' }), 'error'),
      rowOf(game({ id: 'c', name: 'Clash' }), 'conflict'),
      rowOf(game({ id: 's', name: 'Moving' }), 'syncing')
    ];
    expect(names(sortRows(rows, 'attention'))).toEqual(['Clash', 'Failed', 'Moving', 'Fine']);
  });

  it('snapshots, size and added: largest or newest first', () => {
    const sized = (id, sizes, createdAt) =>
      rowOf(game({ id, name: id, createdAt, branches: { main: { snapshots: sizes.map((sizeBytes) => ({ timestamp: minsAgo(1), sizeBytes })) } } }));
    const rows = [sized('few-big', [900], '2026-09-01T00:00:00Z'), sized('many-small', [1, 1, 1], '2026-09-20T00:00:00Z')];
    expect(names(sortRows(rows, 'snapshots'))).toEqual(['many-small', 'few-big']);
    expect(names(sortRows(rows, 'size'))).toEqual(['few-big', 'many-small']);
    expect(names(sortRows(rows, 'added'))).toEqual(['many-small', 'few-big']);
  });

  it('falls back to name for an order it does not know', () => {
    const rows = [rowOf(game({ id: 'b', name: 'B' })), rowOf(game({ id: 'a', name: 'A' }))];
    expect(names(sortRows(rows, 'rating'))).toEqual(['A', 'B']);
    expect(Object.keys(SORTS)).toContain('attention');
  });
});

describe('librarySummary', () => {
  // A game with one snapshot unless its state says it has none.
  const row = (name, state, snapshots = state === 'empty' ? [] : [{ timestamp: minsAgo(9) }]) => ({
    game: { name, branches: { main: { snapshots } } },
    status: { state }
  });

  it('leads with a waiting decision, naming the game when there is one', () => {
    expect(librarySummary([row('Hades', 'conflict'), row('Celeste', 'syncing')])).toEqual({
      tone: 'warn',
      headline: 'Hades needs a decision'
    });
    expect(librarySummary([row('A', 'conflict'), row('B', 'conflict')]).headline).toBe('2 games need a decision');
  });

  it('then a sync in progress', () => {
    expect(librarySummary([row('Celeste', 'syncing'), row('A', 'error')]).headline).toBe('Syncing Celeste…');
    expect(librarySummary([row('A', 'syncing'), row('B', 'syncing')]).headline).toBe('Syncing 2 games…');
  });

  it('then a failed sync, which retries on its own', () => {
    expect(librarySummary([row('Hades', 'error'), row('A', 'empty')])).toMatchObject({ tone: 'warn' });
  });

  it('then games with nothing saved yet, then auto-sync switched off', () => {
    expect(librarySummary([row('A', 'empty'), row('B', 'paused')]).headline).toBe('1 game has no snapshot yet');
    expect(librarySummary([row('A', 'paused'), row('B', 'local')]).headline).toBe('Auto-sync is off for 1 game');
  });

  it('otherwise says everything is backed up', () => {
    expect(librarySummary([row('A', 'local'), row('B', 'synced'), row('C', 'unsynced')])).toEqual({
      tone: 'ok',
      headline: 'All 3 games are backed up'
    });
    expect(librarySummary([row('A', 'local')]).headline).toBe('Your game is backed up');
  });

  it('never counts a game with no snapshot as backed up, whatever its sync says', () => {
    const unsyncedAndEmpty = row('Hades', 'unsynced', []);
    expect(librarySummary([row('A', 'synced'), unsyncedAndEmpty])).toEqual({
      tone: 'muted',
      headline: '1 game has no snapshot yet'
    });
  });
});
