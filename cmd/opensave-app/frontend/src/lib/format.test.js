import { describe, expect, it } from 'vitest';
import { fmtSize, fmtTime, plural } from './format.js';

describe('fmtSize', () => {
  it('shows KB below a megabyte and MB from one', () => {
    expect(fmtSize(0)).toBe('0.0 KB');
    expect(fmtSize(1536)).toBe('1.5 KB');
    expect(fmtSize(1048575)).toBe('1024.0 KB');
    expect(fmtSize(1048576)).toBe('1.0 MB');
    expect(fmtSize(5 * 1048576 + 104858)).toBe('5.1 MB');
    expect(fmtSize(1073741823)).toBe('1024.0 MB');
    expect(fmtSize(10215702528)).toBe('9.5 GB');
  });
});

describe('fmtTime', () => {
  it('is a dash when there is no time', () => {
    expect(fmtTime('')).toBe('—');
    expect(fmtTime(null)).toBe('—');
  });
  it('formats a real time in the locale', () => {
    const t = '2026-09-24T07:43:24.176Z';
    expect(fmtTime(t)).toBe(new Date(t).toLocaleString());
  });
});

describe('plural', () => {
  it('uses the singular only for one', () => {
    expect(plural(1, 'game')).toBe('1 game');
    expect(plural(0, 'game')).toBe('0 games');
    expect(plural(2, 'copy', 'copies')).toBe('2 copies');
  });
});
