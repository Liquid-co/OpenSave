// Themes people make (the ThemeEditorV3 board): a name, six colours — the
// rest worked out from them — and the shape and type of the whole app:
// corners, borders, shadows and the font. Kept with the appearance on this
// device (lib/appearance.js customThemes), and taken to and from other
// people as a small file (.opensave-theme) that is read, never run.
//
// The colours picked:
//   background — behind everything
//   panels — cards, menus and the sidebar's ground
//   text — titles and body text
//   quiet — hints, dates and labels
//   accent — buttons, selection and focus
//   onAccent — text on the accent ('' picks it: white or near-black)
// and, if wanted, the sidebar, and success, warning and danger ('' keeps
// the app's own for dark or light).
//
// What is worked out: whether it is dark, the hover and selected shades,
// the two strengths of border, and the faintest text — kept readable.
import { LOOKS } from './themes.js';
import { contrast, normalizeHex, mix, hexToHsl, hslToHex, isDark, onAccent, accentFill } from './colour.js';

export const THEME_FILE = 'opensave-theme';
export const THEME_FILE_VERSION = 1;
/** A theme file is a few hundred bytes; anything far bigger is not one. */
export const MAX_FILE_BYTES = 16 * 1024;
export const MAX_NAME = 40;
export const MAX_THEMES = 30;
/** What readable means here: WCAG's figure for ordinary text. */
export const READABLE = 4.5;

export const COLOURS = ['background', 'panels', 'text', 'quiet', 'accent', 'onAccent'];
export const MORE_COLOURS = ['sidebar', 'success', 'warning', 'danger'];

/** The fonts a theme can use: each a list, the first one this device has
 *  wins. `custom` is a font named by hand. */
export const FONTS = {
  system: "'Segoe UI Variable Text', 'Segoe UI', system-ui, -apple-system, 'Cantarell', 'Ubuntu', sans-serif",
  rounded: "ui-rounded, 'SF Pro Rounded', 'Nunito', 'Quicksand', 'Varela Round', 'Segoe UI Variable Display', system-ui, sans-serif",
  display: "'Bahnschrift', 'DIN Alternate', 'Barlow', 'Segoe UI', system-ui, sans-serif",
  serif: "'Iowan Old Style', 'Palatino Linotype', 'Book Antiqua', Georgia, serif",
  mono: "'Cascadia Code', 'JetBrains Mono', 'SF Mono', Consolas, ui-monospace, monospace"
};

/** How far each shape setting goes, and where it starts. */
export const SHAPE = {
  roundness: { min: 0, max: 1.75, step: 0.05, default: 1 },
  borders: { min: 0, max: 2, step: 0.1, default: 1 },
  shadows: { min: 0, max: 2, step: 0.1, default: 1 }
};

const clamp = (v, { min, max, default: d }) => (Number.isFinite(Number(v)) ? Math.min(max, Math.max(min, Number(v))) : d);
const cleanName = (name) =>
  String(name ?? '')
    .replace(/[\u0000-\u001f\u007f<>]/g, '')
    .trim()
    .slice(0, MAX_NAME);
// A font family name, as CSS would take it quoted: letters, digits, spaces
// and a few marks, nothing that could close the quote.
const cleanFontName = (name) =>
  String(name ?? '')
    .replace(/[^\p{L}\p{N} ._-]/gu, '')
    .trim()
    .slice(0, 60);

export const newId = () => 't' + Date.now().toString(36) + Math.random().toString(36).slice(2, 6);

/** A theme with every field valid, whatever it was given; null if it has
 *  not the colours a theme needs. */
export function sanitizeTheme(raw) {
  if (!raw || typeof raw !== 'object') return null;
  const c = raw.colours && typeof raw.colours === 'object' ? raw.colours : {};
  const colours = {};
  for (const k of ['background', 'panels', 'text', 'quiet', 'accent']) {
    const hex = normalizeHex(c[k]);
    if (!hex) return null;
    colours[k] = hex;
  }
  for (const k of ['onAccent', ...MORE_COLOURS]) colours[k] = normalizeHex(c[k]) ?? '';
  const font = raw.font in FONTS || raw.font === 'custom' ? raw.font : 'system';
  return {
    id: typeof raw.id === 'string' && /^[a-z0-9]{3,24}$/i.test(raw.id) ? raw.id : newId(),
    name: cleanName(raw.name) || 'My theme',
    colours,
    roundness: clamp(raw.roundness, SHAPE.roundness),
    borders: clamp(raw.borders, SHAPE.borders),
    shadows: clamp(raw.shadows, SHAPE.shadows),
    font,
    customFont: font === 'custom' ? cleanFontName(raw.customFont) : ''
  };
}

/** A new theme, starting from a look (an id in LOOKS, or a look). */
export function themeFromLook(look, name = '') {
  const l = typeof look === 'string' ? (LOOKS[look] ?? LOOKS.night) : look;
  return sanitizeTheme({
    name,
    colours: { background: l.bg, panels: l.raised, text: l.text, quiet: l.textDim, accent: l.accent, onAccent: '' },
    roundness: 1,
    borders: 1,
    shadows: 1,
    font: 'system'
  });
}

/** The theme's colour, moved in lightness only — hue and saturation kept —
 *  just far enough to read at `target` on every one of `on`. */
export function fixLightness(fg, on, target = READABLE) {
  const surfaces = Array.isArray(on) ? on : [on];
  const worst = (c) => Math.min(...surfaces.map((s) => contrast(c, s)));
  if (worst(fg) >= target) return fg;
  const { h, s, l } = hexToHsl(fg);
  // Away from the surfaces: lighter on dark ones, darker on light.
  const lighter = !isDark(surfaces[0]) ? -1 : 1;
  for (let step = 0.01; step <= 1; step += 0.01) {
    const c = hslToHex(h, s, Math.min(1, Math.max(0, l + lighter * step)));
    if (worst(c) >= target) return c;
  }
  return lighter > 0 ? '#ffffff' : '#000000';
}

/** The whole look a theme makes, as lib/themes.js LOOKS are, with its
 *  shape, type and own status colours beside. */
export function deriveLook(theme) {
  const t = sanitizeTheme(theme) ?? themeFromLook('night');
  const { background: bg, panels, text, quiet, accent } = t.colours;
  const dark = isDark(bg);
  const sidebar = t.colours.sidebar || mix(bg, panels, 0.45);
  const hover = mix(panels, text, dark ? 0.045 : 0.04);
  const active = mix(panels, text, dark ? 0.085 : 0.075);
  const b = t.borders;
  const border = mix(panels, text, 0.065 * b);
  const borderStrong = mix(panels, text, 0.13 * b);
  const surfaces = [bg, sidebar, panels, hover];
  return {
    kind: dark ? 'dark' : 'light',
    bg,
    sidebar,
    raised: panels,
    hover,
    active,
    border,
    borderStrong,
    text,
    textDim: quiet,
    // Fainter than the quiet text, and still to be read wherever it is.
    textFaint: fixLightness(mix(quiet, panels, 0.22), surfaces),
    accent,
    onAccent: t.colours.onAccent || onAccent(accent),
    // What a primary button is filled with: the accent, deepened or
    // lightened until its label reads (lib/appearance.js accentVars).
    accentFill: accentFill(accent),
    success: t.colours.success,
    warning: t.colours.warning,
    danger: t.colours.danger,
    roundness: t.roundness,
    shadows: t.shadows,
    font: t.font === 'custom' && t.customFont ? `'${t.customFont}', ${FONTS.system}` : FONTS[t.font]
  };
}

/** Each pair of colours text is read on, and how well. */
export function readability(theme) {
  const l = deriveLook(theme);
  const t = sanitizeTheme(theme);
  return [
    { id: 'textOnBackground', fg: l.text, bg: l.bg },
    { id: 'textOnPanels', fg: l.text, bg: l.raised },
    { id: 'quietOnPanels', fg: l.textDim, bg: l.raised },
    { id: 'quietOnBackground', fg: l.textDim, bg: l.bg },
    { id: 'textOnAccent', fg: l.onAccent, bg: l.accentFill }
  ].map((c) => ({ ...c, ratio: contrast(c.fg, c.bg), ok: contrast(c.fg, c.bg) >= READABLE }));
}

/** The theme with one check passed, by moving the lightness of the colour
 *  the words are in: text, quiet text, or the text on the accent (set to
 *  its own colour if it was picked for you). */
export function fixCheck(theme, id) {
  const t = sanitizeTheme(theme);
  const l = deriveLook(t);
  const c = { ...t.colours };
  if (id === 'textOnBackground' || id === 'textOnPanels') c.text = fixLightness(c.text, [l.bg, l.raised]);
  else if (id === 'quietOnPanels' || id === 'quietOnBackground') c.quiet = fixLightness(c.quiet, [l.bg, l.raised]);
  else if (id === 'textOnAccent') {
    // The label on a button, read on the fill the app draws (accentFill).
    const fixedInk = fixLightness(l.onAccent, l.accentFill);
    // Ink that cannot get there (a fill in the middle of the range) moves
    // the accent instead.
    if (contrast(fixedInk, l.accentFill) >= READABLE) c.onAccent = fixedInk;
    else c.accent = fixLightness(c.accent, l.onAccent);
  }
  return { ...t, colours: c };
}

/** A name not already in use: a theme of your own may not take a built-in
 *  one's name bare, nor another of yours. */
export function uniqueName(name, takenNames = [], builtInNames = []) {
  let base = cleanName(name) || 'My theme';
  const lower = (s) => s.toLocaleLowerCase();
  if (builtInNames.map(lower).includes(lower(base))) base = `${base} (yours)`;
  let candidate = base;
  for (let n = 2; takenNames.map(lower).includes(lower(candidate)); n++) candidate = `${base} ${n}`;
  return candidate;
}

/** A theme as a file to share: what it is, and nothing of where it was. */
export function exportTheme(theme) {
  const t = sanitizeTheme(theme);
  return JSON.stringify(
    {
      format: THEME_FILE,
      version: THEME_FILE_VERSION,
      name: t.name,
      colours: t.colours,
      roundness: t.roundness,
      borders: t.borders,
      shadows: t.shadows,
      font: t.font,
      ...(t.font === 'custom' ? { customFont: t.customFont } : {})
    },
    null,
    2
  );
}

/**
 * A theme from a file, checked before anything uses it: small, of our
 * format and a version known, with real colours. Returns {theme} — a new
 * one, with its own id — or {error}, the reason as a catalog key.
 */
export function importTheme(text) {
  if (typeof text !== 'string' || text.length > MAX_FILE_BYTES) return { error: 'themeEditor.import.tooBig' };
  let data;
  try {
    data = JSON.parse(text);
  } catch {
    return { error: 'themeEditor.import.notATheme' };
  }
  if (!data || typeof data !== 'object' || data.format !== THEME_FILE) return { error: 'themeEditor.import.notATheme' };
  if (!Number.isInteger(data.version) || data.version < 1 || data.version > THEME_FILE_VERSION) return { error: 'themeEditor.import.newerVersion' };
  const theme = sanitizeTheme({ ...data, id: undefined });
  if (!theme) return { error: 'themeEditor.import.badColours' };
  return { theme };
}
