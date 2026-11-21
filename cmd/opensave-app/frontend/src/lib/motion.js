// Whether things move: the Animations switch in Settings → General →
// Appearance, and never while the system asks apps to reduce motion.
//
// CSS reads the same answer from <html data-motion> (see applyAppearance and
// app.css). This is for Svelte's own transitions, which run from script and
// which CSS can only freeze, not skip: an outro stilled by CSS still keeps
// its element on screen for the length of the animation.
import { derived, get, readable } from 'svelte/store';
import { cubicIn, cubicOut } from 'svelte/easing';
import { flip } from 'svelte/animate';
import { appearance, motionAllowed } from './appearance.js';

const query = globalThis.matchMedia?.('(prefers-reduced-motion: reduce)');

/** True while the system asks apps to reduce motion. */
export const systemReducesMotion = readable(!!query?.matches, (set) => {
  const onChange = () => set(!!query?.matches);
  query?.addEventListener?.('change', onChange);
  return () => query?.removeEventListener?.('change', onChange);
});

export const motionOn = derived([appearance, systemReducesMotion], ([a, reduce]) => motionAllowed(a, reduce));

/** A Svelte transition that happens only while things may move, and is
 *  instant otherwise. */
export const gated =
  (transition, isOn = () => get(motionOn)) =>
  (node, params) =>
    isOn() ? transition(node, params) : { duration: 0 };

// ── The app's own movements ─────────────────────────────────────────────
// Each is gated: with motion off it takes no time at all, so leaving is
// instant as well as still.

/** A dialog leaving: the dimming fades and the panel sinks back the way it
 *  rose (dialog-in in app.css). Goes on the overlay; moves its first child.
 *
 *  Clicks go through it while it leaves. When the same dialog opens again
 *  before it has gone (closed, then reopened at once), Svelte stops the
 *  leaving and calls tick(1): everything must be put back then, pointer
 *  events too. They used to stay off, and the dialog that came back was
 *  drawn but could not be clicked or scrolled; the page behind it took the
 *  wheel instead. */
export const dialogOut = gated((node, { duration = 140 } = {}) => {
  const panel = node.firstElementChild;
  // From the moment it starts to leave, not from its first frame: a click
  // just after closing belongs to what is underneath.
  node.style.pointerEvents = 'none';
  return {
    duration,
    easing: cubicIn,
    tick: (t) => {
      node.style.opacity = String(t);
      node.style.pointerEvents = t < 1 ? 'none' : '';
      if (panel) panel.style.transform = t < 1 ? `translateY(${(1 - t) * 8}px) scale(${0.985 + 0.015 * t})` : '';
    }
  };
});

/** A panel opening out of the control it belongs to, and folding back into
 *  it: from its top-right corner by default, where the bell is. */
export const unfold = gated((node, { duration = 170, origin = 'top right', y = -6 } = {}) => ({
  duration,
  easing: cubicOut,
  css: (t, u) => `transform-origin: ${origin}; opacity: ${t}; transform: translateY(${u * y}px) scale(${0.95 + 0.05 * t});`
}));

/** Something leaving to the side it came from: a toast back off the edge. */
export const slideAway = gated((node, { duration = 160, x = 36 } = {}) => ({
  duration,
  easing: cubicIn,
  css: (t, u) => `opacity: ${t}; transform: translateX(${u * x}px);`
}));

// The selected child of a tab bar or segmented control, whatever marks it.
const SELECTED = '.active, .on, [aria-selected="true"], [aria-checked="true"], [aria-current="page"]';

/**
 * One marker that moves to whichever child is selected, rather than each
 * child drawing its own and the mark jumping between them.
 *
 * Svelte action, on the container. mode:
 *   - 'underline': a bar under the selected child (tab bars).
 *   - 'fill':      the selected child's whole box (segmented controls).
 *   - 'bar':       a bar beside it, inset top and bottom.
 * className is added to the marker, for a control that draws it its own way
 * (the sidebar's fill and accent bar together).
 * The container gets the class "slides" while this runs, which app.css uses
 * to put away each child's own mark; the marker is a span.slide-mark.
 *
 * It follows the selection by watching the children's attributes, so the
 * component marking them needs to know nothing about it. The first placement
 * and any resize are instant; only a change of selection moves, and with
 * motion off app.css stills that too.
 */
export function slidingIndicator(node, { mode = 'underline', inset = 0, offsetX = 0, className = '' } = {}) {
  const mark = document.createElement('span');
  mark.className = `slide-mark slide-${mode} ${className}`.trim();
  mark.setAttribute('aria-hidden', 'true');
  node.classList.add('slides');
  if (getComputedStyle(node).position === 'static') node.style.position = 'relative';
  node.prepend(mark);

  let current = null;

  function place(animate) {
    const target = [...node.children].find((c) => c !== mark && c.matches(SELECTED));
    if (!target) {
      mark.style.opacity = '0';
      current = null;
      return;
    }
    mark.classList.toggle('moving', animate && current !== null && target !== current);
    mark.style.opacity = '1';
    const x = target.offsetLeft;
    const y = target.offsetTop;
    if (mode === 'fill') {
      mark.style.width = `${target.offsetWidth}px`;
      mark.style.height = `${target.offsetHeight}px`;
      mark.style.transform = `translate(${x}px, ${y}px)`;
    } else if (mode === 'bar') {
      mark.style.height = `${Math.max(0, target.offsetHeight - inset * 2)}px`;
      mark.style.transform = `translate(${x + offsetX}px, ${y + inset}px)`;
    } else {
      mark.style.width = `${Math.max(0, target.offsetWidth - inset * 2)}px`;
      // Across the bar's bottom border, as each tab's own underline sat.
      mark.style.transform = `translate(${x + inset}px, ${y + target.offsetHeight - 1}px)`;
    }
    current = target;
  }

  // Only the selection is watched for: the marker's own style changes are
  // attribute changes on the container's subtree too, and would loop.
  const watch = new MutationObserver((records) => {
    if (records.some((r) => r.target !== mark)) place(true);
  });
  watch.observe(node, {
    subtree: true,
    childList: true,
    attributes: true,
    attributeFilter: ['class', 'aria-selected', 'aria-checked', 'aria-current']
  });
  const resize = new ResizeObserver(() => place(false));
  resize.observe(node);
  place(false);

  return {
    update(opts = {}) {
      ({ mode = mode, inset = inset, offsetX = offsetX } = opts);
      place(false);
    },
    destroy() {
      watch.disconnect();
      resize.disconnect();
      mark.remove();
      node.classList.remove('slides');
    }
  };
}

// ── The Library's movements ─────────────────────────────────────────────

/** Games finding their new places when the order or a filter changes:
 *  Svelte's flip, gated like the rest. Past `limit` items it is skipped —
 *  measuring hundreds of tiles every change costs more than it shows. */
export function gatedFlip(node, fromTo, { limit = Infinity, count = 0, ...params } = {}) {
  if (!get(motionOn) || count > limit) return { duration: 0 };
  return flip(node, fromTo, { duration: 260, easing: cubicOut, ...params });
}

/** Something arriving from just below, as the page itself does. */
export const rise = gated((node, { duration = 220, y = 8, delay = 0 } = {}) => ({
  duration,
  delay,
  easing: cubicOut,
  css: (t, u) => `opacity: ${t}; transform: translateY(${u * y}px);`
}));

/** Something arriving by moving only, never fading: it is readable from its
 *  first frame, so a slow frame never leaves a blank (the page's own arrival
 *  works the same way, app.css). For what appears on a screen's first paint;
 *  `rise` is for what appears because someone asked for it. */
export const arrive = gated((node, { duration = 260, y = 8, x = 0, delay = 0 } = {}) => ({
  duration,
  delay,
  easing: cubicOut,
  css: (t, u) => `transform: translate(${u * x}px, ${u * y}px);`
}));

/** The delay for the i-th of a run of things arriving one after another,
 *  capped so a long list does not keep the last waiting. */
export const stagger = (i, step = 30, max = 12) => Math.min(i, max) * step;

/** A strip opening its height and closing it again, rather than pushing
 *  everything below it in one jump. */
export const collapse = gated((node, { duration = 200 } = {}) => {
  const h = node.offsetHeight;
  return {
    duration,
    easing: cubicOut,
    css: (t) => `opacity: ${t}; height: ${t * h}px; overflow: hidden;`
  };
});

/** The sidebar coming and going (Ctrl+B): its width, from nothing. */
export const sidebarSlide = gated((node, { duration = 200 } = {}) => {
  const w = node.offsetWidth;
  return {
    duration,
    easing: cubicOut,
    css: (t) => `width: ${t * w}px; min-width: 0; overflow: hidden; opacity: ${0.4 + 0.6 * t};`
  };
});

/** A tile or a bar appearing in place: from a touch smaller, fading up. */
export const pop = gated((node, { duration = 200, delay = 0, start = 0.94 } = {}) => ({
  duration,
  delay,
  easing: cubicOut,
  css: (t) => `opacity: ${t}; transform: scale(${start + (1 - start) * t});`
}));
