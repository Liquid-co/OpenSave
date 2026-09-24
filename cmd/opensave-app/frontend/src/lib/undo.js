// Undo in place of "Are you sure?".
//
// A confirmation asks before every untrack, exclusion or dismissal, and is
// answered Yes out of habit — so it protects against nothing and slows down
// everything. For an action that can simply not happen yet, the better guard
// is to let it go ahead at once, as far as the screen shows, and carry it out
// a few seconds later unless Undo is pressed.
//
// Deliberately not a real action followed by a reversing one: re-tracking a
// game is not the same as never untracking it (it starts a new sync history),
// and a quit in the waiting seconds should leave things as they were, not
// half-done. Actions that cannot be delayed or undone — deleting a snapshot,
// unpairing — keep their confirmation.
import { get, writable } from 'svelte/store';
import { toast } from './stores.js';

export const UNDO_DELAY = 6000;

/** Keys (game ids, paths) of things on their way out: views leave them out
 *  as if the action had already happened. */
export const hiddenKeys = writable(new Set());

const hide = (keys) => hiddenKeys.update((s) => new Set([...s, ...keys]));
const unhide = (keys) =>
  hiddenKeys.update((s) => {
    const next = new Set(s);
    for (const k of keys) next.delete(k);
    return next;
  });

/**
 * Announces an action with an Undo button and carries it out after `delay`
 * unless undone. `keys` are hidden meanwhile. `stillThere(key)` says whether
 * the thing a key names is still listed; once the action is done its keys
 * stay hidden until the daemon's update removes the thing itself, so nothing
 * flickers back in between. Resolves 'done', 'undone' or 'failed'.
 */
export function withUndo({ message, keys = [], run, delay = UNDO_DELAY, notify = toast, stillThere = () => false }) {
  return new Promise((resolve) => {
    let settled = false;
    hide(keys);

    const finish = async (undone) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      if (undone) {
        unhide(keys);
        resolve('undone');
        return;
      }
      try {
        await run();
      } catch (e) {
        unhide(keys);
        notify(e.message, 'error');
        resolve('failed');
        return;
      }
      releaseWhenGone(keys, stillThere);
      resolve('done');
    };

    const timer = setTimeout(() => finish(false), delay);
    notify(message, 'info', { ttl: delay, action: { label: 'Undo', run: () => finish(true) } });
  });
}

// Unhides keys once nothing they name is listed any more — or after a few
// seconds regardless, so a key can never stay hidden for the rest of the
// session (a game tracked again later may come back under the same id).
function releaseWhenGone(keys, stillThere, { every = 250, giveUpAfter = 5000 } = {}) {
  const started = Date.now();
  const check = () => {
    if (keys.some(stillThere) && Date.now() - started < giveUpAfter) {
      setTimeout(check, every);
      return;
    }
    unhide(keys);
  };
  check();
}

/** Whether a key is currently hidden (for code outside a component). */
export const isHidden = (key) => get(hiddenKeys).has(key);
