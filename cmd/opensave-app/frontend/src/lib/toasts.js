// Feedback toasts: what just happened, to what, and the one thing you might
// do next (the ToastsV4 board).
//
// The rules they keep:
// - Done and info go after four seconds. Pointing at the toasts, or
//   focusing one, holds them — and holds whatever waits on one: an Undo is carried out when
//   its toast runs out, not on a clock of its own, so the countdown on the
//   button is the truth.
// - Errors and things under way stay until closed or finished. Any toast
//   can be closed early; one under way that is closed stays closed for the
//   rest of its progress, and comes back only to say how it ended.
// - Something already under way updates its own toast (same `key`) rather
//   than adding another, and ends as done in the same place.
// - A title and a second line: given, or the message split at its dash.
//
// Where they are drawn, and the stack they fold into past three, is
// components/Toasts.svelte.
import { writable } from 'svelte/store';

/** The key of a game's restore toast: the progress the daemon reports and
 *  the answer to the request share it, so they are one toast. */
export const restoreKey = (gameId) => `restore:${gameId}`;

/** How long done and info toasts stay. */
export const TOAST_LIFE = 4000;
/** More than this many fold into a stack. */
export const MAX_SHOWN = 3;

const DASH = /^([\s\S]+?)\s+—\s+([\s\S]+)$|^([\s\S]+?)——([\s\S]+)$/;

/** A message as a title and a second line: what comes after its first
 *  dash goes underneath. "Update failed — the disk is full" reads as
 *  "Update failed", then "The disk is full". Each starts with a capital:
 *  the daemon's own messages, shown as they come, start in lower case. */
export function splitMessage(message) {
  const s = String(message ?? '').trim();
  const m = s.match(DASH);
  if (!m) return { title: capital(s), detail: '' };
  return { title: capital((m[1] ?? m[3]).trim()), detail: capital((m[2] ?? m[4]).trim()) };
}
const capital = (s) => s.charAt(0).toLocaleUpperCase() + s.slice(1);

/** How long a toast of a kind stays, in ms; Infinity until closed. */
export function lifeOf(kind, ttl) {
  if (ttl) return ttl;
  return kind === 'error' || kind === 'progress' ? Infinity : TOAST_LIFE;
}

// What a toast shows, from a message and its options. Everything else a
// toast carries is added by the store: `life`, and `left` as of `woundAt`
// (for drawing a countdown), `round`, counting each time its countdown
// starts again from the top, and `held` while pointed at or focused.
function words(message, opts) {
  if (opts.title) return { title: opts.title, detail: opts.detail ?? '' };
  const w = splitMessage(message);
  return opts.detail ? { ...w, detail: opts.detail } : w;
}
const SHOWN = ['game', 'icon', 'action', 'retry', 'undo', 'progress'];

/**
 * The toast list as a store, with what can be done to it. The clock is
 * passed in so tests can run it.
 */
export function createToasts({ setTimer = setTimeout, clearTimer = clearTimeout, now = () => Date.now() } = {}) {
  const list = writable([]);
  let current = [];
  list.subscribe((v) => (current = v));
  let seq = 0;
  // id -> { left, since, handle }: time left as of `since`, and the timer
  // running it out (none while held).
  const clocks = new Map();
  // id -> { onExpire, onClose }, kept out of the store's objects.
  const hooks = new Map();
  // Keys of toasts under way that were closed: their progress is not shown
  // again, only how they end.
  const quiet = new Set();

  const find = (id) => current.find((t) => t.id === id);
  const replace = (id, fn) => list.update((all) => all.map((t) => (t.id === id ? fn(t) : t)));

  // Starts (or restarts) a toast's countdown from `left`; held, it only
  // remembers what is left.
  function wind(id, left, held) {
    const old = clocks.get(id);
    if (old?.handle) clearTimer(old.handle);
    clocks.delete(id);
    if (!Number.isFinite(left)) return;
    const clock = { left, since: now(), handle: null };
    if (!held) clock.handle = setTimer(() => end(id, 'expired'), left);
    clocks.set(id, clock);
  }
  const remaining = (c) => (c.handle ? Math.max(c.left - (now() - c.since), 0) : c.left);

  /** Shows a toast, or — given the `key` of one still shown — updates that
   *  one in place. Returns its id. */
  function show(message, kind = 'info', opts = {}) {
    if (opts.key != null && quiet.has(opts.key)) {
      if (kind === 'progress') return null;
      quiet.delete(opts.key);
    }
    const same = opts.key != null ? current.find((t) => t.key === opts.key) : null;
    if (same) {
      change(same.id, { ...opts, message, kind });
      return same.id;
    }
    const id = ++seq;
    const life = lifeOf(kind, opts.ttl);
    const toast = { id, key: opts.key ?? null, kind, ...words(message, opts), life, left: life, woundAt: now(), round: 0, held: false };
    for (const k of SHOWN) toast[k] = k === 'undo' ? !!opts.undo : (opts[k] ?? null);
    hooks.set(id, { onExpire: opts.onExpire, onClose: opts.onClose });
    list.update((all) => [...all, toast]);
    wind(id, life, false);
    return id;
  }

  /** Changes a toast where it is: new words, a new kind, progress. Its time
   *  starts again when the kind or ttl changes, so a toast that was under way
   *  stays its full time once it is done. */
  function change(id, patch) {
    const t = find(id);
    if (!t) return;
    const kind = patch.kind ?? t.kind;
    const restart = kind !== t.kind || patch.ttl != null;
    const next = { ...t, kind };
    if (patch.message != null || patch.title != null) Object.assign(next, words(patch.message, patch));
    else if (patch.detail != null) next.detail = patch.detail;
    for (const k of SHOWN) if (k in patch) next[k] = k === 'undo' ? !!patch.undo : patch[k];
    if (restart) {
      next.life = lifeOf(kind, patch.ttl);
      next.left = next.life;
      next.woundAt = now();
      next.round = t.round + 1;
      wind(id, next.life, t.held);
    }
    replace(id, () => next);
    const h = hooks.get(id) ?? {};
    if (patch.onExpire) h.onExpire = patch.onExpire;
    if (patch.onClose) h.onClose = patch.onClose;
    hooks.set(id, h);
  }

  function end(id, why) {
    const t = find(id);
    if (!t) return;
    if (why === 'closed' && t.kind === 'progress' && t.key != null) quiet.add(t.key);
    wind(id, Infinity, true);
    const h = hooks.get(id);
    hooks.delete(id);
    list.update((all) => all.filter((t) => t.id !== id));
    if (why === 'expired') h?.onExpire?.();
    else if (why === 'closed') h?.onClose?.();
  }

  /** Holds a toast: the pointer is on it, or focus is in it. */
  function pause(id) {
    const t = find(id);
    if (!t || t.held) return;
    const c = clocks.get(id);
    const left = c ? remaining(c) : t.left;
    if (c) wind(id, left, true);
    replace(id, (x) => ({ ...x, held: true, left, woundAt: now() }));
  }
  /** Lets it run out again, from where it was held. */
  function resume(id) {
    const t = find(id);
    if (!t || !t.held) return;
    const c = clocks.get(id);
    if (c) wind(id, c.left, false);
    replace(id, (x) => ({ ...x, held: false, left: c ? c.left : x.left, woundAt: now() }));
  }

  return {
    subscribe: list.subscribe,
    show,
    change,
    /** Closed by the person: an Undo waiting on it goes ahead. */
    close: (id) => end(id, 'closed'),
    /** Taken away because what it offered was used (Undo pressed, an
     *  action run): nothing more happens. */
    remove: (id) => end(id, 'removed'),
    pause,
    resume
  };
}
