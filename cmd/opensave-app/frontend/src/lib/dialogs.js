// How many dialogs are open, so a toast can keep clear of them: with one
// open, toasts move from the bottom of the page — where a dialog's buttons
// are — to the top of the window. Each dialog's own element carries
// `use:holdsDialog`.
import { writable } from 'svelte/store';

export const openDialogs = writable(0);

export function holdsDialog() {
  openDialogs.update((n) => n + 1);
  return { destroy: () => openDialogs.update((n) => Math.max(n - 1, 0)) };
}
