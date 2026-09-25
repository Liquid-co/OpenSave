import { describe, expect, it } from 'vitest';
import {
  ACCENTS,
  DEFAULT_APPEARANCE,
  accentVars,
  applyAppearance,
  hexToRgb,
  lighten,
  loadAppearance,
  resolvedTheme,
  sanitizeAppearance,
  saveAppearance
} from './appearance.js';

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

// Just enough of an element for applyAppearance.
const fakeRoot = () => {
  const props = {};
  return {
    dataset: {},
    props,
    style: {
      zoom: '',
      setProperty: (k, v) => {
        props[k] = v;
      }
    }
  };
};

// WCAG relative luminance and contrast, to hold the accents to their promise.
const luminance = (hex) => {
  const [r, g, b] = hexToRgb(hex).map((c) => {
    const s = c / 255;
    return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
};
const contrast = (a, b) => {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (hi + 0.05) / (lo + 0.05);
};

describe('sanitizeAppearance', () => {
  it('keeps valid choices and replaces the rest, field by field', () => {
    expect(sanitizeAppearance({ theme: 'light', accent: 'teal', scale: 1.25, motion: false, controller: 'on' })).toEqual({
      theme: 'light',
      accent: 'teal',
      scale: 1.25,
      motion: false,
      controller: 'on'
    });
    expect(sanitizeAppearance({ controller: 'sometimes' }).controller).toBe('auto');
    expect(sanitizeAppearance({ theme: 'sepia', accent: 'teal', scale: 3 })).toEqual({ ...DEFAULT_APPEARANCE, accent: 'teal' });
    expect(sanitizeAppearance({ scale: '1.1' }).scale).toBe(1.1);
    expect(sanitizeAppearance(null)).toEqual(DEFAULT_APPEARANCE);
  });

  it('keeps animations on unless they were switched off', () => {
    // Saved before the choice existed: on, as it was then.
    expect(sanitizeAppearance({ theme: 'light', accent: 'teal', scale: 1 }).motion).toBe(true);
    expect(sanitizeAppearance({ motion: 'no' }).motion).toBe(true);
    expect(sanitizeAppearance({ motion: false }).motion).toBe(false);
  });
});

describe('loadAppearance and saveAppearance', () => {
  it('round-trips, and falls back to the default when storage is garbled or refuses', () => {
    const s = memoryStorage();
    saveAppearance({ theme: 'system', accent: 'rose', scale: 0.9, motion: false, controller: 'off' }, s);
    expect(loadAppearance(s)).toEqual({ theme: 'system', accent: 'rose', scale: 0.9, motion: false, controller: 'off' });
    expect(loadAppearance(memoryStorage({ 'opensave.appearance': '{nope' }))).toEqual(DEFAULT_APPEARANCE);
    const broken = { getItem: () => { throw new Error('denied'); }, setItem: () => { throw new Error('denied'); } };
    expect(loadAppearance(broken)).toEqual(DEFAULT_APPEARANCE);
    expect(() => saveAppearance(DEFAULT_APPEARANCE, broken)).not.toThrow();
  });
});

describe('resolvedTheme', () => {
  it('follows the system only when asked to', () => {
    expect(resolvedTheme('system', true)).toBe('dark');
    expect(resolvedTheme('system', false)).toBe('light');
    expect(resolvedTheme('light', true)).toBe('light');
    expect(resolvedTheme('dark', false)).toBe('dark');
  });
});

describe('accent colours', () => {
  it('derives the hover shade and the RGB triple the tints use', () => {
    expect(lighten('#000000', 0.5)).toBe('#808080');
    expect(accentVars('violet')).toEqual({ '--accent': '#8a63f4', '--accent-hover': lighten('#8a63f4', 0.12), '--accent-rgb': '138, 99, 244' });
    expect(accentVars('nonsense')['--accent']).toBe(ACCENTS.violet.hex);
  });

  it('keeps white button text readable on every accent', () => {
    for (const [id, a] of Object.entries(ACCENTS)) {
      expect(contrast(a.hex, '#ffffff'), id).toBeGreaterThanOrEqual(3);
    }
  });
});

describe('applyAppearance', () => {
  it('lets things move only when animations are on and the system is not asking for less motion', () => {
    const root = fakeRoot();
    applyAppearance(DEFAULT_APPEARANCE, { root });
    expect(root.dataset.motion).toBe('on');
    applyAppearance({ ...DEFAULT_APPEARANCE, motion: false }, { root });
    expect(root.dataset.motion).toBe('off');
    applyAppearance(DEFAULT_APPEARANCE, { root, reduceMotion: true });
    expect(root.dataset.motion).toBe('off');
  });

  it('sets the theme, the accent and the scale on the page, and the window behind it', () => {
    const root = fakeRoot();
    let windowBg = null;
    applyAppearance({ theme: 'system', accent: 'teal', scale: 1.1 }, { root, prefersDark: false, setWindowBackground: (...rgb) => (windowBg = rgb) });
    expect(root.dataset.theme).toBe('light');
    expect(root.props['--accent']).toBe(ACCENTS.teal.hex);
    expect(root.props['--accent-rgb']).toBe(hexToRgb(ACCENTS.teal.hex).join(', '));
    expect(root.style.zoom).toBe('1.1');
    expect(windowBg).toEqual([244, 244, 247]);

    applyAppearance(DEFAULT_APPEARANCE, { root, prefersDark: false });
    expect(root.dataset.theme).toBe('dark');
    expect(root.style.zoom).toBe('');
  });
});
