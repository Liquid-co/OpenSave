import { describe, expect, it } from 'vitest';
import { matchShortcut, PAGES, SHORTCUTS } from './shortcuts.js';
import { rank, score, snapshotEntries } from './palette.js';

const key = (k, mods = {}, target = { tagName: 'BODY' }) => ({ key: k, ctrlKey: false, metaKey: false, altKey: false, shiftKey: false, target, ...mods });

describe('matchShortcut', () => {
  it('knows the combinations, with Cmd standing in for Ctrl', () => {
    expect(matchShortcut(key('k', { ctrlKey: true }))).toBe('palette');
    expect(matchShortcut(key('K', { metaKey: true }))).toBe('palette');
    expect(matchShortcut(key('f', { ctrlKey: true }))).toBe('find');
    expect(matchShortcut(key(',', { ctrlKey: true }))).toBe('go:settings');
    expect(matchShortcut(key('1', { ctrlKey: true }))).toBe('go:home');
    expect(matchShortcut(key('4', { ctrlKey: true }))).toBe('go:activity');
  });

  it('leaves alone what it does not know, and Ctrl+Alt (AltGr on many keyboards)', () => {
    expect(matchShortcut(key('9', { ctrlKey: true }))).toBe(null);
    expect(matchShortcut(key('k'))).toBe(null);
    expect(matchShortcut(key('k', { ctrlKey: true, altKey: true }))).toBe(null);
  });

  it('works from inside a text field for combinations, but a typed "?" is just a question mark', () => {
    const field = { tagName: 'INPUT' };
    expect(matchShortcut(key('k', { ctrlKey: true }, field))).toBe('palette');
    expect(matchShortcut(key('?', {}, field))).toBe(null);
    expect(matchShortcut(key('?', {}, { tagName: 'DIV', isContentEditable: true }))).toBe(null);
    expect(matchShortcut(key('?'))).toBe('help');
  });

  it('lists a Ctrl+number for every page it handles', () => {
    for (let i = 0; i < PAGES.length; i++) {
      expect(SHORTCUTS.some(([keys]) => keys === `Ctrl ${i + 1}`)).toBe(true);
      expect(matchShortcut(key(String(i + 1), { ctrlKey: true }))).toBe(`go:${PAGES[i].id}`);
    }
  });
});

describe('palette ranking', () => {
  const entries = [
    { label: 'Go to Devices', idle: true },
    { label: 'Hades', weight: 1 },
    { label: 'Snapshot Hades', weight: -1 },
    { label: 'Sync Hades', weight: -1 },
    { label: 'Hollow Knight', weight: 1 },
    { label: 'Elden Ring', weight: 1, keywords: ['souls'] },
    { label: 'Sync all games', idle: true }
  ];
  const labels = (list) => list.map((e) => e.label);

  it('needs every typed word, matched at a word start or inside', () => {
    expect(labels(rank(entries, 'snap had'))).toEqual(['Snapshot Hades']);
    expect(labels(rank(entries, 'ring'))).toEqual(['Elden Ring']);
    expect(labels(rank(entries, 'souls'))).toEqual(['Elden Ring']);
    expect(rank(entries, 'zelda')).toEqual([]);
  });

  it('puts the game itself before actions on it', () => {
    expect(labels(rank(entries, 'hades'))).toEqual(['Hades', 'Sync Hades', 'Snapshot Hades']);
  });

  it('prefers a label that starts with what was typed', () => {
    expect(labels(rank(entries, 'h')).slice(0, 2)).toEqual(['Hades', 'Hollow Knight']);
  });

  it('shows the everyday entries when nothing is typed', () => {
    expect(labels(rank(entries, '  '))).toEqual(['Go to Devices', 'Sync all games']);
  });

  it('scores nothing typed as neutral and a miss as no match', () => {
    expect(score(entries[1], '')).toBe(0);
    expect(score(entries[1], 'xyz')).toBe(null);
  });
});

describe('snapshotEntries', () => {
  const games = [
    {
      id: 'hades',
      name: 'Hades',
      branches: {
        main: {
          snapshots: [
            { id: 's1', isSystemAuto: true, comment: 'Auto backup', note: '' },
            { id: 's2', isSystemAuto: false, comment: 'Before the final boss', note: '' },
            { id: 's3', isSystemAuto: true, comment: 'Before a sync replaced local files', note: 'good run, all keys' },
            { id: 's4', isSystemAuto: false, comment: 'Snapshot', note: '' }
          ]
        }
      }
    }
  ];

  it('offers the snapshots someone put words on, by those words', () => {
    const entries = snapshotEntries(games);
    expect(entries.map((e) => e.label)).toEqual(['Before the final boss — Hades', 'good run, all keys — Hades']);
    expect(entries[0]).toMatchObject({ gameId: 'hades', snapshotId: 's2', kind: 'Snapshot' });
    expect(rank(entries, 'boss').map((e) => e.snapshotId)).toEqual(['s2']);
    expect(rank(entries, 'keys hades').map((e) => e.snapshotId)).toEqual(['s3']);
  });
});
