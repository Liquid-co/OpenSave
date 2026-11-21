import { describe, it, expect } from 'vitest';
import {
  timelineWindow,
  placeIn,
  playLanes,
  snapshotMarks,
  branchOrigin,
  branchMarks,
  devicesOn,
  heroStatus,
  playedTotal,
  latestOf
} from './gamepage.js';

const H = 3600e3;
const NOW = new Date(2026, 10, 20, 15, 0).getTime(); // Nov 20, 3 PM local
const win = timelineWindow(NOW, 14);
const iso = (ms) => new Date(ms).toISOString();

describe('timelineWindow', () => {
  it('runs fourteen local days, ending at tonight’s midnight', () => {
    expect(win.dayStarts.length).toBe(14);
    expect(new Date(win.dayStarts[0]).getDate()).toBe(7);
    expect(new Date(win.end).getDate()).toBe(21);
    expect(placeIn(win, win.start - 1)).toBe(0);
    expect(placeIn(win, win.end + 1)).toBe(1);
  });
});

describe('playLanes', () => {
  const items = [
    { kind: 'played', gameId: 'elden', atMs: NOW - 2 * H, durationMs: 3 * H },
    { kind: 'played', gameId: 'elden', atMs: NOW - 30 * H, durationMs: H, device: 'Steam Deck' },
    { kind: 'played', gameId: 'hades', atMs: NOW - 5 * H, durationMs: H },
    { kind: 'played', gameId: 'elden', atMs: NOW - 40 * 24 * H, durationMs: H }, // before the window
    { kind: 'snapshot', gameId: 'elden', atMs: NOW - H }
  ];
  it('gives this device a lane first, then each device that played it', () => {
    const lanes = playLanes(items, 'elden', 'Gaming PC', win);
    expect(lanes.map((l) => [l.device, l.here, l.sessions.length])).toEqual([
      ['Gaming PC', true, 1],
      ['Steam Deck', false, 1]
    ]);
  });
  it('places a session from its start, which is its end less its length', () => {
    const [pc] = playLanes(items, 'elden', 'Gaming PC', win);
    const s = pc.sessions[0];
    expect(s.startMs).toBe(NOW - 5 * H);
    expect(s.at).toBeCloseTo(placeIn(win, NOW - 5 * H));
    expect(s.width).toBeCloseTo(placeIn(win, NOW - 2 * H) - s.at);
  });
  it('keeps this device’s lane when it never played the game', () => {
    expect(playLanes([], 'elden', 'Gaming PC', win)).toEqual([{ device: 'Gaming PC', here: true, sessions: [] }]);
  });
});

const snap = (id, ms, comment = '', extra = {}) => ({ id, timestamp: iso(ms), comment, isSystemAuto: true, ...extra });

describe('branches', () => {
  it('reads where a branch came from in its first snapshot', () => {
    expect(branchOrigin({ name: 'main', snapshots: [snap('a', NOW - 10 * 24 * H)] })).toEqual({ kind: 'main', ms: NOW - 10 * 24 * H });
    expect(branchOrigin({ name: 'ng', snapshots: [snap('b', NOW - H), snap('a', NOW - 2 * H, 'Branch "ng" created from "main"')] })).toEqual({ kind: 'copied', from: 'main', ms: NOW - 2 * H });
    expect(branchOrigin({ name: 'conflict-steam-deck-1234', snapshots: [snap('c', NOW - 3 * H, 'Synced from peer: Steam Deck (Auto backup)')] })).toEqual({ kind: 'kept', peer: 'Steam Deck', ms: NOW - 3 * H });
    expect(branchOrigin({ name: 'other', snapshots: [snap('d', NOW - H, 'After playing for 40 min')] })).toEqual({ kind: 'made', ms: NOW - H });
    expect(branchOrigin({ name: 'fresh', snapshots: [] })).toEqual({ kind: 'empty', ms: 0 });
  });
  it('marks a branch on the timeline only when it began inside the window', () => {
    const game = {
      branches: {
        main: { name: 'main', snapshots: [snap('m', NOW - 3 * H)] },
        ng: { name: 'ng', snapshots: [snap('n', NOW - 2 * 24 * H, 'Branch "ng" created from "main"')] },
        old: { name: 'old', snapshots: [snap('o', NOW - 60 * 24 * H)] }
      }
    };
    expect(branchMarks(game, win).map((m) => m.name)).toEqual(['ng']);
  });
  it('says who is on a branch: this device by its own choice, others as of their last sync', () => {
    const game = { activeBranch: 'main', peerBranches: { deck: { branch: 'main', seenMs: 5 }, laptop: { branch: 'ng', seenMs: 6 }, gone: { branch: 'main', seenMs: 7 } } };
    const peers = { deck: { name: 'Steam Deck' }, laptop: { name: 'Laptop' } };
    expect(devicesOn('main', game, peers, 'Gaming PC')).toEqual([
      { name: 'Gaming PC', here: true, seenMs: 0 },
      { name: 'Steam Deck', here: false, seenMs: 5 }
    ]);
    expect(devicesOn('ng', game, peers, 'Gaming PC').map((d) => d.name)).toEqual(['Laptop']);
  });
  it('finds a branch’s newest snapshot', () => {
    expect(latestOf({ snapshots: [snap('a', NOW - 2 * H), snap('b', NOW - H)] }).id).toBe('b');
    expect(latestOf({ snapshots: [] })).toBeNull();
  });
});

describe('snapshotMarks', () => {
  it('places the snapshots inside the window, oldest first, from every branch', () => {
    const game = { branches: { main: { name: 'main', snapshots: [snap('b', NOW - H), snap('a', NOW - 3 * H), snap('x', NOW - 90 * 24 * H)] }, ng: { name: 'ng', snapshots: [snap('c', NOW - 2 * H)] } } };
    expect(snapshotMarks(game, win).map((m) => [m.snap.id, m.snap.branch])).toEqual([
      ['a', 'main'],
      ['c', 'ng'],
      ['b', 'main']
    ]);
  });
});

describe('heroStatus', () => {
  const peers = { deck: { id: 'deck', name: 'Steam Deck' }, laptop: { id: 'laptop', name: 'Laptop' } };
  it('puts playing, then syncing, then a choice waiting first', () => {
    expect(heroStatus({ game: { playingSince: iso(NOW) }, peers, activity: { state: 'running' } }).kind).toBe('playing');
    expect(heroStatus({ game: {}, peers, activity: { state: 'running', percentage: 40 }, conflict: true })).toEqual({ kind: 'syncing', percentage: 40 });
    expect(heroStatus({ game: {}, peers, conflict: true }).kind).toBe('conflict');
  });
  it('names the device last synced with, and counts the rest', () => {
    const game = { lastSyncedWith: { deck: iso(NOW - H), laptop: iso(NOW - 2 * H) } };
    expect(heroStatus({ game, peers })).toEqual({ kind: 'synced', device: 'Steam Deck', ms: NOW - H, others: 1 });
  });
  it('tells never synced from no devices at all', () => {
    expect(heroStatus({ game: {}, peers }).kind).toBe('never');
    expect(heroStatus({ game: {}, peers: {} }).kind).toBe('alone');
  });
});

describe('playedTotal', () => {
  it('adds every device’s time, falling back to this device’s', () => {
    expect(playedTotal({ devices: [{ playtimeMs: 2 * H }, { playtimeMs: H }] }, { playtimeMs: 5 })).toBe(3 * H);
    expect(playedTotal(null, { playtimeMs: 5 })).toBe(5);
  });
});
