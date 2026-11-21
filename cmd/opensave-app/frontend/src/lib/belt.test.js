import { describe, expect, it } from 'vitest';
import { beltSlots, beltX, beltRest, inView, wrap } from './belt.js';

describe('beltSlots', () => {
  it('fits four, then three, then two as the window narrows', () => {
    expect(beltSlots(1920)).toBe(4);
    expect(beltSlots(1181)).toBe(4);
    expect(beltSlots(1180)).toBe(3);
    expect(beltSlots(861)).toBe(3);
    expect(beltSlots(860)).toBe(2);
  });
});

describe('wrap', () => {
  it('is never negative', () => {
    expect(wrap(-1, 5)).toBe(4);
    expect(wrap(7, 5)).toBe(2);
    expect(wrap(3, 0)).toBe(0);
  });
});

describe('beltX', () => {
  const step = 100;

  it('lays the tiles out in order when it has not moved, the one that does not fit waiting off the left', () => {
    expect([0, 1, 2, 3, 4].map((i) => beltX(i, 5, step, 0))).toEqual([0, 100, 200, 300, -100]);
    expect([0, 1, 2, 3, 4, 5].map((i) => beltX(i, 6, step, 0))).toEqual([0, 100, 200, 300, 400, -100]);
  });

  it('moves every tile together, and the first comes round to the end', () => {
    expect([0, 1, 2, 3, 4].map((i) => beltX(i, 5, step, 50))).toEqual([-50, 50, 150, 250, 350]);
    expect([0, 1, 2, 3, 4].map((i) => beltX(i, 5, step, 150))).toEqual([350, -50, 50, 150, 250]);
  });

  it('wraps the other way too', () => {
    // Dragged right by one tile: the last comes round to the front.
    expect([0, 1, 2, 3, 4].map((i) => beltX(i, 5, step, -100))).toEqual([100, 200, 300, -100, 0]);
  });

  it('jumps from one end to the other only out of sight', () => {
    // Four shown, tiles 84 wide with a 16 gap (step 100), the row 384 wide —
    // the real shape, where the seam falls in a gap. With one tile more than
    // fits, or several, wherever the belt is and whichever way it moves, no
    // tile that is even partly in view moves by more than the drag.
    const gap = 16;
    const tile = step - gap;
    const width = 4 * step - gap;
    const seen = (x) => x + tile > 0 && x < width;
    for (const n of [5, 6, 9]) {
      for (const move of [7, -7]) {
        for (let offset = -1000; offset < 1000; offset += 7) {
          for (let i = 0; i < n; i++) {
            const a = beltX(i, n, step, offset);
            const b = beltX(i, n, step, offset + move);
            if (seen(a) || seen(b)) expect(Math.abs(b - a), `n=${n} offset=${offset} move=${move} tile ${i}`).toBe(7);
          }
        }
      }
    }
  });
});

describe('beltRest', () => {
  it('settles on the nearest tile', () => {
    expect(beltRest(140, 100)).toBe(100);
    expect(beltRest(160, 100)).toBe(200);
    expect(beltRest(-160, 100)).toBe(-200);
  });

  it('lets a flick carry it on a tile', () => {
    expect(beltRest(120, 100, 0.5)).toBe(200);
    expect(beltRest(120, 100, -0.5)).toBe(0);
  });

  it('stays put with no step', () => {
    expect(beltRest(37, 0)).toBe(37);
  });
});

describe('inView', () => {
  it('is true only for a tile wholly in the row', () => {
    expect(inView(0, 100, 400)).toBe(true);
    expect(inView(300, 100, 400)).toBe(true);
    expect(inView(-50, 100, 400)).toBe(false);
    expect(inView(350, 100, 400)).toBe(false);
  });
});
