// Colour arithmetic: hex in and out, mixing, lightness, and how readable
// one colour is on another (WCAG). Shared by the appearance, the themes and
// the themes people make (lib/appearance.js, lib/themes.js,
// lib/customthemes.js).

/** A colour as "#rrggbb" in lower case, or null if it is not one. */
export function normalizeHex(value) {
  if (typeof value !== 'string') return null;
  let s = value.trim().toLowerCase();
  if (!s.startsWith('#')) s = '#' + s;
  if (/^#[0-9a-f]{3}$/.test(s)) s = '#' + [...s.slice(1)].map((c) => c + c).join('');
  return /^#[0-9a-f]{6}$/.test(s) ? s : null;
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

/** The colour `amount` of the way from `a` to `b`. */
export function mix(a, b, amount) {
  const x = hexToRgb(a);
  const y = hexToRgb(b);
  return '#' + x.map((c, i) => Math.round(c + (y[i] - c) * amount).toString(16).padStart(2, '0')).join('');
}

/** "#rrggbb" as hue (0-360), saturation and lightness (0-1). */
export function hexToHsl(hex) {
  const [r, g, b] = hexToRgb(hex).map((c) => c / 255);
  const max = Math.max(r, g, b);
  const min = Math.min(r, g, b);
  const l = (max + min) / 2;
  const d = max - min;
  let h = 0;
  let sat = 0;
  if (d) {
    sat = d / (1 - Math.abs(2 * l - 1));
    if (max === r) h = ((g - b) / d) % 6;
    else if (max === g) h = (b - r) / d + 2;
    else h = (r - g) / d + 4;
    h = (h * 60 + 360) % 360;
  }
  return { h, s: sat, l };
}

/** Hue (0-360), saturation and lightness (0-1) as "#rrggbb". */
export function hslToHex(h, sat, l) {
  const c = (1 - Math.abs(2 * l - 1)) * sat;
  const x = c * (1 - Math.abs(((h / 60) % 2) - 1));
  const m = l - c / 2;
  const [r, g, b] = h < 60 ? [c, x, 0] : h < 120 ? [x, c, 0] : h < 180 ? [0, c, x] : h < 240 ? [0, x, c] : h < 300 ? [x, 0, c] : [c, 0, x];
  return '#' + [r, g, b].map((v) => Math.round(Math.min(Math.max(v + m, 0), 1) * 255).toString(16).padStart(2, '0')).join('');
}

/** Whether a colour is dark: nearer black than white, as text on it shows. */
export const isDark = (hex) => contrast(hex, '#000000') < contrast(hex, '#ffffff');

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
