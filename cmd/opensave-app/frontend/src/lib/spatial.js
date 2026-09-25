// Moving focus by direction, the way a controller moves through a console's
// menus: from the control in focus to the nearest one that way.
//
// Nearest means along the direction first, and across it second, weighted so
// a control straight ahead beats a closer one off to the side — pressing
// right from a tile goes to the next tile in the row, not to the sidebar entry
// that happens to be nearer as the crow flies. Off to the side is the gap
// between the two, not between their middles: a wide box straight below a tab
// is straight below it, however far its middle is.

/** @typedef {{left: number, top: number, right: number, bottom: number}} Rect */

const centre = (r) => ({ x: (r.left + r.right) / 2, y: (r.top + r.bottom) / 2 });

/**
 * The index of the rect to go to from `from`, heading `dir` ('up', 'down',
 * 'left' or 'right'), among `rects`; -1 when nothing lies that way.
 * @param {Rect[]} rects
 * @param {Rect} from
 * @param {'up'|'down'|'left'|'right'} dir
 */
export function nextInDirection(rects, from, dir) {
  const c = centre(from);
  let best = -1;
  let bestScore = Infinity;
  rects.forEach((r, i) => {
    if (r === from) return;
    const p = centre(r);
    // How far its middle is past the edge being moved across; how far it is
    // that way from edge to edge; how far off the line its middle is; and the
    // gap between the two across the line, nought when they are level.
    let ahead;
    let along;
    let across;
    let gap;
    switch (dir) {
      case 'right':
        ahead = p.x - from.right;
        along = r.left - from.right;
        across = Math.abs(p.y - c.y);
        gap = Math.max(0, r.top - from.bottom, from.top - r.bottom);
        break;
      case 'left':
        ahead = from.left - p.x;
        along = from.left - r.right;
        across = Math.abs(p.y - c.y);
        gap = Math.max(0, r.top - from.bottom, from.top - r.bottom);
        break;
      case 'down':
        ahead = p.y - from.bottom;
        along = r.top - from.bottom;
        across = Math.abs(p.x - c.x);
        gap = Math.max(0, r.left - from.right, from.left - r.right);
        break;
      case 'up':
        ahead = from.top - p.y;
        along = from.top - r.bottom;
        across = Math.abs(p.x - c.x);
        gap = Math.max(0, r.left - from.right, from.left - r.right);
        break;
      default:
        return;
    }
    // That way means past the edge, and no further off to the side than it
    // is ahead: at the end of a row, right does not reach round the corner to
    // a button above it.
    if (ahead <= 0 || gap > ahead) return;
    // The middles only break ties between controls level with each other.
    const score = Math.max(0, along) + gap * 3 + across / 4;
    if (score < bestScore) {
      bestScore = score;
      best = i;
    }
  });
  return best;
}

/** Whatever is in front of the page: a dialog, the palette, a menu. Focus
 *  stays inside the frontmost one, and B closes it rather than going back. */
export const LAYERS = '.overlay, .backdrop[role="presentation"], .menu, [role="dialog"]';

const FOCUSABLE =
  'button:not([disabled]), a[href], input:not([disabled]):not([type="hidden"]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

/** What focus can move among now: inside the dialog in front if one is
 *  open, else the whole window; only what is on screen and not hidden. */
export function focusables(doc = globalThis.document) {
  const dialogs = doc.querySelectorAll(LAYERS);
  const scope = dialogs.length ? dialogs[dialogs.length - 1] : doc.body;
  return [...scope.querySelectorAll(FOCUSABLE)].filter((el) => {
    if (el.closest('[hidden], [aria-hidden="true"]')) return false;
    const r = el.getBoundingClientRect();
    return r.width > 0 && r.height > 0;
  });
}

/** Moves focus one step `dir` from what is focused, and says whether it did. */
export function moveFocus(dir, doc = globalThis.document) {
  const els = focusables(doc);
  if (els.length === 0) return false;
  const current = doc.activeElement;
  if (!current || current === doc.body || !els.includes(current)) {
    // Nothing in focus yet: start in the page, not in the sidebar.
    const start = els.find((el) => el.closest('main')) ?? els[0];
    start.focus();
    start.scrollIntoView?.({ block: 'nearest' });
    return true;
  }
  const rects = els.map((el) => el.getBoundingClientRect());
  const i = nextInDirection(rects, rects[els.indexOf(current)], dir);
  if (i < 0) return false;
  els[i].focus();
  els[i].scrollIntoView?.({ block: 'nearest', inline: 'nearest' });
  return true;
}
