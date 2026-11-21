import { describe, it, expect } from 'vitest';
import { stackPlaces, STACK } from './toaststack.js';

const order = (...ids) => ids.map((id) => ({ id }));

describe('stackPlaces', () => {
  it('puts each older toast a step behind the newest, cut to its height', () => {
    const { places, height } = stackPlaces(order('c', 'b', 'a'), { c: 60, b: 90, a: 40 });
    expect(places.c).toMatchObject({ y: 0, scale: 1, shown: true, height: 60, depth: 0 });
    expect(places.b).toMatchObject({ y: STACK.peek, shown: true, height: 60, depth: 1 });
    expect(places.a).toMatchObject({ y: STACK.peek * 2, shown: true, height: 60, depth: 2 });
    expect(places.b.scale).toBeLessThan(1);
    expect(places.a.scale).toBeLessThan(places.b.scale);
    expect(height).toBe(60 + STACK.peek * 2);
  });

  it('keeps those past the third out of sight, without growing the stack', () => {
    const { places, height } = stackPlaces(order('e', 'd', 'c', 'b', 'a'), {});
    expect(['e', 'd', 'c', 'b', 'a'].map((id) => places[id].shown)).toEqual([true, true, true, false, false]);
    expect(height).toBe(STACK.height + STACK.peek * 2);
  });

  it('stands them in a column when open, each at its own height', () => {
    const { places, height } = stackPlaces(order('c', 'b', 'a'), { c: 60, b: 90, a: 40 }, { open: true });
    expect(places.c).toMatchObject({ y: 0, scale: 1, height: 60 });
    expect(places.b).toMatchObject({ y: 60 + STACK.gap, scale: 1, height: 90 });
    expect(places.a).toMatchObject({ y: 60 + 90 + 2 * STACK.gap, height: 40 });
    expect(height).toBe(60 + 90 + 40 + 2 * STACK.gap);
  });

  it('has nothing to open with one toast, and no height with none', () => {
    expect(stackPlaces(order('a'), { a: 70 }, { open: true })).toEqual({
      places: { a: { y: 0, scale: 1, shown: true, height: 70, depth: 0 } },
      height: 70
    });
    expect(stackPlaces([], {})).toEqual({ places: {}, height: 0 });
  });
});
