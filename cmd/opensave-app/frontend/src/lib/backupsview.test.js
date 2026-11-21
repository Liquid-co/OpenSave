import { describe, it, expect } from 'vitest';
import {
  copyKind,
  copyTime,
  gameState,
  backupSummary,
  shelfOrder,
  inFilter,
  stackEdges
} from './backupsview.js';

const copy = (o = {}) => ({ name: 'x', snapshotId: 'snap_1', known: true, comment: '', isSystemAuto: true, ...o });

describe('copyKind', () => {
  it('reads the reason from this PC’s own record', () => {
    expect(copyKind(copy({ isSystemAuto: false, comment: 'Before the boss' }))).toBe('manual');
    expect(copyKind(copy({ comment: 'After playing for 42 min' }))).toBe('session');
    expect(copyKind(copy({ safety: true, comment: 'Before sync replaced local files' }))).toBe('safety');
    expect(copyKind(copy({ comment: '' }))).toBe('auto');
    expect(copyKind(copy({ comment: 'Initial snapshot' }))).toBe('auto');
  });
  it('does not guess for a copy this PC does not hold', () => {
    expect(copyKind(copy({ known: false, isSystemAuto: false }))).toBe('unknown');
  });
});

describe('copyTime', () => {
  it('prefers when the snapshot was taken over when it was uploaded', () => {
    expect(copyTime({ takenMs: 1000, createdTime: '2026-11-20T10:00:00Z' })).toBe(1000);
    expect(copyTime({ takenMs: 0, createdTime: '2026-11-20T10:00:00Z' })).toBe(Date.parse('2026-11-20T10:00:00Z'));
    expect(copyTime({})).toBe(0);
  });
});

describe('gameState', () => {
  it('is safe only when the save this PC is at is up', () => {
    expect(gameState({ tracked: true, copies: [copy()], current: 'snap_1', currentUploaded: true })).toBe('safe');
    expect(gameState({ tracked: true, copies: [copy()], current: 'snap_2', currentUploaded: false })).toBe('behind');
  });
  it('tells waiting from having nothing to send', () => {
    expect(gameState({ tracked: true, copies: [], localSnapshots: 3 })).toBe('waiting');
    expect(gameState({ tracked: true, copies: [], localSnapshots: 0 })).toBe('empty');
    expect(gameState({ tracked: false, copies: [copy()] })).toBe('cloudOnly');
  });
});

describe('backupSummary', () => {
  it('counts tracked games only, and names the ones waiting', () => {
    const games = [
      { gameName: 'A', tracked: true, copies: [copy({ takenMs: 50 })], current: 'snap_1', currentUploaded: true },
      { gameName: 'B', tracked: true, copies: [], localSnapshots: 2 },
      { gameName: 'C', tracked: true, copies: [copy({ takenMs: 10 })], current: 'snap_9', currentUploaded: false },
      { gameName: 'D', tracked: false, copies: [copy({ takenMs: 90 })] }
    ];
    expect(backupSummary(games)).toEqual({ tracked: 3, safe: 1, waiting: ['B', 'C'], lastMs: 90 });
  });
  it('dates the last backup by the upload, not the snapshot', () => {
    const old = copy({ takenMs: 1000, createdTime: '2026-11-21T02:00:00Z' });
    expect(backupSummary([{ gameName: 'A', tracked: true, copies: [old] }]).lastMs).toBe(Date.parse('2026-11-21T02:00:00Z'));
  });
});

describe('shelf', () => {
  const games = [
    { gameName: 'Old', tracked: true, copies: [copy({ takenMs: 10 })], current: 'snap_1', currentUploaded: true },
    { gameName: 'New', tracked: true, copies: [copy({ takenMs: 90 })], current: 'snap_1', currentUploaded: true },
    { gameName: 'Waiting', tracked: true, copies: [], localSnapshots: 1 }
  ];
  it('puts what needs an upload first, then the newest', () => {
    expect(shelfOrder(games).map((g) => g.gameName)).toEqual(['Waiting', 'New', 'Old']);
  });
  it('filters', () => {
    expect(games.filter((g) => inFilter(g, 'safe')).length).toBe(2);
    expect(games.filter((g) => inFilter(g, 'notYet')).map((g) => g.gameName)).toEqual(['Waiting']);
  });
  it('grows the stack with the copies', () => {
    expect([0, 1, 2, 4, 5, 20].map(stackEdges)).toEqual([0, 0, 1, 1, 2, 2]);
  });
});
