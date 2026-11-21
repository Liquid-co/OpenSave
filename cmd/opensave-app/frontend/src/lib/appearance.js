// How the app looks on this device: light or dark, the accent colour, how
// large everything is drawn, and whether things move.
//
// Kept on this device, like the library view (see libraryview.js), and for
// the same reasons: it is about this screen and the person in front of it,
// and it takes effect as it is chosen rather than when a form is saved.
import { writable } from 'svelte/store';

export const THEMES = { dark: 'Dark', light: 'Light', system: 'Match system' };

// Around the colour wheel. Any colour can be offered, the bright yellows and
// limes included: the text on an accent, and the accent used as text on the
// page, are each worked out from it to stay readable (onAccent, accentText
// below). The first six are the ones there have always been, and look exactly
// as they did. Beyond these, any colour can be picked (a custom accent, stored
// as its hex); slate, once the last of these, is now one of those.
export const ACCENTS = {
  rose: { label: 'Rose', hex: '#e11d48' },
  red: { label: 'Red', hex: '#dc2626' },
  orange: { label: 'Orange', hex: '#ea580c' },
  amber: { label: 'Amber', hex: '#f59e0b' },
  gold: { label: 'Gold', hex: '#eab308' },
  yellow: { label: 'Yellow', hex: '#facc15' },
  lime: { label: 'Lime', hex: '#84cc16' },
  green: { label: 'Green', hex: '#16a34a' },
  emerald: { label: 'Emerald', hex: '#10b981' },
  teal: { label: 'Teal', hex: '#0d9488' },
  cyan: { label: 'Cyan', hex: '#06b6d4' },
  sky: { label: 'Sky', hex: '#0ea5e9' },
  blue: { label: 'Blue', hex: '#3b82f6' },
  indigo: { label: 'Indigo', hex: '#6366f1' },
  violet: { label: 'Violet', hex: '#8a63f4' },
  purple: { label: 'Purple', hex: '#a855f7' },
  pink: { label: 'Pink', hex: '#ec4899' }
};

// Accents that used to be presets, kept as the colour they were: someone who
// chose slate keeps it, as a picked colour.
const RETIRED_ACCENTS = { slate: '#64748b' };

// How many picked colours are kept beside the presets: one row of them. A new
// one past that pushes out the oldest.
export const MAX_CUSTOM_ACCENTS = 8;

export const SCALES = [0.9, 1, 1.1, 1.25];

// What lights the top of the Library: the cover of the game you play (or
// played last), a glow in the accent colour, or nothing.
export const BACKDROPS = { cover: 'Cover art', accent: 'Accent glow', off: 'Off' };

export const DEFAULT_APPEARANCE = Object.freeze({
  theme: 'dark',
  accent: 'violet',
  customAccents: Object.freeze([]),
  scale: 1,
  motion: true,
  controller: 'auto',
  backdrop: 'cover',
  // A game's page takes its Play button and tab colours from the cover
  // (lib/covertint.js), rather than the accent.
  coverColours: true
});

// Controller navigation (lib/controller.js): on by itself when a controller is
// used or on a Steam Deck or handheld, or set on or off.
const CONTROLLER_CHOICES = ['auto', 'on', 'off'];

/** A colour as "#rrggbb" in lower case, or null if it is not one. */
export function normalizeHex(value) {
  if (typeof value !== 'string') return null;
  let s = value.trim().toLowerCase();
  if (!s.startsWith('#')) s = '#' + s;
  if (/^#[0-9a-f]{3}$/.test(s)) s = '#' + [...s.slice(1)].map((c) => c + c).join('');
  return /^#[0-9a-f]{6}$/.test(s) ? s : null;
}

/** The hex an accent stands for: a preset's colour, or a picked one's own. */
export function accentHex(accent) {
  return ACCENTS[accent]?.hex ?? RETIRED_ACCENTS[accent] ?? normalizeHex(accent) ?? ACCENTS[DEFAULT_APPEARANCE.accent].hex;
}

/** An appearance with every field valid, whatever was stored. */
export function sanitizeAppearance(raw) {
  const v = raw && typeof raw === 'object' ? raw : {};
  const scale = Number(v.scale);
  const customAccents = [];
  for (const c of Array.isArray(v.customAccents) ? v.customAccents : []) {
    const hex = normalizeHex(c);
    if (hex && !customAccents.includes(hex)) customAccents.push(hex);
  }
  // A preset by name, or a picked colour by its hex. A picked colour is always
  // among the ones shown, so the one in use can be seen chosen.
  let accent = DEFAULT_APPEARANCE.accent;
  if (v.accent in ACCENTS) {
    accent = v.accent;
  } else {
    const hex = RETIRED_ACCENTS[v.accent] ?? normalizeHex(v.accent);
    if (hex) {
      accent = hex;
      if (!customAccents.includes(hex)) customAccents.push(hex);
    }
  }
  while (customAccents.length > MAX_CUSTOM_ACCENTS) {
    const oldest = customAccents.findIndex((c) => c !== accent);
    customAccents.splice(oldest, 1);
  }
  return {
    theme: v.theme in THEMES ? v.theme : DEFAULT_APPEARANCE.theme,
    accent,
    customAccents,
    scale: SCALES.includes(scale) ? scale : DEFAULT_APPEARANCE.scale,
    motion: typeof v.motion === 'boolean' ? v.motion : DEFAULT_APPEARANCE.motion,
    controller: CONTROLLER_CHOICES.includes(v.controller) ? v.controller : DEFAULT_APPEARANCE.controller,
    backdrop: v.backdrop in BACKDROPS ? v.backdrop : DEFAULT_APPEARANCE.backdrop,
    coverColours: typeof v.coverColours === 'boolean' ? v.coverColours : DEFAULT_APPEARANCE.coverColours
  };
}

/** The appearance with a picked colour added (newest last) and in use. */
export function withCustomAccent(v, color) {
  const hex = normalizeHex(color);
  if (!hex) return sanitizeAppearance(v);
  const a = sanitizeAppearance(v);
  const customAccents = [...a.customAccents.filter((c) => c !== hex), hex];
  while (customAccents.length > MAX_CUSTOM_ACCENTS) customAccents.shift();
  return { ...a, accent: hex, customAccents };
}

/** The appearance without a picked colour; if it was in use, back to the
 *  default accent. */
export function withoutCustomAccent(v, color) {
  const a = sanitizeAppearance(v);
  const hex = normalizeHex(color);
  return {
    ...a,
    accent: a.accent === hex ? DEFAULT_APPEARANCE.accent : a.accent,
    customAccents: a.customAccents.filter((c) => c !== hex)
  };
}

/** Hue (0-360), saturation and value (0-1) as "#rrggbb". */
export function hsvToHex(h, s, v) {
  const f = (n) => {
    const k = (n + h / 60) % 6;
    return v - v * s * Math.max(0, Math.min(k, 4 - k, 1));
  };
  return '#' + [f(5), f(3), f(1)].map((c) => Math.round(c * 255).toString(16).padStart(2, '0')).join('');
}

/** "#rrggbb" as hue (0-360), saturation and value (0-1). */
export function hexToHsv(hex) {
  const [r, g, b] = hexToRgb(normalizeHex(hex) ?? '#000000').map((c) => c / 255);
  const max = Math.max(r, g, b);
  const d = max - Math.min(r, g, b);
  let h = 0;
  if (d) {
    if (max === r) h = ((g - b) / d) % 6;
    else if (max === g) h = (b - r) / d + 2;
    else h = (r - g) / d + 4;
    h = (h * 60 + 360) % 360;
  }
  return { h, s: max ? d / max : 0, v: max };
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

/** The colour a fraction of the way to black. */
export function darken(hex, amount) {
  return (
    '#' +
    hexToRgb(hex)
      .map((c) => Math.round(c * (1 - amount)).toString(16).padStart(2, '0'))
      .join('')
  );
}

/** WCAG relative luminance. */
export function luminance(hex) {
  const [r, g, b] = hexToRgb(hex).map((c) => {
    const s = c / 255;
    return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

/** WCAG contrast ratio between two colours, 1 to 21. */
export function contrast(a, b) {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (hi + 0.05) / (lo + 0.05);
}

// The least contrast text and marks keep against the colour behind them: what
// WCAG asks of large text and of the parts of a control, and what every
// accent there has ever been already met in white.
export const MIN_CONTRAST = 3;

// Near-black for text on a light accent: the app's own darkest ink rather
// than pure black, which looks harsh on a bright colour.
export const DARK_INK = '#141416';

/** The text on an accent — every primary button, every badge: white while
 *  white reads, near-black on the light colours where it would not. */
export function onAccent(hex) {
  return contrast(hex, '#ffffff') >= MIN_CONTRAST ? '#ffffff' : DARK_INK;
}

// What a button's label needs against its fill: WCAG's figure for ordinary
// text. A label at 13px is not "large", so the 3:1 that is enough for a badge
// or a tick is not enough for the words on a primary button.
export const TEXT_CONTRAST = 4.5;

/** The fill of a primary button: the accent itself where its label reads at
 *  4.5:1, and otherwise darkened (white label) or lightened (dark label) just
 *  enough. The violet's white label was 4.1:1 on it; this makes it 4.5:1. */
export function accentFill(hex) {
  const ink = onAccent(hex);
  const shift = ink === '#ffffff' ? darken : lighten;
  for (let amount = 0; amount <= 1; amount += 0.02) {
    const c = amount === 0 ? hex : shift(hex, amount);
    if (contrast(c, ink) >= TEXT_CONTRAST) return c;
  }
  return ink === '#ffffff' ? '#000000' : '#ffffff';
}

// The surface accent-coloured text is judged against in each theme: the
// palest one it is drawn on — the raised card in either.
const TEXT_SURFACE = { dark: '#17171a', light: '#ffffff' };

/** The accent as text or an icon on the page: itself where it reads, and
 *  otherwise lightened (dark theme) or darkened (light) just enough to. A
 *  yellow is fine on dark grey and unreadable on white. */
export function accentText(hex, theme = 'dark') {
  const surface = TEXT_SURFACE[theme] ?? TEXT_SURFACE.dark;
  const shift = theme === 'light' ? darken : lighten;
  for (let amount = 0; amount <= 1; amount += 0.05) {
    const c = amount === 0 ? hex : shift(hex, amount);
    if (contrast(c, surface) >= MIN_CONTRAST) return c;
  }
  return theme === 'light' ? DARK_INK : '#ffffff';
}

/** The CSS variables that carry the accent (see app.css). */
export function accentVars(accent, theme = 'dark') {
  const hex = accentHex(accent);
  return {
    '--accent': hex,
    '--accent-hover': lighten(hex, 0.12),
    '--accent-rgb': hexToRgb(hex).join(', '),
    '--on-accent': onAccent(hex),
    '--accent-text': accentText(hex, theme),
    '--accent-fill': accentFill(hex),
    // Hovering moves the fill away from its label, never towards it, so the
    // label reads at least as well under the pointer.
    '--accent-fill-hover': onAccent(hex) === '#ffffff' ? darken(accentFill(hex), 0.1) : lighten(accentFill(hex), 0.1)
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
  const vars = accentVars(a.accent, theme);
  for (const [name, value] of Object.entries(vars)) root.style.setProperty(name, value);
  // For what CSS cannot colour with a variable: the tick drawn inside a
  // checkbox is an inline image (see app.css).
  root.dataset.onAccent = vars['--on-accent'] === '#ffffff' ? 'light' : 'dark';
  // zoom rather than a larger root font size: much of the app is measured in
  // pixels — icons, tiles, paddings — and would stay put while the text grew.
  root.style.zoom = a.scale === 1 ? '' : String(a.scale);
  setWindowBackground?.(...WINDOW_BACKGROUND[theme]);
}

export const appearance = writable(loadAppearance());
appearance.subscribe((v) => saveAppearance(v));
