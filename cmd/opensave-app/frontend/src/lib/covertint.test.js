import { describe, it, expect } from 'vitest';
import { tintFromPixels, tintVars } from './covertint.js';
import { hexToHsv, contrast } from './appearance.js';

const pixels = (...list) => {
  const out = [];
  for (const [rgb, n] of list) for (let i = 0; i < n; i++) out.push(...rgb, 255);
  return new Uint8ClampedArray(out);
};

describe('tintFromPixels', () => {
  it('takes the colourful part, not the average', () => {
    // Mostly near-black, a little strong gold: gold, not brown.
    const hex = tintFromPixels(pixels([[12, 10, 8], 80], [[220, 170, 60], 20]));
    const { h } = hexToHsv(hex);
    expect(h).toBeGreaterThan(30);
    expect(h).toBeLessThan(50);
  });
  it('averages hues round the circle', () => {
    const { h } = hexToHsv(tintFromPixels(pixels([[230, 30, 60], 50], [[230, 60, 30], 50])));
    expect(h < 15 || h > 345).toBe(true);
  });
  it('gives nothing for a cover with next to no colour', () => {
    expect(tintFromPixels(pixels([[128, 128, 128], 90], [[250, 250, 250], 10]))).toBeNull();
    expect(tintFromPixels(new Uint8ClampedArray())).toBeNull();
  });
});

describe('tintVars', () => {
  it('makes a fill its label reads on', () => {
    const v = tintVars('#e2bd62');
    expect(contrast(v['--game-fill'], v['--game-on-fill'])).toBeGreaterThanOrEqual(4.5);
  });
});
