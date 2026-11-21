// A game's colour, taken from its cover art: for its page's Play button and
// tab underline, when Settings → Appearance says so (coverColours). Never
// used as-is: the same rules as the accent make it readable — a fill its
// label reads on at 4.5:1, a text colour that reads on the page.
//
// Taken from the colourful part of the cover, not its average: a dark cover
// with one bright figure is that figure's colour, not brown. A cover with
// next to no colour in it (black and white, grey) gives none, and the page
// keeps the accent.
import { hsvToHex, accentFill, accentText, onAccent, hexToRgb, darken, lighten } from './appearance.js';

/**
 * The colour in RGBA pixels, or null when there is too little of it.
 * Each pixel counts by how colourful and how bright it is; hues are averaged
 * round the circle, so red at 350 and red at 10 make red, not cyan.
 */
export function tintFromPixels(data) {
  let x = 0;
  let y = 0;
  let sSum = 0;
  let vSum = 0;
  let total = 0;
  let pixels = 0;
  for (let i = 0; i + 3 < data.length; i += 4) {
    if (data[i + 3] < 128) continue;
    pixels++;
    const r = data[i] / 255;
    const g = data[i + 1] / 255;
    const b = data[i + 2] / 255;
    const max = Math.max(r, g, b);
    const d = max - Math.min(r, g, b);
    const s = max ? d / max : 0;
    if (max < 0.18 || s < 0.22) continue;
    let h = 0;
    if (max === r) h = ((g - b) / d) % 6;
    else if (max === g) h = (b - r) / d + 2;
    else h = (r - g) / d + 4;
    h = ((h * 60 + 360) % 360) * (Math.PI / 180);
    const w = s * s * max;
    x += Math.cos(h) * w;
    y += Math.sin(h) * w;
    sSum += s * w;
    vSum += max * w;
    total += w;
  }
  // Under about a twentieth of the cover's worth of strong colour: none.
  if (!pixels || total < pixels * 0.05) return null;
  const hue = ((Math.atan2(y, x) * 180) / Math.PI + 360) % 360;
  const s = Math.min(0.85, Math.max(0.42, sSum / total));
  const v = Math.min(0.95, Math.max(0.72, vSum / total));
  return hsvToHex(hue, s, v);
}

/** The page's CSS variables for a colour, readable in the theme. */
export function tintVars(hex, theme = 'dark') {
  const fill = accentFill(hex);
  const on = onAccent(hex);
  return {
    '--game-accent': hex,
    '--game-accent-rgb': hexToRgb(hex).join(', '),
    '--game-fill': fill,
    '--game-on-fill': on,
    '--game-fill-hover': on === '#ffffff' ? darken(fill, 0.1) : lighten(fill, 0.1),
    '--game-text': accentText(hex, theme)
  };
}

const cache = new Map();

/**
 * A cover's colour (a hex), or null: read once per cover from a few dozen
 * pixels of it. The bytes are fetched, so the canvas is never tainted by an
 * image from another origin.
 */
export function coverTint(url) {
  if (!url) return Promise.resolve(null);
  if (!cache.has(url)) cache.set(url, read(url));
  return cache.get(url);
}

async function read(url) {
  try {
    const res = await fetch(url);
    if (!res.ok) return null;
    const bitmap = await createImageBitmap(await res.blob());
    const canvas = document.createElement('canvas');
    canvas.width = 24;
    canvas.height = 36;
    const c = canvas.getContext('2d', { willReadFrequently: true });
    c.drawImage(bitmap, 0, 0, 24, 36);
    bitmap.close?.();
    return tintFromPixels(c.getImageData(0, 0, 24, 36).data);
  } catch {
    return null;
  }
}
