import { describe, expect, it } from 'vitest';
import {
  ACCENTS,
  DEFAULT_APPEARANCE,
  MAX_CUSTOM_ACCENTS,
  accentVars,
  accentFill,
  applyAppearance,
  hexToHsv,
  hexToRgb,
  hsvToHex,
  lighten,
  loadAppearance,
  normalizeHex,
  resolvedTheme,
  sanitizeAppearance,
  saveAppearance,
  withCustomAccent,
  withoutCustomAccent
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
      removeProperty: (k) => {
        delete props[k];
      },
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
      family: 'opensave',
      theme: 'light',
      accent: 'teal',
      customAccents: [],
      scale: 1.25,
      motion: false,
      controller: 'on',
      backdrop: 'cover',
      backdropCycle: true,
      coverColours: true,
      customThemes: []
    });
    expect(sanitizeAppearance({ controller: 'sometimes' }).controller).toBe('auto');
    // The accent glow is gone; whoever chose it gets no picture, not a cover.
    expect(sanitizeAppearance({ backdrop: 'accent' }).backdrop).toBe('off');
    expect(sanitizeAppearance({ backdrop: 'off' }).backdrop).toBe('off');
    expect(sanitizeAppearance({ backdropCycle: false }).backdropCycle).toBe(false);
    expect(sanitizeAppearance({ backdropCycle: 'no' }).backdropCycle).toBe(true);
    expect(sanitizeAppearance({ coverColours: false }).coverColours).toBe(false);
    expect(sanitizeAppearance({ coverColours: 'yes' }).coverColours).toBe(true);
    expect(sanitizeAppearance({ backdrop: 'video' }).backdrop).toBe('cover');
    expect(sanitizeAppearance({ theme: 'sepia', accent: 'teal', scale: 3 })).toEqual({ ...DEFAULT_APPEARANCE, accent: 'teal' });
    expect(sanitizeAppearance({ scale: '1.1' }).scale).toBe(1.1);
    expect(sanitizeAppearance(null)).toEqual(DEFAULT_APPEARANCE);
  });

  it("keeps a theme it knows and its own accent, and starts anyone else on OpenSave's", () => {
    expect(sanitizeAppearance({ family: 'frost', accent: 'theme' })).toMatchObject({ family: 'frost', accent: 'theme' });
    expect(sanitizeAppearance({ family: 'solarized' }).family).toBe('opensave');
    // Saved before themes: the same look as before.
    expect(sanitizeAppearance({ theme: 'light', accent: 'teal' })).toMatchObject({ family: 'opensave', theme: 'light', accent: 'teal' });
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
    expect(loadAppearance(s)).toEqual({ family: 'opensave', theme: 'system', accent: 'rose', customAccents: [], scale: 0.9, motion: false, controller: 'off', backdrop: 'cover', backdropCycle: true, coverColours: true, customThemes: [] });
    // A picked colour, and the ones kept beside the presets, come back too.
    saveAppearance({ ...DEFAULT_APPEARANCE, accent: '#12AB34', customAccents: ['#ff0000', '#12ab34'] }, s);
    expect(loadAppearance(s)).toMatchObject({ accent: '#12ab34', customAccents: ['#ff0000', '#12ab34'] });
    expect(loadAppearance(memoryStorage({ 'opensave.appearance': '{nope' }))).toEqual(DEFAULT_APPEARANCE);
    const broken = { getItem: () => { throw new Error('denied'); }, setItem: () => { throw new Error('denied'); } };
    expect(loadAppearance(broken)).toEqual(DEFAULT_APPEARANCE);
    expect(() => saveAppearance(DEFAULT_APPEARANCE, broken)).not.toThrow();
  });
});

describe('picked accent colours', () => {
  it('takes any colour as an accent, and always shows the one in use', () => {
    const a = sanitizeAppearance({ accent: '#ABC' });
    expect(a.accent).toBe('#aabbcc');
    expect(a.customAccents).toEqual(['#aabbcc']);
    expect(accentVars(a.accent)['--accent']).toBe('#aabbcc');
    expect(sanitizeAppearance({ accent: 'not a colour' }).accent).toBe(DEFAULT_APPEARANCE.accent);
    expect(sanitizeAppearance({ customAccents: ['#ff0000', 'nope', '#FF0000', 7] }).customAccents).toEqual(['#ff0000']);
  });

  it('keeps slate, once a preset, as the colour it was', () => {
    const a = sanitizeAppearance({ accent: 'slate' });
    expect(a.accent).toBe('#64748b');
    expect(a.customAccents).toEqual(['#64748b']);
    expect('slate' in ACCENTS).toBe(false);
  });

  it('adds a picked colour newest last, uses it, and keeps at most a row of them', () => {
    let v = withCustomAccent(DEFAULT_APPEARANCE, '#ff0000');
    expect(v.accent).toBe('#ff0000');
    expect(v.customAccents).toEqual(['#ff0000']);
    v = withCustomAccent(v, '#00ff00');
    v = withCustomAccent(v, '#ff0000'); // again: moved to the end, not doubled
    expect(v.customAccents).toEqual(['#00ff00', '#ff0000']);
    for (let i = 0; i < MAX_CUSTOM_ACCENTS + 2; i++) v = withCustomAccent(v, hsvToHex(i * 30, 1, 1));
    expect(v.customAccents).toHaveLength(MAX_CUSTOM_ACCENTS);
    expect(v.customAccents.at(-1)).toBe(v.accent);
    expect(withCustomAccent(DEFAULT_APPEARANCE, 'nope').accent).toBe(DEFAULT_APPEARANCE.accent);
  });

  it('removes a picked colour, going back to the default if it was in use', () => {
    const v = withCustomAccent(withCustomAccent(DEFAULT_APPEARANCE, '#ff0000'), '#00ff00');
    expect(withoutCustomAccent(v, '#00ff00')).toMatchObject({ accent: DEFAULT_APPEARANCE.accent, customAccents: ['#ff0000'] });
    expect(withoutCustomAccent(v, '#ff0000')).toMatchObject({ accent: '#00ff00', customAccents: ['#00ff00'] });
  });

  it('converts between the wheel and hex', () => {
    expect(hsvToHex(0, 1, 1)).toBe('#ff0000');
    expect(hsvToHex(120, 1, 1)).toBe('#00ff00');
    expect(hsvToHex(240, 1, 0.5)).toBe('#000080');
    expect(hsvToHex(0, 0, 1)).toBe('#ffffff');
    for (const hex of ['#8a63f4', '#e11d48', '#0d9488', '#facc15', '#000000', '#ffffff']) {
      const { h, s, v } = hexToHsv(hex);
      expect(hsvToHex(h, s, v), hex).toBe(hex);
    }
    expect(normalizeHex(' 0F0 ')).toBe('#00ff00');
    expect(normalizeHex('#12345')).toBe(null);
  });

  it('keeps any picked colour readable, as the presets are', () => {
    for (let h = 0; h < 360; h += 20) {
      for (const [s, v] of [[1, 1], [0.3, 1], [1, 0.4], [0, 0.5]]) {
        const hex = hsvToHex(h, s, v);
        for (const theme of ['dark', 'light']) {
          const vars = accentVars(hex, theme);
          expect(contrast(hex, vars['--on-accent']), `${hex} button text`).toBeGreaterThanOrEqual(3);
          expect(contrast(vars['--accent-text'], theme === 'dark' ? '#17171a' : '#ffffff'), `${hex} ${theme}`).toBeGreaterThanOrEqual(3);
        }
      }
    }
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
    expect(accentVars('violet')).toMatchObject({ '--accent': '#8a63f4', '--accent-hover': lighten('#8a63f4', 0.12), '--accent-rgb': '138, 99, 244' });
    expect(accentVars('nonsense')['--accent']).toBe(ACCENTS.violet.hex);
  });

  it('keeps the text on every accent readable: white where it reads, dark on the light colours', () => {
    for (const [id, a] of Object.entries(ACCENTS)) {
      expect(contrast(a.hex, accentVars(id)['--on-accent']), id).toBeGreaterThanOrEqual(3);
    }
    // The point of the change: a yellow can be offered at all.
    expect(accentVars('yellow')['--on-accent']).not.toBe('#ffffff');
  });

  it('fills primary buttons so their label reads at 4.5:1, for every accent', () => {
    for (const [id, a] of Object.entries(ACCENTS)) {
      const v = accentVars(id);
      expect(contrast(v['--accent-fill'], v['--on-accent']), id).toBeGreaterThanOrEqual(4.5);
      expect(contrast(v['--accent-fill-hover'], v['--on-accent']), id + ' hover').toBeGreaterThanOrEqual(4.5);
    }
    // The violet's white label was 4.1:1 on the violet itself.
    expect(contrast(ACCENTS.violet.hex, '#ffffff')).toBeLessThan(4.5);
    expect(accentVars('violet')['--accent-fill']).not.toBe(ACCENTS.violet.hex);
    // An accent that already reads is left as it is.
    expect(accentFill('#5b21b6')).toBe('#5b21b6');
  });

  it('keeps the accent readable as text on the page, in either theme', () => {
    for (const id of Object.keys(ACCENTS)) {
      expect(contrast(accentVars(id, 'dark')['--accent-text'], '#17171a'), `${id} on dark`).toBeGreaterThanOrEqual(3);
      expect(contrast(accentVars(id, 'light')['--accent-text'], '#ffffff'), `${id} on light`).toBeGreaterThanOrEqual(3);
    }
  });

  it('leaves the accents there have always been exactly as they were', () => {
    for (const id of ['violet', 'blue', 'teal', 'green', 'orange', 'rose']) {
      for (const theme of ['dark', 'light']) {
        const v = accentVars(id, theme);
        expect(v['--on-accent'], id).toBe('#ffffff');
        expect(v['--accent-text'], `${id} ${theme}`).toBe(ACCENTS[id].hex);
      }
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

  it("puts a theme's colours and own accent on the page, and its background behind the window", () => {
    const root = fakeRoot();
    let windowBg = null;
    applyAppearance({ ...DEFAULT_APPEARANCE, family: 'frost', theme: 'system', accent: 'theme' }, { root, prefersDark: true, setWindowBackground: (...rgb) => (windowBg = rgb) });
    expect(root.dataset.theme).toBe('dark');
    expect(root.dataset.look).toBe('frost');
    expect(root.props['--bg']).toBe('#1c212b');
    expect(root.props['--accent']).toBe('#88c0d0');
    expect(windowBg).toEqual([28, 33, 43]);
    applyAppearance({ ...DEFAULT_APPEARANCE, family: 'frost', theme: 'system', accent: 'theme' }, { root, prefersDark: false });
    expect(root.dataset.look).toBe('frostDay');
    expect(root.dataset.theme).toBe('light');
    // A dark-only theme stays dark, whatever is asked.
    applyAppearance({ ...DEFAULT_APPEARANCE, family: 'blackout', theme: 'light' }, { root });
    expect(root.dataset.theme).toBe('dark');
    expect(root.props['--bg']).toBe('#000000');
  });

  it("puts a theme of your own on the page — its corners, font, status colours and shadows — and takes them off again", () => {
    const root = fakeRoot();
    const mine = {
      id: 'gold1',
      name: 'Gold',
      colours: { background: '#0f0d0a', panels: '#1a1712', text: '#f1e9da', quiet: '#b0a48f', accent: '#d8b052', onAccent: '', success: '#33cc66' },
      roundness: 0.5,
      shadows: 0,
      font: 'mono'
    };
    applyAppearance({ ...DEFAULT_APPEARANCE, customThemes: [mine], family: 'custom:gold1', accent: 'theme' }, { root });
    expect(root.dataset.look).toBe('custom:gold1');
    expect(root.props['--bg']).toBe('#0f0d0a');
    expect(root.props['--round']).toBe('0.5');
    expect(root.props['--font-ui']).toContain('Cascadia Code');
    expect(root.props['--success']).toBe('#33cc66');
    expect(root.props['--success-rgb']).toBe('51, 204, 102');
    expect(root.props['--shadow']).toContain('0.000');
    applyAppearance(DEFAULT_APPEARANCE, { root });
    for (const k of ['--round', '--font-ui', '--success', '--shadow']) expect(root.props[k], k).toBeUndefined();
  });

  it("forgets a theme of your own that is gone, and goes back to OpenSave's", () => {
    expect(sanitizeAppearance({ family: 'custom:gone', customThemes: [] }).family).toBe('opensave');
  });

  it('tells the page when marks on the accent must be dark, which an inline image cannot read from a variable', () => {
    const root = fakeRoot();
    applyAppearance({ ...DEFAULT_APPEARANCE, theme: 'light', accent: 'yellow' }, { root });
    expect(root.dataset.onAccent).toBe('dark');
    expect(root.props['--accent-text']).not.toBe(ACCENTS.yellow.hex); // darkened to read on white
    applyAppearance({ ...DEFAULT_APPEARANCE, accent: 'blue' }, { root });
    expect(root.dataset.onAccent).toBe('light');
  });
});
