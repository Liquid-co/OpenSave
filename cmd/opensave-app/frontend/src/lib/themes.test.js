import { describe, it, expect } from 'vitest';
import { LOOKS, FAMILIES, THEMES_BUILT_IN, lookFor, isCurrent, pick, hasBoth, lookVars } from './themes.js';
import { contrast } from './appearance.js';

describe('every look', () => {
  // Text is read on the page, on panels and on what is pointed at: each
  // shade of it keeps WCAG's 4.5:1 for ordinary text on all three.
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
});

describe('families', () => {
  it('name only looks that exist, of the kind they are filed under', () => {
    for (const [id, f] of Object.entries(FAMILIES)) {
      if (f.dark) expect(LOOKS[f.dark].kind, id).toBe('dark');
      if (f.light) expect(LOOKS[f.light].kind, id).toBe('light');
    }
    for (const t of THEMES_BUILT_IN) {
      expect(FAMILIES[t.family], t.id).toBeTruthy();
      if (t.look) expect(Object.values(FAMILIES[t.family])).toContain(t.look);
    }
  });
});

describe('lookFor', () => {
  it('picks the kind asked for where a family has both, and follows the system when asked', () => {
    expect(lookFor('opensave', 'dark')).toBe('night');
    expect(lookFor('opensave', 'light')).toBe('daylight');
    expect(lookFor('catppuccin', 'system', false)).toBe('latte');
    expect(lookFor('catppuccin', 'system', true)).toBe('mocha');
  });
  it("keeps a family's only look whatever is asked", () => {
    expect(lookFor('oled', 'light')).toBe('oled');
    expect(lookFor('tokyo', 'system', false)).toBe('tokyo');
  });
  it("falls back to the app's own for a family it does not know", () => {
    expect(lookFor('gone', 'dark')).toBe('night');
  });
});

describe('the cards', () => {
  const card = (id) => THEMES_BUILT_IN.find((t) => t.id === id);
  it('shows as picked the one in use — Night or Daylight by which look is on', () => {
    expect(isCurrent(card('night'), 'opensave', 'night')).toBe(true);
    expect(isCurrent(card('daylight'), 'opensave', 'night')).toBe(false);
    expect(isCurrent(card('daylight'), 'opensave', 'daylight')).toBe(true);
    expect(isCurrent(card('mocha'), 'catppuccin', 'latte')).toBe(true);
  });
  it("picks the family and its accent; Night and Daylight their own kind; Catppuccin keeps the kind chosen", () => {
    const now = { family: 'catppuccin', theme: 'system', accent: 'green' };
    expect(pick(card('daylight'), now)).toMatchObject({ family: 'opensave', theme: 'light', accent: 'theme' });
    expect(pick(card('night'), now)).toMatchObject({ family: 'opensave', theme: 'dark' });
    expect(pick(card('mocha'), { ...now, family: 'opensave' })).toMatchObject({ family: 'catppuccin', theme: 'system' });
    expect(pick(card('oled'), now)).toMatchObject({ family: 'oled', theme: 'system' });
    expect(hasBoth('oled')).toBe(false);
  });
});
