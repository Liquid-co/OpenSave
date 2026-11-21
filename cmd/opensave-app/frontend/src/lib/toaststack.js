// Where each toast sits in the stack (components/Toasts.svelte).
//
// Collapsed, the newest is in front and each older one is a step behind it:
// a little smaller, a little further back, so only its edge shows past the
// one in front. Past the third, they wait out of sight.
//
// Open, they stand in a column, newest nearest the edge.
//
// Toasts are all one height (STACK.height); `heights` (id -> px) can say
// otherwise for one, which the tests use. `order` is newest first. y grows away from the edge the toasts sit on: up
// from the bottom of the page, or down from the top of the window.

export const STACK = Object.freeze({ peek: 15, shrink: 0.05, layers: 3, gap: 8, height: 60 });

export function stackPlaces(order, heights, { open = false, opt = STACK } = {}) {
  const h = (id) => heights[id] ?? opt.height;
  const places = {};
  if (!order.length) return { places, height: 0 };
  const front = h(order[0].id);
  if (open && order.length > 1) {
    let y = 0;
    for (const t of order) {
      places[t.id] = { y, scale: 1, shown: true, height: h(t.id), depth: 0 };
      y += h(t.id) + opt.gap;
    }
    return { places, height: y - opt.gap };
  }
  order.forEach((t, i) => {
    const depth = Math.min(i, opt.layers);
    places[t.id] = {
      y: opt.peek * depth,
      scale: 1 - opt.shrink * depth,
      shown: i < opt.layers,
      height: front,
      depth
    };
  });
  return { places, height: front + opt.peek * Math.min(order.length - 1, opt.layers - 1) };
}
