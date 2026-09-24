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
   *  success; a failure is announced with the daemon's own message. */
  async function run(label, fn) {
    if (running) return;
    running = true;
    busy.set(true);
    try {
      await fn();
      if (label) notify(label, 'success');
    } catch (e) {
      notify(e.message, 'error');
    } finally {
      running = false;
      busy.set(false);
    }
  }

  return { busy, run };
}
