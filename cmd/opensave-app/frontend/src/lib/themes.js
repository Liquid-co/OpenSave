// Whole-app themes (the SettingsV3 board): the colours of every surface and
// of text, and an accent of their own. A theme is a family of looks — dark,
// light, or both — and the dark / light / match-system choice picks between
// them where it has both. lib/appearance.js puts the one in use on the page.
//
// A look gives the surfaces and text; everything that says what kind of
// thing something is (success, warning, the device colours) stays the app's
// own for dark or light (app.css), so it means the same in every theme.

/** The looks: their kind, surfaces from the page up, text, and accent. */
export const LOOKS = {
  // The app's own, as it has always been.
  night: {
    kind: 'dark',
    bg: '#0c0c0d',
    sidebar: '#111113',
    raised: '#17171a',
    hover: '#1f1f23',
    active: '#27272c',
    border: '#25252b',
    borderStrong: '#33333b',
    text: '#ececef',
    textDim: '#a4a4ad',
    textFaint: '#868690',
    accent: '#8a63f4'
  },
  daylight: {
    kind: 'light',
    bg: '#f4f4f7',
    sidebar: '#ececf1',
    raised: '#ffffff',
    hover: '#f0f0f4',
    active: '#e6e6ed',
    border: '#e2e2e9',
    borderStrong: '#cfcfda',
    text: '#17171c',
    textDim: '#4f4f5a',
    textFaint: '#696974',
    accent: '#7650e4'
  },
  // True black behind everything, for OLED screens and dark rooms.
  oled: {
    kind: 'dark',
    bg: '#000000',
    sidebar: '#000000',
    raised: '#0f0f11',
    hover: '#18181b',
    active: '#202024',
    border: '#1d1d21',
    borderStrong: '#2c2c32',
    text: '#ececef',
    textDim: '#a4a4ad',
    textFaint: '#85858f',
    accent: '#8a63f4'
  },
  // Catppuccin's own colours: Mocha (base, mantle, surfaces, text, mauve)…
  mocha: {
    kind: 'dark',
    bg: '#1e1e2e',
    sidebar: '#181825',
    raised: '#252537',
    hover: '#2d2d40',
    active: '#313244',
    border: '#313244',
    borderStrong: '#45475a',
    text: '#cdd6f4',
    textDim: '#bac2de',
    textFaint: '#9399b2',
    accent: '#cba6f7'
  },
  // …and Latte.
  latte: {
    kind: 'light',
    bg: '#eff1f5',
    sidebar: '#e6e9ef',
    raised: '#fbfbfd',
    hover: '#e6e9ef',
    active: '#dce0e8',
    border: '#dce0e8',
    borderStrong: '#bcc0cc',
    text: '#4c4f69',
    textDim: '#5c5f77',
    textFaint: '#64677c',
    accent: '#8839ef'
  },
  // Tokyo Night's storm blues.
  tokyo: {
    kind: 'dark',
    bg: '#1a1b26',
    sidebar: '#16161e',
    raised: '#1f2335',
    hover: '#24283b',
    active: '#292e42',
    border: '#292e42',
    borderStrong: '#3b4261',
    text: '#c0caf5',
    textDim: '#a9b1d6',
    textFaint: '#8a92bb',
    accent: '#7aa2f7'
  },
  // Warm, low light: browns and an orange glow.
  ember: {
    kind: 'dark',
    bg: '#110e0c',
    sidebar: '#15110e',
    raised: '#1c1713',
    hover: '#241e19',
    active: '#2d261f',
    border: '#2a231d',
    borderStrong: '#3a3028',
    text: '#f1e9e2',
    textDim: '#bcafa3',
    textFaint: '#998c80',
    accent: '#f0954f'
  }
};

/**
 * The themes, in the order shown. Each is a family of looks; `card` is the
 * look its card shows when it stands for one kind only (OpenSave Night and
 * Daylight are one family, shown as two cards, as they always were two
 * choices).
 */
export const THEMES_BUILT_IN = [
  { id: 'night', family: 'opensave', look: 'night' },
  { id: 'oled', family: 'oled', look: 'oled' },
  { id: 'mocha', family: 'catppuccin', look: null },
  { id: 'tokyo', family: 'tokyo', look: 'tokyo' },
  { id: 'ember', family: 'ember', look: 'ember' },
  { id: 'daylight', family: 'opensave', look: 'daylight' }
];

/** The families: their dark look and their light one, where they have it. */
export const FAMILIES = {
  opensave: { dark: 'night', light: 'daylight' },
  oled: { dark: 'oled' },
  catppuccin: { dark: 'mocha', light: 'latte' },
  tokyo: { dark: 'tokyo' },
  ember: { dark: 'ember' }
};

export const DEFAULT_FAMILY = 'opensave';

/** Whether a family has both a dark and a light look to choose between. */
export const hasBoth = (family) => !!(FAMILIES[family]?.dark && FAMILIES[family]?.light);

/**
 * The look in use: the family's look for the kind asked for — dark, light,
 * or the system's when following it — or its only one.
 */
export function lookFor(family, mode, prefersDark = true) {
  const f = FAMILIES[family] ?? FAMILIES[DEFAULT_FAMILY];
  const kind = mode === 'system' ? (prefersDark ? 'dark' : 'light') : mode === 'light' ? 'light' : 'dark';
  return f[kind] ?? f.dark ?? f.light;
}

/** Whether a theme's card is the one in use. */
export function isCurrent(theme, family, look) {
  return theme.family === family && (theme.look === null || theme.look === look);
}

/** What picking a theme's card changes: its family, and — for a card that
 *  stands for one kind (Night, Daylight) — that kind, so it shows at once. */
export function pick(theme, appearance) {
  const next = { ...appearance, family: theme.family, accent: 'theme' };
  if (theme.look) {
    const kind = LOOKS[theme.look].kind;
    if (hasBoth(theme.family)) next.theme = kind;
  }
  return next;
}

/** The CSS variables a look sets (app.css names them). */
export function lookVars(look) {
  const l = LOOKS[look] ?? LOOKS.night;
  return {
    '--bg': l.bg,
    '--bg-sidebar': l.sidebar,
    '--bg-raised': l.raised,
    '--bg-hover': l.hover,
    '--bg-active': l.active,
    '--border': l.border,
    '--border-strong': l.borderStrong,
    '--text': l.text,
    '--text-dim': l.textDim,
    '--text-faint': l.textFaint,
    '--glass': `rgba(${hexRgb(l.raised)}, ${l.kind === 'dark' ? 0.72 : 0.78})`
  };
}

const hexRgb = (hex) => {
  const n = parseInt(hex.slice(1), 16);
  return [(n >> 16) & 255, (n >> 8) & 255, n & 255].join(', ');
};
