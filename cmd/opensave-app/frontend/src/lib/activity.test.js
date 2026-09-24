import { describe, expect, it } from 'vitest';
import { asText, feedByDay, filterEntries, linkGames } from './activity.js';

const now = new Date(2026, 8, 24, 16, 0, 0);
const entry = (d, h, level, message) => ({ timestamp: new Date(2026, 8, d, h).toISOString(), level, message });

describe('filterEntries', () => {
  const log = [
    entry(24, 9, 'info', 'watching "D:\\Games\\Hades"'),
    entry(24, 10, 'warn', 'cloud: upload of Hades failed'),
    entry(24, 11, 'error', 'sync with Deck failed'),
    entry(24, 12, 'success', 'synced Celeste with Deck')
  ];
  const levels = (show) => filterEntries(log, { show }).map((e) => e.level);

  it('leaves out routine info in the highlights', () => {
    expect(levels('highlights')).toEqual(['warn', 'error', 'success']);
  });
  it('keeps only warnings and errors for problems', () => {
    expect(levels('problems')).toEqual(['warn', 'error']);
  });
  it('shows everything when asked', () => {
    expect(levels('all')).toHaveLength(4);
    expect(filterEntries(log)).toHaveLength(4);
    expect(filterEntries(null)).toEqual([]);
  });
  it('searches the message without regard to case', () => {
    expect(filterEntries(log, { query: 'DECK' })).toHaveLength(2);
    expect(filterEntries(log, { query: 'hades', show: 'problems' })).toHaveLength(1);
  });
});

describe('feedByDay', () => {
  it('puts the newest first and runs a day together', () => {
    const log = [entry(23, 9, 'info', 'a'), entry(24, 9, 'info', 'b'), entry(24, 15, 'info', 'c')];
    const g = feedByDay(log, now);
    expect(g.map((x) => [x.day, x.entries.map((e) => e.message)])).toEqual([
      ['Today', ['c', 'b']],
      ['Yesterday', ['a']]
    ]);
  });
});

describe('linkGames', () => {
  const games = [
    { id: 'hades', name: 'Hades' },
    { id: 'hades-2', name: 'Hades II' },
    { id: 'celeste', name: 'Celeste' },
    { id: 'x', name: 'X' }
  ];
  const linked = (msg) => linkGames(msg, games).filter((s) => s.gameId).map((s) => [s.text, s.gameId]);

  it('finds each game named in a message', () => {
    expect(linked('synced "Celeste" and Hades with Deck')).toEqual([
      ['Celeste', 'celeste'],
      ['Hades', 'hades']
    ]);
  });
  it('finds a game named by its ID, and shows its name', () => {
    expect(linked('auto-snapshot created for "hades-2"')).toEqual([['Hades II', 'hades-2']]);
    expect(linked('post-snapshot sync for celeste: no online peers')).toEqual([['Celeste', 'celeste']]);
  });
  it('prefers the longer name and ignores a name inside another word', () => {
    expect(linked('now tracking Hades II')).toEqual([['Hades II', 'hades-2']]);
    expect(linked('Hadesque is not a game')).toEqual([]);
  });
  it('leaves a folder in a path alone, even one named after the game', () => {
    expect(linked('watching "D:\\Games\\Hades\\save" and /home/me/celeste/x')).toEqual([]);
  });
  it('ignores one-letter names, which would match everywhere', () => {
    expect(linked('X marks the spot')).toEqual([]);
  });
  it('keeps the rest of the message exactly', () => {
    const msg = 'restored "Hades" at D:\\Games';
    expect(
      linkGames(msg, games)
        .map((s) => s.text)
        .join('')
    ).toBe(msg);
    expect(linkGames('', games)).toEqual([{ text: '' }]);
  });
});

describe('asText', () => {
  it('writes one line per entry with its time and level', () => {
    const t = asText([{ timestamp: '2026-09-24T10:00:00Z', level: 'warn', message: 'cloud: failed' }]);
    expect(t).toBe('2026-09-24T10:00:00Z WARN    cloud: failed');
  });
});
