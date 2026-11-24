import { describe, expect, it } from 'vitest';
import {
  arrivalMessage,
  badgeCount,
  canDismiss,
  clearAllHappened,
  dismissOne,
  happened,
  incompleteSnapshotsMessage,
  loadCleared,
  loadSeenAt,
  notCleared,
  saveCleared,
  saveSeenAt,
  waitingOnYou,
  waitingText,
  ARRIVAL_QUIET_MS
} from './notifications.js';
import { translate } from './i18n.js';
import en from '../locales/en.json';

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

  it('does not ask about a game whose drive this device does not have', () => {
    const w = waitingOnYou({ games: { ark: { id: 'ark', name: 'ARK', savePathMissing: true, saveDriveMissing: true, branches: {} } } });
    expect(w).toEqual([]);
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

describe('a missing save folder found on another drive', () => {
  it('waits for a yes, whether or not the drive it was on exists', () => {
    const w = waitingOnYou({ games: { ark: { id: 'ark', name: 'ARK', savePathMissing: true, saveDriveMissing: true, relocateSuggestion: 'E:\SteamLibrary\ARK', branches: {} } } });
    const r = w.find((x) => x.id.startsWith('relocate:'));
    expect(r?.title).toContain('E:\SteamLibrary\ARK');
    expect(r?.go).toEqual({ view: 'game', params: { gameId: 'ark' } });
  });
});

describe('files that may be settings for this device', () => {
  const game = { id: 'ark', name: 'ARK', branches: {}, suggestedSettings: [{ pattern: '/GameUserSettings.ini' }, { pattern: '/Engine.ini' }] };

  it('asks once per game, in the tab where it is answered', () => {
    const w = waitingOnYou({ games: { ark: game } }).filter((x) => x.id.startsWith('settings:'));
    expect(w).toHaveLength(1);
    expect(w[0].title).toBe('2 files of ARK may be settings for this device');
    expect(w[0].go).toEqual({ view: 'game', params: { gameId: 'ark', tab: 'config' } });
  });

  it('asks again when another file is suggested, after one was dismissed', () => {
    const one = waitingOnYou({ games: { ark: { ...game, suggestedSettings: [{ pattern: '/Engine.ini' }] } } })[0];
    const two = waitingOnYou({ games: { ark: game } })[0];
    expect(one.id).not.toBe(two.id);
  });

  it('says nothing for a game with no suggestion', () => {
    expect(waitingOnYou({ games: { ark: { ...game, suggestedSettings: [] } } })).toEqual([]);
  });
});

describe('a game database link to confirm', () => {
  it('asks in the tab where it is answered, and again for another match', () => {
    const game = { id: 'duck', name: 'Duck', branches: {}, gameDBSuggestion: { appId: '312530', name: 'Duck Game' } };
    const w = waitingOnYou({ games: { duck: game } });
    expect(w.map((x) => x.id)).toEqual(['gamedb:duck:312530']);
    expect(w[0].title).toBe('Duck may be Duck Game in the game database');
    expect(w[0].go).toEqual({ view: 'game', params: { gameId: 'duck', tab: 'config' } });
    expect(canDismiss(w[0])).toBe(true);
  });
});

describe('incompleteSnapshotsMessage', () => {
  const t = (key, params) => translate(en, key, params, new Intl.PluralRules('en'));

  it('names the game and the files it holds open', () => {
    expect(incompleteSnapshotsMessage({ gameName: 'Hades', files: ['save.lock'], count: 1 }, t)).toBe(
      '"Hades": automatic snapshots keep missing save.lock, which the game holds open. Until those can be read, usually once the game is closed, its changes are not backed up or synced.'
    );
  });

  it('says how many more there are than it names', () => {
    const one = incompleteSnapshotsMessage({ gameName: 'Hades', files: ['a', 'b', 'c'], count: 4 }, t);
    expect(one).toContain('missing a, b, c and one more file, which');
    const more = incompleteSnapshotsMessage({ gameName: 'Hades', files: ['a', 'b', 'c'], count: 9 }, t);
    expect(more).toContain('missing a, b, c and 6 more files, which');
  });
});

describe('clearing the bell', () => {
  const ev = (id, atMs) => ({ id, atMs });
  const store = () => {
    const m = new Map();
    return { getItem: (k) => (m.has(k) ? m.get(k) : null), setItem: (k, v) => m.set(k, v) };
  };

  it('dismisses news but never a problem', () => {
    for (const id of ['new:Hades', 'update:2.5.0', 'offer:hades:snap_1', 'settings:hades:/a.ini']) expect(canDismiss({ id }), id).toBe(true);
    for (const id of ['conflict:hades', 'location:hades:config', 'emptied:hades', 'missing:hades', 'damaged:hades', 'pair:node_1', 'relocate:hades:E:/Games/x'])
      expect(canDismiss({ id }), id).toBe(false);
  });

  it('hides a dismissed item, and everything up to a clear-all', () => {
    const list = [ev('received:a:100', 100), ev('received:a:200', 200), ev('received:a:300', 300), { id: 'update:2.5.0' }];
    let cleared = { before: 0, ids: [] };
    const listed = new Set(list.map((x) => x.id));
    cleared = dismissOne(cleared, 'received:a:300', listed);
    expect(notCleared(list, cleared).map((x) => x.id)).toEqual(['received:a:100', 'received:a:200', 'update:2.5.0']);
    cleared = clearAllHappened(cleared, 250, listed);
    // What happened after the clear still shows; what waits on you is not
    // passed by a clear of what happened.
    expect(notCleared([...list, ev('received:a:400', 400)], cleared).map((x) => x.id)).toEqual(['update:2.5.0', 'received:a:400']);
  });

  it('shows the next one of the same kind', () => {
    let cleared = dismissOne({ before: 0, ids: [] }, 'update:2.5.0', new Set(['update:2.5.0']));
    expect(notCleared([{ id: 'update:2.5.0' }], cleared)).toEqual([]);
    expect(notCleared([{ id: 'update:2.6.0' }], cleared)).toEqual([{ id: 'update:2.6.0' }]);
  });

  it('forgets ids of what is no longer listed', () => {
    let cleared = { before: 0, ids: ['update:2.4.0', 'new:Old'] };
    cleared = dismissOne(cleared, 'update:2.5.0', new Set(['update:2.5.0', 'new:Old']));
    expect(cleared.ids.sort()).toEqual(['new:Old', 'update:2.5.0']);
  });

  it('is kept across a restart, and survives storage that refuses', () => {
    const s = store();
    saveCleared({ before: 5, ids: ['new:Hades'] }, s);
    expect(loadCleared(s)).toEqual({ before: 5, ids: ['new:Hades'] });
    expect(loadCleared({ getItem: () => 'not json' })).toEqual({ before: 0, ids: [] });
    expect(loadCleared({ getItem: () => { throw new Error('no'); } })).toEqual({ before: 0, ids: [] });
    expect(() => saveCleared({ before: 1, ids: [] }, { setItem: () => { throw new Error('no'); } })).not.toThrow();
  });
});

// waitingOnYou and waitingText build the same cases twice: once in English
// words, once as catalog keys. The bell reads them by position, so the two
// must agree case for case and in the same order. A case added to one, dropped
// from the other, or listed in a different order fails here.
describe('the bell says what waits on you', () => {
  const t = (key, params) => translate(en, key, params, new Intl.PluralRules('en'));

  // One of every case at once, so the full order is exercised.
  const everything = {
    games: {
      hades: { id: 'hades', name: 'Hades', emptied: { state: 'held' }, branches: { main: { snapshots: [{ id: 's1', problem: 'damaged' }] } } },
      tunic: { id: 'tunic', name: 'Tunic', savePathMissing: true, saveDriveMissing: true, relocateSuggestion: 'E:\SteamLibrary\Tunic', branches: {} },
      celeste: { id: 'celeste', name: 'Celeste', savePathMissing: true, branches: {}, suggestedSettings: [{ pattern: '/settings.ini' }] },
      duck: { id: 'duck', name: 'Duck', branches: {}, gameDBSuggestion: { appId: '312530', name: 'Duck Game' } }
    },
    conflicts: { hades: {} },
    locationConflicts: [{ gameId: 'hades', root: 'Saves' }],
    pairingRequests: [{ peerId: 'p', deviceName: 'Steam Deck' }],
    cloudOffers: [{ gameId: 'celeste', snapshotId: 's9', gameName: 'Celeste', deviceName: 'Laptop' }],
    newGames: [{ name: 'Balatro' }],
    update: { latest: '2.5.0', current: '2.4.0' }
  };

  // Which case a waitingText piece speaks for, read back from its keys.
  const caseOfSaid = (piece) => {
    if (piece.title.key === 'notifications.waiting.conflict') {
      return piece.detail.key === 'notifications.waiting.locationDetail' ? 'location' : 'conflict';
    }
    return piece.title.key.slice('notifications.waiting.'.length);
  };

  it('lists the same cases, in the same order, as waitingOnYou', () => {
    const listed = waitingOnYou(everything);
    const said = waitingText(everything, t);
    expect(said).toHaveLength(listed.length);
    expect(said.map(caseOfSaid)).toEqual(listed.map((x) => x.id.split(':')[0]));
    expect(listed.map((x) => x.id.split(':')[0])).toEqual([
      'conflict', 'location', 'emptied', 'pair', 'offer', 'relocate', 'missing', 'damaged', 'settings', 'gamedb', 'new', 'update'
    ]);
  });

  it('names every piece with a key the catalog has', () => {
    for (const piece of waitingText(everything, t)) {
      expect(en[piece.title.key], piece.title.key).toBeTruthy();
      if (piece.detail.key) expect(en[piece.detail.key], piece.detail.key).toBeTruthy();
    }
  });

  it('says nothing when nothing waits', () => {
    expect(waitingText({}, t)).toEqual([]);
    expect(waitingOnYou({})).toEqual([]);
  });
});
