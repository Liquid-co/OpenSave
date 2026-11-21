import { describe, expect, it } from 'vitest';
import { peerWhere, gamesOnBoth, recentWith, isOlder, homeRelayAddress, syncingWith } from './devicesview.js';

describe('peerWhere', () => {
  it('tells this network, the internet and offline apart', () => {
    expect(peerWhere({ status: 'online', address: '192.168.1.5' })).toBe('network');
    expect(peerWhere({ status: 'online', address: 'relay' })).toBe('internet');
    expect(peerWhere({ status: 'online', address: '192.168.1.5', overRelay: true })).toBe('internet');
    expect(peerWhere({ status: 'offline', address: '192.168.1.5' })).toBe('offline');
    expect(peerWhere(undefined)).toBe('offline');
  });
});

describe('gamesOnBoth', () => {
  it('is the games synced with that device, by name', () => {
    const games = {
      a: { id: 'a', name: 'Hades II', lastSyncedWith: { deck: 'x' } },
      b: { id: 'b', name: 'Celeste', lastSyncedWith: { deck: 'x', laptop: 'y' } },
      c: { id: 'c', name: 'Balatro', lastSyncedWith: { laptop: 'y' } },
      d: { id: 'd', name: 'Sekiro' }
    };
    expect(gamesOnBoth(games, 'deck').map((g) => g.id)).toEqual(['b', 'a']);
    expect(gamesOnBoth(games, 'nobody')).toEqual([]);
  });
});

describe('recentWith', () => {
  it('keeps the latest files sent to or received from that device', () => {
    const items = [
      { kind: 'sent', device: 'Deck', atMs: 1 },
      { kind: 'received', device: 'Deck', atMs: 3 },
      { kind: 'snapshot', device: 'Deck', atMs: 9 },
      { kind: 'sent', device: 'Laptop', atMs: 5 },
      { kind: 'sent', device: 'Deck', atMs: 2 }
    ];
    expect(recentWith(items, 'Deck').map((i) => i.atMs)).toEqual([3, 2]);
  });
});

describe('isOlder', () => {
  it('compares by number, and puts a pre-release before its release', () => {
    expect(isOlder('2.4.1', '2.5.0')).toBe(true);
    expect(isOlder('2.5.0', '2.5.0')).toBe(false);
    expect(isOlder('2.10.0', '2.9.3')).toBe(false);
    expect(isOlder('2.5.0-beta.1', '2.5.0')).toBe(true);
    expect(isOlder('2.5.0', '2.5.0-beta.1')).toBe(false);
    expect(isOlder('', '2.5.0')).toBe(false);
    expect(isOlder('2.4.1', 'dev')).toBe(false);
  });
});

describe('homeRelayAddress', () => {
  it('uses a private address on the relay port', () => {
    // A WSL or Docker adapter is not where the other devices are.
    expect(homeRelayAddress(['172.18.0.1', '192.168.1.20'], 8384)).toBe('ws://192.168.1.20:8384');
    expect(homeRelayAddress(['172.18.0.1', '100.101.5.6'], 8384)).toBe('ws://100.101.5.6:8384');
    expect(homeRelayAddress(['203.0.113.9', '192.168.1.20'], 8384)).toBe('ws://192.168.1.20:8384');
    expect(homeRelayAddress([], 8384)).toBe('');
  });
});

describe('syncingWith', () => {
  it("lists only what is running with that device, newest first", () => {
    const activity = {
      hades: { state: 'running', peerName: 'Steam Deck', direction: 'download', percentage: 40, at: 2 },
      celeste: { state: 'running', peerName: 'Steam Deck', at: 5 },
      tunic: { state: 'running', peerName: 'Laptop', at: 9 },
      hk: { state: 'done', peerName: 'Steam Deck', at: 10 }
    };
    expect(syncingWith(activity, 'Steam Deck')).toEqual([
      { gameId: 'celeste', direction: '', percentage: 0 },
      { gameId: 'hades', direction: 'download', percentage: 40 }
    ]);
    expect(syncingWith(activity, 'Nobody')).toEqual([]);
    expect(syncingWith(null, 'Steam Deck')).toEqual([]);
    expect(syncingWith(activity, '')).toEqual([]);
  });
});
