// One action at a time on a screen, with the outcome announced.
//
// A game's page has a dozen buttons that each call the daemon — sync,
// snapshot, restore, switch branch — and while one is running the others
// wait: two restores racing over the same save folder is the kind of thing
// nobody means to do. `busy` disables the buttons; `run` does the rest.
import { writable } from 'svelte/store';
import { toast } from './stores.js';

export function createRunner(notify = toast) {
  const busy = writable(false);
  let running = false;

  /** Runs fn unless something else is running. A label is announced on
   *  success; a failure is announced with the daemon's own message, and
   *  offers to try again. `shown` goes to both toasts (lib/toasts.js): a
   *  `key` to finish a toast already under way, an `action` for the done
   *  one. Says whether it ran and succeeded. */
  async function run(label, fn, shown = {}) {
    if (running) return false;
    running = true;
    busy.set(true);
    try {
      await fn();
      if (label) notify(label, 'success', shown);
      return true;
    } catch (e) {
      const { action, ...rest } = shown;
      notify(e.message, 'error', { ...rest, retry: () => run(label, fn, shown) });
      return false;
    } finally {
      running = false;
      busy.set(false);
    }
  }

  return { busy, run };
}
