import { describe, expect, it } from 'vitest';
import { SORTS, conflictedIds, gameStatus, latestSnapshotAt, librarySummary } from './gamestatus.js';

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

describe('SORTS.recent', () => {
  it('puts the most recently changed first and games without snapshots last, by name', () => {
    const list = [
      game({ id: 'z', name: 'Zed' }),
      game({ id: 'o', name: 'Old', branches: snaps(minsAgo(600)) }),
      game({ id: 'a', name: 'Alpha' }),
      game({ id: 'n', name: 'New', branches: snaps(minsAgo(5)) })
    ];
    expect([...list].sort(SORTS.recent.compare).map((g) => g.name)).toEqual(['New', 'Old', 'Alpha', 'Zed']);
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
