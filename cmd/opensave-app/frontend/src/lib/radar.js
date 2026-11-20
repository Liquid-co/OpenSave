// Where each device found nearby sits on Add a device's radar.
//
// Not random: a device's angle comes from a hash of its id, so the same Steam
// Deck always sits in the same place and nothing jumps about while the list
// fills in, yet no two radars look alike. Each device then takes the free
// spot that keeps it furthest from the ones already placed, and never one
// where it would touch another.
//
// Up to four: one ring, a dot with the name under it. Five or more: two rings
// of icons, the names in the list beside. Past eight, the first eight found
// are drawn and the rest counted ("+4"); the list has everyone.

export const RADAR_MAX = 8;
export const LABELLED_UP_TO = 4;

/** FNV-1a: the same id, the same number, on any device. */
export function hashId(s) {
  let h = 2166136261;
  for (const ch of String(s)) {
    h ^= ch.charCodeAt(0);
    h = Math.imul(h, 16777619) >>> 0;
  }
  return h;
}

const gap = (a, b) => Math.min(Math.abs(a - b), 360 - Math.abs(a - b));

/**
 * @param {{id: string, name: string}[]} devices in the order they were found
 * @param {{width: number, height: number}} box the radar's size in px
 * @returns {{labelled: boolean, placed: {device, angle: number, x: number, y: number}[], hidden: number}}
 */
export function placeOnRadar(devices, { width = 360, height = 330, max = RADAR_MAX } = {}) {
  const list = devices ?? [];
  const cx = width / 2;
  const cy = height / 2;
  const labelled = list.length <= LABELLED_UP_TO;
  const rings = labelled ? [{ r: 118, n: 8, off: 22.5 }] : [{ r: 100, n: 6, off: 0 }, { r: 146, n: 9, off: 20 }];
  const slots = [];
  rings.forEach((ring, ri) => {
    for (let i = 0; i < ring.n; i++) {
      const a = (ring.off + (360 / ring.n) * i) % 360;
      slots.push({ ri, a, x: cx + ring.r * Math.sin((a * Math.PI) / 180), y: cy - ring.r * Math.cos((a * Math.PI) / 180) });
    }
  });
  // What a dot (and its name, when labelled) covers.
  const boxOf = (d, x, y) => {
    if (!labelled) return { x0: x - 17, y0: y - 17, x1: x + 17, y1: y + 17 };
    const w = Math.min(Math.round(String(d.name).length * 6.6 + 12), 112);
    return { x0: x - w / 2, y0: y - 17, x1: x + w / 2, y1: y + 37 };
  };
  const hit = (a, b, m) => a.x0 < b.x1 + m && b.x0 < a.x1 + m && a.y0 < b.y1 + m && b.y0 < a.y1 + m;
  const centre = { x0: cx - 34, y0: cy - 34, x1: cx + 34, y1: cy + 34 };
  const inside = (b) => b.x0 >= 4 && b.y0 >= 4 && b.x1 <= width - 4 && b.y1 <= height - 4;
  const sep = (360 / Math.max(Math.min(list.length, max), 3)) * 0.7;
  const placed = [];
  const used = new Set();
  for (const d of list) {
    if (placed.length >= max) break;
    const h = hashId(d.id);
    const want = h % 360;
    const ringPref = (h >>> 9) % 2;
    const order = slots
      .map((s, i) => ({ s, i, cost: gap(s.a, want) + (s.ri === ringPref ? 0 : 25) + Math.max(0, sep - placed.reduce((m, p) => Math.min(m, gap(p.angle, s.a)), 360)) * 6 }))
      .sort((p, q) => p.cost - q.cost);
    for (const { s, i } of order) {
      if (used.has(i)) continue;
      const box = boxOf(d, s.x, s.y);
      if (!inside(box) || hit(box, centre, 6) || placed.some((p) => hit(box, p.box, 8))) continue;
      used.add(i);
      placed.push({ device: d, angle: s.a, x: Math.round(s.x), y: Math.round(s.y), box });
      break;
    }
  }
  return { labelled, placed: placed.map(({ box, ...p }) => p), hidden: list.length - placed.length };
}
