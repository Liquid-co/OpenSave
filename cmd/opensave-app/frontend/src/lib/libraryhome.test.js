import { describe, expect, it } from 'vitest';
import { recentlyPlayed, backdropGame, backdropCycle, playedFromReport, syncState, waitingSummary } from './libraryhome.js';

const g = (id, extra = {}) => ({ id, name: id, ...extra });

describe('recentlyPlayed', () => {
  it('puts the game being played first, then the rest newest first, and leaves out the unplayed', () => {
    const games = [
      g('old', { lastPlayedAt: '2026-11-01T10:00:00Z' }),
      g('never'),
      g('now', { playingSince: '2026-11-20T09:00:00Z' }),
      g('recent', { lastPlayedAt: '2026-11-19T10:00:00Z' })
    ];
    expect(recentlyPlayed(games).map((r) => [r.game.id, r.playing])).toEqual([
      ['now', true],
      ['recent', false],
      ['old', false]
    ]);
  });

  it('takes the later of this device and the activity report, and says where only for another device', () => {
    const games = [g('hk', { lastPlayedAt: '2026-11-18T10:00:00Z' }), g('celeste', { lastPlayedAt: '2026-11-20T10:00:00Z' })];
    const played = {
      hk: { at: Date.parse('2026-11-19T10:00:00Z'), on: 'Steam Deck' },
      celeste: { at: Date.parse('2026-11-01T10:00:00Z'), on: 'Steam Deck' }
    };
    const [first, second] = recentlyPlayed(games, played);
    expect([first.game.id, first.on]).toEqual(['celeste', '']);
    expect([second.game.id, second.on]).toEqual(['hk', 'Steam Deck']);
  });

  it('counts a game played only on another device', () => {
    const r = recentlyPlayed([g('hades')], { hades: { at: 5, on: 'Steam Deck' } });
    expect(r).toEqual([{ game: g('hades'), playing: false, at: 5, on: 'Steam Deck' }]);
  });

  it('stops at the limit, and survives nothing at all', () => {
    const games = Array.from({ length: 6 }, (_, i) => g('g' + i, { lastPlayedAt: new Date(2026, 0, i + 1).toISOString() }));
    expect(recentlyPlayed(games, {}, 4)).toHaveLength(4);
    expect(recentlyPlayed(undefined)).toEqual([]);
    expect(recentlyPlayed([g('bad', { lastPlayedAt: 'not a date' })])).toEqual([]);
  });
});

describe('backdropGame', () => {
  it('is the game being played, else the last one played, else none', () => {
    expect(backdropGame([g('a', { lastPlayedAt: '2026-11-19T10:00:00Z' }), g('b', { playingSince: '2026-11-01T10:00:00Z' })]).id).toBe('b');
    expect(backdropGame([g('a', { lastPlayedAt: '2026-11-19T10:00:00Z' })]).id).toBe('a');
    expect(backdropGame([g('a')])).toBeNull();
  });
});

describe('playedFromReport', () => {
  it('keeps the games the report says were played, with where', () => {
    const report = { games: [{ gameId: 'a', lastPlayedAt: 10, lastPlayedOn: 'Steam Deck' }, { gameId: 'b', lastSnapshotAt: 4 }, { gameId: 'c', lastPlayedAt: 3 }] };
    expect(playedFromReport(report)).toEqual({ a: { at: 10, on: 'Steam Deck' }, c: { at: 3, on: '' } });
    expect(playedFromReport(null)).toEqual({});
  });
});

describe('syncState', () => {
  const deck = { status: 'online', lastSynced: '2026-11-20T10:00:00Z' };
  const laptop = { status: 'offline', lastSynced: '2026-11-17T10:00:00Z' };

  it('says paused before anything else', () => {
    expect(syncState({ peers: { deck }, activity: { x: { state: 'running' } }, paused: true }).kind).toBe('paused');
  });

  it('counts the syncs running', () => {
    const s = syncState({ peers: { deck }, activity: { x: { state: 'running' }, y: { state: 'running' }, z: { state: 'done' } } });
    expect([s.kind, s.running]).toEqual(['syncing', 2]);
  });

  it('tells apart no devices, none online, and synced', () => {
    expect(syncState({}).kind).toBe('alone');
    expect(syncState({ peers: { laptop } }).kind).toBe('offline');
    const s = syncState({ peers: { deck, laptop } });
    expect([s.kind, s.paired, s.online, s.lastAt]).toEqual(['synced', 2, 1, Date.parse(deck.lastSynced)]);
  });
});

describe('waitingSummary', () => {
  it('counts the list behind the bell by kind, in its order, and leaves out an update', () => {
    const items = [
      { id: 'conflict:a' },
      { id: 'conflict:b' },
      { id: 'missing:c' },
      { id: 'new:Hades,Celeste,Balatro' },
      { id: 'update:2.5.1' }
    ];
    expect(waitingSummary(items)).toEqual([
      { kind: 'conflict', n: 2 },
      { kind: 'missing', n: 1 },
      { kind: 'new', n: 3 }
    ]);
    expect(waitingSummary(undefined)).toEqual([]);
  });
});

describe('removing from Recently played', () => {
  const at = (iso) => Date.parse(iso);
  const games = [
    g('a', { lastPlayedAt: '2026-11-20T10:00:00Z' }),
    g('b', { lastPlayedAt: '2026-11-19T10:00:00Z' }),
    g('c', { lastPlayedAt: '2026-11-18T10:00:00Z' })
  ];

  it('leaves a removed play out, and the next one takes its place', () => {
    const hidden = { a: at('2026-11-20T10:00:00Z') };
    expect(recentlyPlayed(games, {}, 2, hidden).map((r) => r.game.id)).toEqual(['b', 'c']);
    expect(backdropGame(games, {}, hidden).id).toBe('b');
  });

  it('brings a game back when it is played again', () => {
    const hidden = { a: at('2026-11-19T00:00:00Z') };
    expect(recentlyPlayed(games, {}, 4, hidden).map((r) => r.game.id)).toEqual(['a', 'b', 'c']);
    const playing = [g('a', { playingSince: '2026-11-21T09:00:00Z' }), ...games.slice(1)];
    expect(recentlyPlayed(playing, {}, 4, { a: at('2026-11-20T10:00:00Z') })[0]).toMatchObject({ playing: true });
  });

  it('counts a play reported from another device the same way', () => {
    const played = { c: { at: '2026-11-21T08:00:00Z', on: 'Deck' } };
    expect(recentlyPlayed(games, played, 1, { c: at('2026-11-21T08:00:00Z') })[0].game.id).toBe('a');
  });
});

describe('backdropCycle', () => {
  const games = [g('a'), g('b', { coverExplicit: true }), g('c'), g('d')];

  it('starts on the game it is given and comes round through the rest, without explicit art', () => {
    expect(backdropCycle(games, g('c')).map((x) => x.id)).toEqual(['c', 'd', 'a']);
  });

  it('starts at the top when there is no game to start on, or it is not there', () => {
    expect(backdropCycle(games).map((x) => x.id)).toEqual(['a', 'c', 'd']);
    expect(backdropCycle(games, g('zz')).map((x) => x.id)).toEqual(['a', 'c', 'd']);
    expect(backdropCycle(null)).toEqual([]);
  });
});
