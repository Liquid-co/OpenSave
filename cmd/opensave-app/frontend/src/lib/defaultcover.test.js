import { describe, expect, it } from 'vitest';
import { coverHue } from './defaultcover.js';

describe('coverHue', () => {
  it('gives a hue in range', () => {
    for (const name of ['', 'test', 'conflict-demo', 'red-dead-redemption-2', 'Ghost of Tsushima']) {
      const h = coverHue(name);
      expect(Number.isInteger(h)).toBe(true);
      expect(h).toBeGreaterThanOrEqual(0);
      expect(h).toBeLessThan(360);
    }
  });

  it('is the same for the same name, whatever its case or edges', () => {
    expect(coverHue('Hollow Knight')).toBe(coverHue('  hollow knight '));
  });

  it('tells similar names apart', () => {
    expect(coverHue('test')).not.toBe(coverHue('test2'));
  });

  it('copes with nothing', () => {
    expect(coverHue(undefined)).toBe(coverHue(''));
  });
});
