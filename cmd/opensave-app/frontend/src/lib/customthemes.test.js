import { describe, it, expect } from 'vitest';
import {
  sanitizeTheme,
  themeFromLook,
  deriveLook,
  readability,
  fixCheck,
  fixLightness,
  uniqueName,
  exportTheme,
  importTheme,
  READABLE,
  MAX_FILE_BYTES,
  FONTS
} from './customthemes.js';
import { contrast, hexToHsl } from './colour.js';

const gold = {
  id: 'midnightgold',
  name: 'Midnight Gold',
  colours: { background: '#0f0d0a', panels: '#1a1712', text: '#f1e9da', quiet: '#7d7262', accent: '#d8b052', onAccent: '' },
  roundness: 1.4,
  borders: 1,
  shadows: 1,
  font: 'display'
};

describe('sanitizeTheme', () => {
  it('needs the five colours a theme is made of, and nothing else', () => {
    expect(sanitizeTheme({ colours: { background: '#000' } })).toBe(null);
    expect(sanitizeTheme(null)).toBe(null);
    const t = sanitizeTheme({ ...gold, colours: { ...gold.colours, success: 'nonsense' }, script: 'alert(1)' });
    expect(t.colours.success).toBe('');
    expect(t).not.toHaveProperty('script');
  });
  it('keeps shapes in range, names tidy, and a font name from closing its quote', () => {
    const t = sanitizeTheme({ ...gold, roundness: 9, borders: -1, shadows: 'x', name: '  <b>Gold</b>  ', font: 'custom', customFont: "Fira'; } body { color: red" });
    expect([t.roundness, t.borders, t.shadows]).toEqual([1.75, 0, 1]);
    expect(t.name).toBe('bGold/b');
    expect(t.customFont).not.toMatch(/['";{}]/);
  });
});

describe('deriveLook', () => {
  it("works out the kind, the shades and the borders from the colours picked", () => {
    const l = deriveLook(gold);
    expect(l.kind).toBe('dark');
    expect(l.raised).toBe('#1a1712');
    expect(l.font).toBe(FONTS.display);
    expect(deriveLook({ ...gold, borders: 0 }).border).toBe('#1a1712');
    expect(deriveLook(themeFromLook('daylight')).kind).toBe('light');
  });
  it('keeps the faintest text readable on every surface, whatever was picked', () => {
    for (const t of [gold, themeFromLook('handheld'), themeFromLook('blackout')]) {
      const l = deriveLook(t);
      for (const s of [l.bg, l.sidebar, l.raised, l.hover]) expect(contrast(l.textFaint, s)).toBeGreaterThanOrEqual(READABLE);
    }
  });
  it('picks the text on the accent unless it was set', () => {
    expect(deriveLook(gold).onAccent).toBe('#141416');
    expect(deriveLook({ ...gold, colours: { ...gold.colours, onAccent: '#ffffff' } }).onAccent).toBe('#ffffff');
  });
});

describe('readability and Fix', () => {
  it('says which pairs fail, and Fix makes them pass by lightness alone', () => {
    const checks = readability(gold);
    const quiet = checks.find((c) => c.id === 'quietOnPanels');
    expect(quiet.ok).toBe(false); // the board's own example, 3.7:1
    const fixed = fixCheck(gold, 'quietOnPanels');
    expect(readability(fixed).find((c) => c.id === 'quietOnPanels').ok).toBe(true);
    expect(Math.abs(hexToHsl(fixed.colours.quiet).h - hexToHsl(gold.colours.quiet).h)).toBeLessThan(4);
    expect(fixed.colours.background).toBe(gold.colours.background);
  });
  it('fixes the text on an accent, or the accent when no ink can read on it', () => {
    const grey = { ...gold, colours: { ...gold.colours, accent: '#777777', onAccent: '#888888' } };
    const fixed = fixCheck(grey, 'textOnAccent');
    expect(readability(fixed).find((c) => c.id === 'textOnAccent').ok).toBe(true);
  });
  it('leaves a colour alone that already reads', () => {
    expect(fixLightness('#ffffff', '#000000')).toBe('#ffffff');
  });
});

describe('uniqueName', () => {
  it("never takes a built-in theme's name bare, nor another of yours", () => {
    expect(uniqueName('Frost', [], ['Frost', 'Moss'])).toBe('Frost (yours)');
    expect(uniqueName('Gold', ['gold', 'Gold 2'])).toBe('Gold 3');
    expect(uniqueName('  ')).toBe('My theme');
  });
});

describe('theme files', () => {
  it('round-trip a theme, as a new one', () => {
    const back = importTheme(exportTheme(gold));
    expect(back.theme).toMatchObject({ name: 'Midnight Gold', colours: sanitizeTheme(gold).colours, roundness: 1.4, font: 'display' });
    expect(back.theme.id).not.toBe('midnightgold');
    expect(exportTheme(gold)).not.toContain('midnightgold');
  });
  it('refuse what is not one, a newer one, broken colours, or too much', () => {
    expect(importTheme('{nope').error).toBe('themeEditor.import.notATheme');
    expect(importTheme(JSON.stringify({ format: 'something', version: 1 })).error).toBe('themeEditor.import.notATheme');
    expect(importTheme(JSON.stringify({ ...JSON.parse(exportTheme(gold)), version: 99 })).error).toBe('themeEditor.import.newerVersion');
    expect(importTheme(JSON.stringify({ ...JSON.parse(exportTheme(gold)), colours: { background: 'red' } })).error).toBe('themeEditor.import.badColours');
    expect(importTheme('x'.repeat(MAX_FILE_BYTES + 1)).error).toBe('themeEditor.import.tooBig');
  });
});
