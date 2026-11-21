// How the app looks on this device: its theme (lib/themes.js) and whether it
// is light or dark, the accent colour, how large everything is drawn, and
// whether things move.
//
// Kept on this device, like the library view (see libraryview.js), and for
// the same reasons: it is about this screen and the person in front of it,
// and it takes effect as it is chosen rather than when a form is saved.
import { writable } from 'svelte/store';
import { normalizeHex, hsvToHex, hexToHsv, hexToRgb, lighten, darken, luminance, contrast, onAccent, MIN_CONTRAST, DARK_INK, accentFill, TEXT_CONTRAST } from './colour.js';
export { normalizeHex, hsvToHex, hexToHsv, hexToRgb, lighten, darken, luminance, contrast, onAccent, MIN_CONTRAST, DARK_INK, accentFill, TEXT_CONTRAST };
import { LOOKS, FAMILIES, DEFAULT_FAMILY, lookFor, lookVars } from './themes.js';
import { sanitizeTheme, deriveLook, MAX_THEMES, FONTS } from './customthemes.js';
const FONT_DEFAULT = FONTS.system;

/** The family id a theme of your own goes by. */
export const customFamily = (id) => `custom:${id}`;

// Light or dark, for a theme that has both (lib/themes.js FAMILIES).
export const THEMES = { dark: 'Dark', light: 'Light', system: 'Match system' };

// The accent that is the theme's own: violet for OpenSave's, mauve for
// Catppuccin's. Picking a theme picks it; any colour can still be chosen on
// top.
export const THEME_ACCENT = 'theme';

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
  // The theme (lib/themes.js), and light or dark where it has both.
  family: DEFAULT_FAMILY,
  theme: 'dark',
  accent: 'violet',
  customAccents: Object.freeze([]),
  scale: 1,
  motion: true,
  controller: 'auto',
  backdrop: 'cover',
  // A game's page takes its Play button and tab colours from the cover
  // (lib/covertint.js), rather than the accent.
  coverColours: true,
  // Themes made here (lib/customthemes.js), newest last.
  customThemes: Object.freeze([])
});

// Controller navigation (lib/controller.js): on by itself when a controller is
// used or on a Steam Deck or handheld, or set on or off.
const CONTROLLER_CHOICES = ['auto', 'on', 'off'];

/** The hex an accent stands for: a preset's colour, a picked one's own, or
 *  — for the theme's own — `themeAccent`. */
export function accentHex(accent, themeAccent = LOOKS.night.accent) {
  if (accent === THEME_ACCENT) return themeAccent;
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
  if (v.accent in ACCENTS || v.accent === THEME_ACCENT) {
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
  const customThemes = [];
  for (const raw of Array.isArray(v.customThemes) ? v.customThemes : []) {
    const t = sanitizeTheme(raw);
    if (t && !customThemes.some((x) => x.id === t.id) && customThemes.length < MAX_THEMES) customThemes.push(t);
  }
  const family = v.family in FAMILIES || customThemes.some((t) => customFamily(t.id) === v.family) ? v.family : DEFAULT_APPEARANCE.family;
  return {
    family,
    customThemes,
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


// The surface accent-coloured text is judged against in each theme: the
// palest one it is drawn on — the raised card in either. A theme passes its
// own.
const TEXT_SURFACE = { dark: '#17171a', light: '#ffffff' };

/** The accent as text or an icon on the page: itself where it reads, and
 *  otherwise lightened (dark theme) or darkened (light) just enough to. A
 *  yellow is fine on dark grey and unreadable on white. */
export function accentText(hex, theme = 'dark', surface = TEXT_SURFACE[theme] ?? TEXT_SURFACE.dark) {
  const shift = theme === 'light' ? darken : lighten;
  for (let amount = 0; amount <= 1; amount += 0.05) {
    const c = amount === 0 ? hex : shift(hex, amount);
    if (contrast(c, surface) >= MIN_CONTRAST) return c;
  }
  return theme === 'light' ? DARK_INK : '#ffffff';
}

/** The CSS variables that carry the accent (see app.css), on a theme whose
 *  panels are `surface` and whose own accent is `themeAccent`. */
export function accentVars(accent, theme = 'dark', { surface, themeAccent } = {}) {
  const hex = accentHex(accent, themeAccent);
  return {
    '--accent': hex,
    '--accent-hover': lighten(hex, 0.12),
    '--accent-rgb': hexToRgb(hex).join(', '),
    '--on-accent': onAccent(hex),
    '--accent-text': accentText(hex, theme, surface),
    '--accent-fill': accentFill(hex),
    // Hovering moves the fill away from its label, never towards it, so the
    // label reads at least as well under the pointer.
    '--accent-fill-hover': onAccent(hex) === '#ffffff' ? darken(accentFill(hex), 0.1) : lighten(accentFill(hex), 0.1)
  };
}

// The window's own background, behind the page, for each theme: what shows
// for a moment while the window is resized. It matches --bg in app.css.
export const WINDOW_BACKGROUND = { dark: [12, 12, 13], light: [244, 244, 247] };

/** The look in use (lib/themes.js LOOKS): the theme's, light or dark as
 *  chosen or as the system is. A theme of your own has one look, its id
 *  being its family's. */
export function currentLook(v, prefersDark = true) {
  return lookOf(v, prefersDark).id;
}

/** The look in use and its id: one of the app's own, or a theme of your own
 *  worked out from its colours (lib/customthemes.js deriveLook). */
export function lookOf(v, prefersDark = true) {
  const a = sanitizeAppearance(v);
  const mine = a.customThemes.find((t) => customFamily(t.id) === a.family);
  if (mine) return { id: a.family, look: deriveLook(mine), theme: mine };
  const id = lookFor(a.family, a.theme, prefersDark);
  return { id, look: LOOKS[id], theme: null };
}

// The shadows a theme's Shadows setting scales: the app's own for dark and
// light (app.css), at that strength.
const SHADOWS = {
  dark: (k) => [`0 16px 40px rgba(0, 0, 0, ${(0.55 * k).toFixed(3)})`, `0 18px 34px -14px rgba(0, 0, 0, ${Math.min(0.9 * k, 1).toFixed(3)})`],
  light: (k) => [`0 16px 40px rgba(20, 20, 40, ${(0.16 * k).toFixed(3)})`, `0 16px 30px -14px rgba(20, 20, 40, ${(0.35 * k).toFixed(3)})`]
};
const rgbOf = (hex) => hexToRgb(hex).join(', ');

/** Puts an appearance on the page: the theme's colours and whether it is
 *  dark or light (data-theme, which the kind-specific rules in app.css read),
 *  accent, scale, and data-motion, which app.css reads to still everything
 *  when it is 'off'. */
export function applyAppearance(
  v,
  { root = globalThis.document?.documentElement, prefersDark = true, reduceMotion = false, setWindowBackground } = {}
) {
  if (!root) return;
  const a = sanitizeAppearance(v);
  const { id: lookId, look } = lookOf(a, prefersDark);
  const theme = look.kind;
  root.dataset.theme = theme;
  root.dataset.look = lookId;
  root.dataset.motion = motionAllowed(a, reduceMotion) ? 'on' : 'off';
  const vars = { ...lookVars(look), ...accentVars(a.accent, theme, { surface: look.raised, themeAccent: look.accent }) };
  // A theme of your own can set what the app's do not: the text on its
  // accent, its own status colours, its corners, shadows and font. Each is
  // cleared again when it does not, so the app's own come back.
  if (look.onAccent && a.accent === THEME_ACCENT) vars['--on-accent'] = look.onAccent;
  const own = {
    '--round': look.roundness != null && look.roundness !== 1 ? String(look.roundness) : null,
    '--font-ui': look.font && look.font !== FONT_DEFAULT ? look.font : null,
    '--success': look.success || null,
    '--success-rgb': look.success ? rgbOf(look.success) : null,
    '--warn': look.warning || null,
    '--warn-rgb': look.warning ? rgbOf(look.warning) : null,
    '--danger': look.danger || null,
    '--danger-rgb': look.danger ? rgbOf(look.danger) : null,
    '--danger-text': look.danger || null,
    '--shadow': look.shadows != null && look.shadows !== 1 ? SHADOWS[theme](look.shadows)[0] : null,
    '--shadow-lift': look.shadows != null && look.shadows !== 1 ? SHADOWS[theme](look.shadows)[1] : null
  };
  for (const [name, value] of Object.entries(vars)) root.style.setProperty(name, value);
  for (const [name, value] of Object.entries(own)) {
    if (value == null) root.style.removeProperty?.(name);
    else root.style.setProperty(name, value);
  }
  // For what CSS cannot colour with a variable: the tick drawn inside a
  // checkbox is an inline image (see app.css).
  root.dataset.onAccent = vars['--on-accent'] === '#ffffff' ? 'light' : 'dark';
  // zoom rather than a larger root font size: much of the app is measured in
  // pixels — icons, tiles, paddings — and would stay put while the text grew.
  root.style.zoom = a.scale === 1 ? '' : String(a.scale);
  setWindowBackground?.(...hexToRgb(look.bg));
}

export const appearance = writable(loadAppearance());
appearance.subscribe((v) => saveAppearance(v));
