// Folders dropped onto the window. The desktop shell hands over the paths
// (a browser would not: it keeps them from the page), and a dropped folder
// opens the Track card with the folder filled in, for a name to be checked
// before anything is tracked.
import { writable } from 'svelte/store';
import { navigate } from './stores.js';

/** The last folder of a path, on either kind of separator; '' for a root. */
export function folderName(path) {
  const trimmed = String(path ?? '').replace(/[\\/]+$/, '');
  const parts = trimmed.split(/[\\/]/);
  const last = parts[parts.length - 1] ?? '';
  return /^[A-Za-z]:$/.test(last) ? '' : last;
}

/** Whether a drag is over the window, for the "drop to track" overlay. */
export const dragging = writable(false);

/**
 * Listens for dropped files through the desktop runtime; a no-op in a
 * browser. The first dropped path is the one taken: tracking is one folder
 * at a time, and a drop of several is more likely a slip than a request.
 */
export function listenForDrops(runtime = globalThis.runtime) {
  if (!runtime?.OnFileDrop) return () => {};
  runtime.OnFileDrop((_x, _y, paths) => {
    dragging.set(false);
    const first = paths?.[0];
    if (first) navigate('home', { add: true, path: first, at: Date.now() });
  }, false);
  return () => runtime.OnFileDropOff?.();
}
