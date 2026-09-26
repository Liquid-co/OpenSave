import { describe, expect, it } from 'vitest';
import { arrivalMessage, badgeCount, happened, loadSeenAt, saveSeenAt, waitingOnYou, ARRIVAL_QUIET_MS } from './notifications.js';

const games = {
  hades: { id: 'hades', name: 'Hades', branches: { main: { snapshots: [{ id: 's1', problem: 'damaged' }] } } },
  celeste: { id: 'celeste', name: 'Celeste', emptied: { state: 'held' }, branches: {} },
  tunic: { id: 'tunic', name: 'Tunic', savePathMissing: true, branches: {} }
};

describe('notifications', () => {
  it('lists what is waiting on you, most pressing first', () => {
    const w = waitingOnYou({
      games,
      conflicts: { hades: {} },
      pairingRequests: [{ peerId: 'p', deviceName: 'Steam Deck' }],
      newGames: [{ name: 'Balatro' }],
      update: { latest: '2.5.0', current: '2.4.0' }
    });
    expect(w.map((x) => x.id.split(':')[0])).toEqual(['conflict', 'emptied', 'pair', 'missing', 'damaged', 'new', 'update']);
    expect(w[0]).toMatchObject({ title: 'Hades needs a decision', go: { view: 'game', params: { gameId: 'hades' } } });
    expect(w.find((x) => x.id.startsWith('damaged')).title).toBe("1 snapshot of Hades can't be restored");
  });

  it('turns activity into events, unread since the bell was last opened', () => {
    const now = Date.UTC(2026, 8, 25, 12);
    const items = [
      { kind: 'received', gameId: 'hades', device: 'Steam Deck', files: 2, atMs: now - 60_000 },
      { kind: 'sent', gameId: 'hades', device: 'Steam Deck', files: 2, atMs: now - 50_000 },
      { kind: 'cloud-pulled', gameId: 'celeste', device: 'Laptop', atMs: now - 3_600_000 },
      { kind: 'received', gameId: 'gone', device: 'X', files: 1, atMs: now - 1000 },
      { kind: 'received', gameId: 'hades', device: 'Deck', files: 1, atMs: now - 30 * 86_400_000 }
    ];
    const ev = happened(items, games, now - 600_000, now);
    expect(ev.map((e) => e.title)).toEqual(['Hades: got 2 files from Steam Deck', "Celeste: brought Laptop's newer save from the cloud"]);
    expect(ev.map((e) => e.unread)).toEqual([true, false]);
    expect(badgeCount([{}, {}], ev)).toBe(3);
  });

  it('remembers when the bell was opened', () => {
    const mem = new Map();
    const storage = { getItem: (k) => mem.get(k) ?? null, setItem: (k, v) => mem.set(k, v) };
    expect(loadSeenAt(storage)).toBe(0);
    saveSeenAt(1234, storage);
    expect(loadSeenAt(storage)).toBe(1234);
  });
});

describe('a save arriving', () => {
  it('is said once in a while for each game, not at every save the other device makes', () => {
    const shown = {};
    const t0 = 1_000_000;
    const ev = { kind: 'received', gameId: 'hades', device: 'Steam Deck', files: 2 };
    expect(arrivalMessage(ev, games, shown, t0)).toBe('Hades: got 2 files from Steam Deck');
    expect(arrivalMessage(ev, games, shown, t0 + 60_000)).toBeNull();
    expect(arrivalMessage({ ...ev, gameId: 'celeste' }, games, shown, t0 + 60_000)).toBe('Celeste: got 2 files from Steam Deck');
    expect(arrivalMessage(ev, games, shown, t0 + ARRIVAL_QUIET_MS + 1)).not.toBeNull();
    expect(arrivalMessage({ ...ev, kind: 'sent' }, games, {}, t0)).toBeNull();
  });
});
