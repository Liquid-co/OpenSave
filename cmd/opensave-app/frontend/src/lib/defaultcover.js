// The picture a game gets when it has no cover art anywhere: a colour of its
// own, worked out from its name, so the same game looks the same on every
// screen and on every device, and two games side by side are told apart.

/** A hue (0-359) for a game's name. The same name always gives the same hue;
 *  case and the spaces around it do not change it. */
export function coverHue(name) {
  const s = String(name ?? '').trim().toLowerCase();
  // FNV-1a: small, and spreads names that differ by one letter far apart.
  let h = 0x811c9dc5;
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 0x01000193);
  }
  return (h >>> 0) % 360;
}
