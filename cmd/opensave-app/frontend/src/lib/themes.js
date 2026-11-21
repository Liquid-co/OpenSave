// Whole-app themes: the colours of every surface and of text, and an accent
// of their own. A theme is a family of looks — dark, light, or both — and
// the dark / light / match-system choice picks between them where it has
// both. lib/appearance.js puts the one in use on the page; themes people
// make themselves are lib/customthemes.js.
//
// OpenSave's own, each a different mood rather than the same dark grey in
// another tint: borrowing nobody's names, though several nod to palettes
// people love (Nord's ice, Gruvbox's warmth, Everforest's greens, Rosé Pine's
// petals) and to hardware (a CRT's glow, a handheld's green screen).
//
// A look gives the surfaces and text; what says what kind of thing
// something is (success, warning, the device colours) stays the app's own
// for dark or light (app.css), so it means the same in every theme.

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
  blackout: {
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
    textFaint: '#868690',
    accent: '#e2e2e8'
  },
  // A CRT left on in a dark arcade: indigo dark, a hot pink glow.
  arcade: {
    kind: 'dark',
    bg: '#120f24',
    sidebar: '#0e0b1d',
    raised: '#1a1633',
    hover: '#221d40',
    active: '#2b2550',
    border: '#2a2448',
    borderStrong: '#3d3566',
    text: '#f3efff',
    textDim: '#bdb4e0',
    textFaint: '#9d93c8',
    accent: '#ff4f9a'
  },
  // An old handheld's screen: four shades of green, lit…
  handheld: {
    kind: 'light',
    bg: '#c8d3a2',
    sidebar: '#bdc994',
    raised: '#d6dfb4',
    hover: '#c2ce9a',
    active: '#b6c389',
    border: '#aab97c',
    borderStrong: '#8e9e60',
    text: '#17220f',
    textDim: '#2c3b22',
    textFaint: '#3a4b2e',
    accent: '#2e5a26'
  },
  // …and with the lights off.
  handheldDark: {
    kind: 'dark',
    bg: '#0e1910',
    sidebar: '#0a130c',
    raised: '#142116',
    hover: '#1a2a1d',
    active: '#213425',
    border: '#1e3022',
    borderStrong: '#2e4733',
    text: '#dcf2c9',
    textDim: '#a9c99a',
    textFaint: '#88a97b',
    accent: '#9bd65a'
  },
  // Arctic blue-greys, cool and quiet…
  frost: {
    kind: 'dark',
    bg: '#1c212b',
    sidebar: '#181c25',
    raised: '#232936',
    hover: '#2a3141',
    active: '#323a4c',
    border: '#2c3343',
    borderStrong: '#3e4759',
    text: '#eceff4',
    textDim: '#c5ccd8',
    textFaint: '#a0aabb',
    accent: '#88c0d0'
  },
  // …and snow in daylight.
  frostDay: {
    kind: 'light',
    bg: '#eef2f6',
    sidebar: '#e4e9f0',
    raised: '#fafcfd',
    hover: '#e4e9f0',
    active: '#d8dfe8',
    border: '#d8dfe8',
    borderStrong: '#bcc6d3',
    text: '#232a36',
    textDim: '#3e4859',
    textFaint: '#566175',
    accent: '#3a7894'
  },
  // Warm, low light, like an old game by a fire…
  campfire: {
    kind: 'dark',
    bg: '#1d1a17',
    sidebar: '#181512',
    raised: '#26221e',
    hover: '#2f2a25',
    active: '#39332c',
    border: '#332d27',
    borderStrong: '#4a4138',
    text: '#f0e3cf',
    textDim: '#c9b79b',
    textFaint: '#ab9a80',
    accent: '#f2a33c'
  },
  // …and the same warmth on old paper.
  campfireDay: {
    kind: 'light',
    bg: '#f6efe2',
    sidebar: '#efe5d3',
    raised: '#fdf9f1',
    hover: '#efe5d3',
    active: '#e5d8c1',
    border: '#e4d6be',
    borderStrong: '#cdbb9c',
    text: '#3a2f24',
    textDim: '#5a4a38',
    textFaint: '#6c5a45',
    accent: '#a85c16'
  },
  // A forest floor: soft greens, easy on tired eyes…
  moss: {
    kind: 'dark',
    bg: '#1b201c',
    sidebar: '#171b18',
    raised: '#222823',
    hover: '#29302a',
    active: '#313a32',
    border: '#2c342d',
    borderStrong: '#3e4a40',
    text: '#e3e8dc',
    textDim: '#b9c3b0',
    textFaint: '#98a38f',
    accent: '#a7c080'
  },
  // …and in sunlight.
  mossDay: {
    kind: 'light',
    bg: '#f2f1e6',
    sidebar: '#e9e8da',
    raised: '#fbfaf3',
    hover: '#e9e8da',
    active: '#dddcc9',
    border: '#ddddc9',
    borderStrong: '#c3c2a8',
    text: '#2f3a2c',
    textDim: '#485645',
    textFaint: '#5a6756',
    accent: '#557638'
  },
  // Dusk and petals: muted plum, a rose accent…
  petal: {
    kind: 'dark',
    bg: '#1d1820',
    sidebar: '#18141b',
    raised: '#261f2a',
    hover: '#2e2633',
    active: '#372e3d',
    border: '#322a37',
    borderStrong: '#473c4f',
    text: '#f1e6ee',
    textDim: '#cbb8c6',
    textFaint: '#ab98a6',
    accent: '#eb8fb4'
  },
  // …and blossom in the morning.
  petalDay: {
    kind: 'light',
    bg: '#faf3f2',
    sidebar: '#f3e9e8',
    raised: '#fffaf9',
    hover: '#f3e9e8',
    active: '#ebdddc',
    border: '#ecdedd',
    borderStrong: '#d6c2c1',
    text: '#3a2a33',
    textDim: '#5b4651',
    textFaint: '#715a66',
    accent: '#b54677'
  },
  // The deep sea: near-black teal, an aqua light.
  abyss: {
    kind: 'dark',
    bg: '#0b1418',
    sidebar: '#081114',
    raised: '#0f1c22',
    hover: '#14252c',
    active: '#1a2f37',
    border: '#172a31',
    borderStrong: '#234049',
    text: '#def4f5',
    textDim: '#a6c9cc',
    textFaint: '#85aaae',
    accent: '#3fd2c7'
  }
};

/** The families: their dark look and their light one, where they have it,
 *  in the order their cards are shown. */
export const FAMILIES = {
  opensave: { dark: 'night', light: 'daylight' },
  blackout: { dark: 'blackout' },
  arcade: { dark: 'arcade' },
  handheld: { dark: 'handheldDark', light: 'handheld' },
  frost: { dark: 'frost', light: 'frostDay' },
  campfire: { dark: 'campfire', light: 'campfireDay' },
  moss: { dark: 'moss', light: 'mossDay' },
  petal: { dark: 'petal', light: 'petalDay' },
  abyss: { dark: 'abyss' }
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

/** What picking a theme changes: the family, and its own accent. Light or
 *  dark stays as chosen, for the family that has both. */
export function pick(family, appearance) {
  return { ...appearance, family, accent: 'theme' };
}

/** The CSS variables a look sets (app.css names them). `look` is an id in
 *  LOOKS or a look itself (a theme of someone's own). */
export function lookVars(look) {
  const l = typeof look === 'string' ? (LOOKS[look] ?? LOOKS.night) : look;
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
