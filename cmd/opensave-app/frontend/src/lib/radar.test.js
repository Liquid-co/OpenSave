import { describe, expect, it } from 'vitest';
import { placeOnRadar, hashId, RADAR_MAX } from './radar.js';

const names = ['Steam Deck OLED', 'Living Room PC', "Alex's Steam Deck", 'ROG Ally', 'Study Laptop', 'Office PC', 'Legion Go', 'Steam Deck', 'DESKTOP-7Q2KD', "Sam's PC", 'Travel Laptop', 'Steam Deck LCD'];
const devs = (n, seed = 'a') => Array.from({ length: n }, (_, i) => ({ id: `${seed}-${i}-${Math.random().toString(36).slice(2)}`, name: names[i % names.length] }));
const dist = (a, b) => Math.min(Math.abs(a - b), 360 - Math.abs(a - b));
const box = (p, labelled) =>
  labelled
    ? { x0: p.x - Math.min(p.device.name.length * 6.6 + 12, 112) / 2, x1: p.x + Math.min(p.device.name.length * 6.6 + 12, 112) / 2, y0: p.y - 17, y1: p.y + 37 }
    : { x0: p.x - 17, x1: p.x + 17, y0: p.y - 17, y1: p.y + 17 };
const overlaps = (a, b) => a.x0 < b.x1 && b.x0 < a.x1 && a.y0 < b.y1 && b.y0 < a.y1;

describe('placeOnRadar', () => {
  it('puts the same device in the same place every time', () => {
    const d = [{ id: 'deck-1', name: 'Steam Deck' }];
    expect(placeOnRadar(d).placed[0]).toEqual(placeOnRadar(d).placed[0]);
    expect(hashId('deck-1')).toBe(hashId('deck-1'));
  });

  it('never overlaps, and keeps two or three devices well apart', () => {
    for (const n of [2, 3, 4, 5, 8, 12]) {
      for (let t = 0; t < 300; t++) {
        const r = placeOnRadar(devs(n, String(t)));
        expect(r.placed.length).toBe(Math.min(n, RADAR_MAX));
        for (let i = 0; i < r.placed.length; i++) {
          for (let j = i + 1; j < r.placed.length; j++) {
            expect(overlaps(box(r.placed[i], r.labelled), box(r.placed[j], r.labelled))).toBe(false);
            if (n <= 3) expect(dist(r.placed[i].angle, r.placed[j].angle)).toBeGreaterThanOrEqual(90);
          }
        }
      }
    }
  });

  it('labels up to four, and counts what does not fit past eight', () => {
    expect(placeOnRadar(devs(4)).labelled).toBe(true);
    expect(placeOnRadar(devs(5)).labelled).toBe(false);
    const r = placeOnRadar(devs(12));
    expect([r.placed.length, r.hidden]).toEqual([8, 4]);
    expect(placeOnRadar([])).toEqual({ labelled: true, placed: [], hidden: 0 });
  });
});
