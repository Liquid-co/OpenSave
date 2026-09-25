import { describe, expect, it } from 'vitest';
import { DEFAULT_VIEW, filterRows, gridColumns, loadView, offeredFilters, sanitizeView, saveView } from './libraryview.js';

const memoryStorage = (initial = {}) => {
  const data = { ...initial };
  return {
    data,
    getItem: (k) => (k in data ? data[k] : null),
    setItem: (k, v) => {
      data[k] = String(v);
    }
  };
};

describe('sanitizeView', () => {
  it('keeps valid choices', () => {
    expect(sanitizeView({ cover: 'tall', columns: 5, size: 'large', sort: 'recent' })).toEqual({
      cover: 'tall',
      columns: 5,
      size: 'large',
      sort: 'recent',
      reverse: false,
      overview: true
    });
  });
  it('shows the overview unless it was turned off', () => {
    expect(sanitizeView({ cover: 'tall' }).overview).toBe(true);
    expect(sanitizeView({ overview: 'no' }).overview).toBe(true);
    expect(sanitizeView({ overview: false }).overview).toBe(false);
  });
  it('replaces anything unknown with the default, field by field', () => {
    expect(sanitizeView({ cover: 'round', columns: 12, size: 'huge', sort: 'rating' })).toEqual(DEFAULT_VIEW);
    expect(sanitizeView({ cover: 'tall', columns: '4' })).toMatchObject({ cover: 'tall', columns: 4 });
    expect(sanitizeView(null)).toEqual(DEFAULT_VIEW);
  });
});

describe('gridColumns', () => {
  it('fixes the count when one is chosen', () => {
    expect(gridColumns({ columns: 4 })).toBe('repeat(4, minmax(0, 1fr))');
  });
  it('fits as many as the window allows otherwise, narrower for tall art and small tiles', () => {
    const wide = gridColumns({ cover: 'wide', columns: 'auto', size: 'medium' });
    const tall = gridColumns({ cover: 'tall', columns: 'auto', size: 'medium' });
    const tallSmall = gridColumns({ cover: 'tall', columns: 'auto', size: 'small' });
    const width = (t) => Number(/minmax\((\d+)px/.exec(t)[1]);
    expect(width(tall)).toBeLessThan(width(wide));
    expect(width(tallSmall)).toBeLessThan(width(tall));
  });
});

describe('loadView and saveView', () => {
  it('round-trips a view', () => {
    const s = memoryStorage();
    saveView({ cover: 'tall', columns: 6, size: 'small', sort: 'size', reverse: true, overview: false }, s);
    expect(loadView(s)).toEqual({ cover: 'tall', columns: 6, size: 'small', sort: 'size', reverse: true, overview: false });
  });
  it('carries over the order kept before the rest of the view existed', () => {
    expect(loadView(memoryStorage({ 'opensave.librarySort': 'recent' })).sort).toBe('recent');
  });
  it('falls back to the default when storage is unreadable or garbled', () => {
    expect(loadView(memoryStorage({ 'opensave.libraryView': '{not json' }))).toEqual(DEFAULT_VIEW);
    const broken = { getItem: () => { throw new Error('denied'); }, setItem: () => { throw new Error('denied'); } };
    expect(loadView(broken)).toEqual(DEFAULT_VIEW);
    expect(() => saveView(DEFAULT_VIEW, broken)).not.toThrow();
  });
});

describe('filters', () => {
  const row = (name, state) => ({ game: { name }, status: { state } });
  const rows = [
    row('Hades', 'synced'),
    row('Celeste', 'conflict'),
    row('Elden Ring', 'syncing'),
    row('Balatro', 'synced'),
    row('Hollow Knight', 'error')
  ];

  it('searches names without regard to case', () => {
    expect(filterRows(rows, { query: 'EN' }).map((r) => r.game.name)).toEqual(['Elden Ring']);
    expect(filterRows(rows, { query: '  ' })).toHaveLength(5);
  });

  it('gathers a conflict and a failed sync under needing attention', () => {
    expect(filterRows(rows, { status: 'attention' }).map((r) => r.game.name)).toEqual(['Celeste', 'Hollow Knight']);
  });

  it('applies the search and the status together', () => {
    expect(filterRows(rows, { query: 'a', status: 'synced' }).map((r) => r.game.name)).toEqual(['Hades', 'Balatro']);
  });

  it('offers only filters that would narrow the list, with their counts', () => {
    expect(offeredFilters(rows).map((f) => [f.id, f.count])).toEqual([
      ['all', 5],
      ['attention', 2],
      ['syncing', 1],
      ['synced', 2]
    ]);
    // With nothing paired, what is worth finding is a game not yet saved at all.
    expect(offeredFilters([row('A', 'local'), row('B', 'empty'), row('C', 'paused')]).map((f) => [f.id, f.count])).toEqual([
      ['all', 3],
      ['paused', 1],
      ['empty', 1]
    ]);
    // Everything synced: a "Synced" filter would show exactly what All does.
    expect(offeredFilters([row('A', 'synced'), row('B', 'synced')]).map((f) => f.id)).toEqual(['all']);
  });
});
