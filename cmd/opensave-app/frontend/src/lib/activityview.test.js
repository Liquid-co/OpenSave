import { describe, expect, it } from 'vitest';
import { periodStart, deviceSlots, activityStats, gameRows, sortRows, barParts, DEVICE_COLOURS } from './activityview.js';

const NOW = Date.parse('2026-11-20T12:00:00Z');
const H = 3600e3;
const ago = (hours) => NOW - hours * H;

const items = [
  { kind: 'played', gameId: 'elden', atMs: ago(2), durationMs: 3 * H },
  { kind: 'played', gameId: 'elden', atMs: ago(30), durationMs: 1 * H, device: 'Steam Deck' },
  { kind: 'played', gameId: 'hades', atMs: ago(50), durationMs: 2 * H, device: 'Steam Deck' },
  { kind: 'played', gameId: 'hades', atMs: ago(24 * 20), durationMs: 5 * H }, // 20 days ago: in the month, not the week
  { kind: 'snapshot', gameId: 'elden', atMs: ago(1), auto: true },
  { kind: 'snapshot', gameId: 'elden', atMs: ago(3), auto: false },
  { kind: 'snapshot', gameId: 'hades', atMs: ago(4), auto: true, device: 'Steam Deck' }, // a copy of the deck's save
  { kind: 'sent', gameId: 'elden', atMs: ago(1), device: 'Steam Deck' },
  { kind: 'received', gameId: 'hades', atMs: ago(5), device: 'Old Laptop' }
];
const games = [
  { gameId: 'elden', lastPlayedAt: ago(2), devices: [{ device: 'Gaming PC', here: true, playtimeMs: 40 * H, sessions: 20 }, { device: 'Steam Deck', playtimeMs: 10 * H, sessions: 4 }] },
  { gameId: 'hades', lastPlayedAt: ago(5), lastPlayedOn: 'Old Laptop', devices: [{ device: 'Steam Deck', playtimeMs: 2 * H, sessions: 1 }] },
  { gameId: 'celeste' }
];

describe('periodStart', () => {
  it('reaches back a rolling week or month, or to the start', () => {
    expect(periodStart('week', NOW)).toBe(NOW - 7 * 864e5);
    expect(periodStart('month', NOW)).toBe(NOW - 30 * 864e5);
    expect(periodStart('all', NOW)).toBe(0);
  });
});

describe('deviceSlots', () => {
  it('puts this device first, then the paired ones in the order they were paired', () => {
    const slots = deviceSlots('Gaming PC', [
      { name: 'Old Laptop', pairedAt: '2026-03-01' },
      { name: 'Steam Deck', pairedAt: '2026-01-01' }
    ], ['Gone Device']);
    expect([...slots]).toEqual([['Gaming PC', 0], ['Steam Deck', 1], ['Old Laptop', 2], ['Gone Device', 3]]);
  });

  it('gives every device past the fifth the shared other colour', () => {
    const peers = Array.from({ length: 8 }, (_, i) => ({ name: 'D' + i, pairedAt: String(i) }));
    const slots = deviceSlots('Here', peers);
    expect(slots.get('D3')).toBe(4);
    expect(slots.get('D4')).toBe(DEVICE_COLOURS);
    expect(slots.get('D7')).toBe(DEVICE_COLOURS);
  });
});

describe('activityStats', () => {
  it('adds up play in the period on each device, with the longest session', () => {
    const s = activityStats({ items, games, here: 'Gaming PC', period: 'week', now: NOW });
    expect(s.playedMs).toBe(6 * H);
    expect(s.byDevice).toEqual({ 'Gaming PC': 3 * H, 'Steam Deck': 3 * H });
    expect(s.sessions).toBe(3);
    expect(s.longest).toEqual({ ms: 3 * H, gameId: 'elden', device: 'Gaming PC' });
  });

  it('counts this device’s own snapshots, and syncs with how many devices', () => {
    const s = activityStats({ items, games, here: 'Gaming PC', period: 'week', now: NOW });
    expect([s.snapshots, s.takenByYou]).toEqual([2, 1]);
    expect([s.syncs, s.syncDevices]).toEqual([2, 2]);
  });

  it('narrows to one device', () => {
    const s = activityStats({ items, games, here: 'Gaming PC', period: 'month', device: 'Steam Deck', now: NOW });
    expect(s.playedMs).toBe(3 * H);
    expect(s.syncs).toBe(1);
  });

  it('takes all-time play from each game’s totals, which are exact', () => {
    const s = activityStats({ items, games, here: 'Gaming PC', period: 'all', now: NOW });
    expect(s.playedMs).toBe(52 * H);
    expect(s.sessions).toBe(25);
    expect(s.byDevice).toEqual({ 'Gaming PC': 40 * H, 'Steam Deck': 12 * H });
  });
});

describe('gameRows', () => {
  it('gives each game its time in the period by device', () => {
    const rows = gameRows({ items, games, here: 'Gaming PC', period: 'month', now: NOW });
    const elden = rows.find((r) => r.gameId === 'elden');
    expect(elden.byDevice).toEqual({ 'Gaming PC': 3 * H, 'Steam Deck': 1 * H });
    expect([elden.playedMs, elden.sessions, elden.lastPlayedOn]).toEqual([4 * H, 2, 'Gaming PC']);
  });

  it('says a game’s time is not recorded when it was last played on a device that tells nothing', () => {
    const rows = gameRows({ items, games, here: 'Gaming PC', period: 'all', now: NOW });
    const hades = rows.find((r) => r.gameId === 'hades');
    expect([hades.lastPlayedOn, hades.unrecorded]).toEqual(['Old Laptop', true]);
    expect(rows.find((r) => r.gameId === 'elden').unrecorded).toBe(false);
    const celeste = rows.find((r) => r.gameId === 'celeste');
    expect([celeste.lastPlayedOn, celeste.playedMs, celeste.unrecorded]).toEqual(['', 0, false]);
  });

  it('sorts by last played, time, sessions or name', () => {
    const rows = gameRows({ items, games, here: 'Gaming PC', period: 'all', now: NOW });
    expect(sortRows(rows, 'played').map((r) => r.gameId)).toEqual(['elden', 'hades', 'celeste']);
    expect(sortRows(rows, 'time').map((r) => r.gameId)).toEqual(['elden', 'hades', 'celeste']);
    expect(sortRows(rows, 'name', { elden: 'Elden Ring', hades: 'Hades II', celeste: 'Celeste' }).map((r) => r.gameId)).toEqual(['celeste', 'elden', 'hades']);
  });
});

describe('barParts', () => {
  it('splits by device in colour order and leaves out devices with no time', () => {
    const slots = deviceSlots('Gaming PC', [{ name: 'Steam Deck', pairedAt: '1' }]);
    expect(barParts({ 'Steam Deck': 1 * H, 'Gaming PC': 3 * H, Other: 0 }, slots)).toEqual([
      { device: 'Gaming PC', ms: 3 * H, share: 0.75, slot: 0 },
      { device: 'Steam Deck', ms: 1 * H, share: 0.25, slot: 1 }
    ]);
    expect(barParts({}, slots)).toEqual([]);
  });
});
