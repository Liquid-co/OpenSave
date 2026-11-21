import { describe, expect, it } from 'vitest';
import { sanitizeHidden, loadHidden, saveHidden } from './recenthidden.js';

const memoryStorage = () => {
  const m = new Map();
  return { getItem: (k) => (m.has(k) ? m.get(k) : null), setItem: (k, v) => m.set(k, String(v)) };
};

describe('recent hidden', () => {
  it('keeps only well-formed entries', () => {
    expect(sanitizeHidden({ a: 5, b: 'x', c: -1, d: NaN, '': 3 })).toEqual({ a: 5 });
    expect(sanitizeHidden([1, 2])).toEqual({});
    expect(sanitizeHidden(null)).toEqual({});
  });

  it('keeps the newest two hundred', () => {
    const many = Object.fromEntries(Array.from({ length: 250 }, (_, i) => ['g' + i, i + 1]));
    const kept = sanitizeHidden(many);
    expect(Object.keys(kept)).toHaveLength(200);
    expect(kept.g249).toBe(250);
    expect(kept.g0).toBeUndefined();
  });

  it('round-trips, and reads garbage as nothing', () => {
    const s = memoryStorage();
    saveHidden({ a: 10 }, s);
    expect(loadHidden(s)).toEqual({ a: 10 });
    s.setItem('opensave.recentHidden', '{not json');
    expect(loadHidden(s)).toEqual({});
    expect(loadHidden({ getItem: () => { throw new Error('blocked'); } })).toEqual({});
  });
});
