import { describe, it, expect } from 'vitest';
import { LOOKS, FAMILIES, lookFor, pick, hasBoth, lookVars } from './themes.js';
import { contrast } from './appearance.js';

describe('every look', () => {
  // Text is read on the page, on panels and on what is pointed at: each
  // shade of it keeps WCAG's 4.5:1 for ordinary text on all of them.
  it('keeps every shade of text readable on every surface it sits on', () => {
    for (const [id, l] of Object.entries(LOOKS)) {
      for (const surface of ['bg', 'sidebar', 'raised', 'hover']) {
        for (const ink of ['text', 'textDim', 'textFaint']) {
          const ratio = contrast(l[ink], l[surface]);
          expect(ratio, `${id}: ${ink} on ${surface} is ${ratio.toFixed(2)}:1`).toBeGreaterThanOrEqual(4.5);
        }
      }
    }
  });
  it('is the kind it says, by its background', () => {
    for (const [id, l] of Object.entries(LOOKS)) {
      const dark = contrast(l.bg, '#000000') < contrast(l.bg, '#ffffff');
      expect(dark ? 'dark' : 'light', id).toBe(l.kind);
    }
  });
  it('sets every surface and text variable', () => {
    for (const id of Object.keys(LOOKS)) {
      const vars = lookVars(id);
      for (const name of ['--bg', '--bg-sidebar', '--bg-raised', '--bg-hover', '--bg-active', '--border', '--border-strong', '--text', '--text-dim', '--text-faint', '--glass']) {
        expect(vars[name], `${id} ${name}`).toBeTruthy();
      }
    }
  });
  it('is used by a family, and no two themes look alike', () => {
    const used = Object.values(FAMILIES).flatMap((f) => [f.dark, f.light].filter(Boolean));
    expect(new Set(used)).toEqual(new Set(Object.keys(LOOKS)));
    const accents = Object.values(FAMILIES).map((f) => LOOKS[f.dark ?? f.light].accent);
    expect(new Set(accents).size).toBe(accents.length);
  });
});

describe('families', () => {
  it('name only looks that exist, of the kind they are filed under', () => {
    for (const [id, f] of Object.entries(FAMILIES)) {
      if (f.dark) expect(LOOKS[f.dark].kind, id).toBe('dark');
      if (f.light) expect(LOOKS[f.light].kind, id).toBe('light');
    }
  });
});

describe('lookFor', () => {
  it('picks the kind asked for where a family has both, and follows the system when asked', () => {
    expect(lookFor('opensave', 'dark')).toBe('night');
    expect(lookFor('opensave', 'light')).toBe('daylight');
    expect(lookFor('frost', 'system', false)).toBe('frostDay');
    expect(lookFor('frost', 'system', true)).toBe('frost');
  });
  it("keeps a family's only look whatever is asked", () => {
    expect(lookFor('blackout', 'light')).toBe('blackout');
    expect(lookFor('abyss', 'system', false)).toBe('abyss');
  });
  it("falls back to the app's own for a family it does not know", () => {
    expect(lookFor('gone', 'dark')).toBe('night');
  });
});

describe('pick', () => {
  it('takes the family and its own accent, and leaves light or dark as chosen', () => {
    expect(pick('campfire', { family: 'opensave', theme: 'system', accent: 'green' })).toMatchObject({ family: 'campfire', theme: 'system', accent: 'theme' });
    expect(hasBoth('arcade')).toBe(false);
    expect(hasBoth('handheld')).toBe(true);
  });
});
