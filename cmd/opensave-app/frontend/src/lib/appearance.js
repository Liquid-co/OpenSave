// How the app looks on this device: light or dark, the accent colour, how
// large everything is drawn, and whether things move.
//
// Kept on this device, like the library view (see libraryview.js), and for
// the same reasons: it is about this screen and the person in front of it,
// and it takes effect as it is chosen rather than when a form is saved.
import { writable } from 'svelte/store';

export const THEMES = { dark: 'Dark', light: 'Light', system: 'Match system' };

// Each dark enough that white text on it — every primary button — stays
// readable, which rules out the bright yellows and limes.
export const ACCENTS = {
  violet: { label: 'Violet', hex: '#8a63f4' },
  blue: { label: 'Blue', hex: '#3b82f6' },
  teal: { label: 'Teal', hex: '#0d9488' },
  green: { label: 'Green', hex: '#16a34a' },
  orange: { label: 'Orange', hex: '#ea580c' },
  rose: { label: 'Rose', hex: '#e11d48' }
};

export const SCALES = [0.9, 1, 1.1, 1.25];

export const DEFAULT_APPEARANCE = Object.freeze({ theme: 'dark', accent: 'violet', scale: 1, motion: true });

/** An appearance with every field valid, whatever was stored. */
export function sanitizeAppearance(raw) {
  const v = raw && typeof raw === 'object' ? raw : {};
  const scale = Number(v.scale);
  return {
    theme: v.theme in THEMES ? v.theme : DEFAULT_APPEARANCE.theme,
    accent: v.accent in ACCENTS ? v.accent : DEFAULT_APPEARANCE.accent,
    scale: SCALES.includes(scale) ? scale : DEFAULT_APPEARANCE.scale,
    motion: typeof v.motion === 'boolean' ? v.motion : DEFAULT_APPEARANCE.motion
  };
}

/** Whether things may move: chosen here, and never while the system asks
 *  apps to reduce motion. */
export const motionAllowed = (v, reduceMotion) => sanitizeAppearance(v).motion && !reduceMotion;

const KEY = 'opensave.appearance';

export function loadAppearance(storage = globalThis.localStorage) {
  try {
    const saved = storage?.getItem(KEY);
    return saved ? sanitizeAppearance(JSON.parse(saved)) : { ...DEFAULT_APPEARANCE };
  } catch {
    return { ...DEFAULT_APPEARANCE };
  }
}

export function saveAppearance(v, storage = globalThis.localStorage) {
  try {
    storage?.setItem(KEY, JSON.stringify(sanitizeAppearance(v)));
  } catch {
    // Storage refused (private mode, full): the choice lasts until restart.
  }
}

/** 'dark' or 'light', with 'system' settled by what the OS prefers. */
export function resolvedTheme(theme, prefersDark) {
  if (theme === 'system') return prefersDark ? 'dark' : 'light';
  return theme === 'light' ? 'light' : 'dark';
}

export function hexToRgb(hex) {
  const n = parseInt(hex.replace('#', ''), 16);
  return [(n >> 16) & 255, (n >> 8) & 255, n & 255];
}

/** The colour a fraction of the way to white: the accent's hover shade. */
export function lighten(hex, amount) {
  return (
    '#' +
    hexToRgb(hex)
      .map((c) => Math.round(c + (255 - c) * amount).toString(16).padStart(2, '0'))
      .join('')
  );
}

/** The CSS variables that carry the accent (see app.css). */
export function accentVars(accent) {
  const hex = (ACCENTS[accent] ?? ACCENTS[DEFAULT_APPEARANCE.accent]).hex;
  return {
    '--accent': hex,
    '--accent-hover': lighten(hex, 0.12),
    '--accent-rgb': hexToRgb(hex).join(', ')
  };
}

// The window's own background, behind the page, for each theme: what shows
// for a moment while the window is resized. It matches --bg in app.css.
export const WINDOW_BACKGROUND = { dark: [12, 12, 13], light: [244, 244, 247] };

/** Puts an appearance on the page: theme attribute, accent, scale, and
 *  data-motion, which app.css reads to still everything when it is 'off'. */
export function applyAppearance(
  v,
  { root = globalThis.document?.documentElement, prefersDark = true, reduceMotion = false, setWindowBackground } = {}
) {
  if (!root) return;
  const a = sanitizeAppearance(v);
  const theme = resolvedTheme(a.theme, prefersDark);
  root.dataset.theme = theme;
  root.dataset.motion = motionAllowed(a, reduceMotion) ? 'on' : 'off';
  for (const [name, value] of Object.entries(accentVars(a.accent))) root.style.setProperty(name, value);
  // zoom rather than a larger root font size: much of the app is measured in
  // pixels — icons, tiles, paddings — and would stay put while the text grew.
  root.style.zoom = a.scale === 1 ? '' : String(a.scale);
  setWindowBackground?.(...WINDOW_BACKGROUND[theme]);
}

export const appearance = writable(loadAppearance());
appearance.subscribe((v) => saveAppearance(v));
