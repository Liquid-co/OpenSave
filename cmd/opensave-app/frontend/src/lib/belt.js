// A conveyor belt of tiles: a row that holds more than it shows, dragged
// sideways and coming round again at the other end. Recently played uses it
// (views/home/RecentlyPlayed.svelte). Plain arithmetic, so it can be tested.

/** How many tiles fit across a window this wide: four, three, then two. */
export function beltSlots(windowWidth) {
  if (windowWidth > 1180) return 4;
  if (windowWidth > 860) return 3;
  return 2;
}

/** `a` modulo `n`, never negative. */
export const wrap = (a, n) => (n > 0 ? ((a % n) + n) % n : 0);

/**
 * Where tile `i` of `n` sits (its left edge, px), the belt moved on by
 * `offset` px with `step` px from one tile to the next. A tile leaving on
 * the left reappears on the right: positions run from -step up to just short
 * of (n - 1) * step, so with more tiles than fit, the jump from one end to
 * the other always happens out of sight.
 */
export function beltX(i, n, step, offset) {
  return wrap(i * step - offset + step, n * step) - step;
}

/**
 * Where a released belt comes to rest: the tile boundary nearest to where it
 * would coast to, a flick (`velocity`, px per ms, positive moving tiles to the
 * left) carrying it on for `coastMs`.
 */
export function beltRest(offset, step, velocity = 0, coastMs = 180) {
  if (!(step > 0)) return offset;
  return Math.round((offset + velocity * coastMs) / step) * step;
}

/** Whether a tile at `x`, `w` wide, is wholly inside a row `width` wide. */
export const inView = (x, w, width) => x >= -1 && x + w <= width + 1;
