// The window's frame around the pages: how much of the sidebar shows.
//
// Kept on this device, like the appearance (appearance.js): it is about this
// screen. Three sizes: the full sidebar, its icons only, or none. The button
// at the left of the title bar folds the full one to icons and back; Ctrl+B
// puts it away entirely and brings it back as it was.
import { writable } from 'svelte/store';

export const SIDEBAR_MODES = ['full', 'icons', 'hidden'];

// Below this width a full sidebar draws as icons, so the page keeps room.
// Chosen from the scaling rules: a 1024 px window still shows four covers.
export const FOLD_BELOW = 1100;

export const DEFAULT_SIDEBAR = Object.freeze({ mode: 'full', lastShown: 'full' });

/** A sidebar state with every field valid, whatever was stored. */
export function sanitizeSidebar(raw) {
  const v = raw && typeof raw === 'object' ? raw : {};
  const mode = SIDEBAR_MODES.includes(v.mode) ? v.mode : DEFAULT_SIDEBAR.mode;
  const lastShown = v.lastShown === 'icons' || v.lastShown === 'full' ? v.lastShown : mode === 'hidden' ? 'full' : mode;
  return { mode, lastShown: mode === 'hidden' ? lastShown : mode };
}

/** The title bar's button: full and icons swap; a hidden sidebar comes back. */
export function pressSidebarButton(state) {
  const s = sanitizeSidebar(state);
  if (s.mode === 'hidden') return { mode: s.lastShown, lastShown: s.lastShown };
  const mode = s.mode === 'full' ? 'icons' : 'full';
  return { mode, lastShown: mode };
}

/** Ctrl+B: away entirely, or back as it was. */
export function toggleSidebarHidden(state) {
  const s = sanitizeSidebar(state);
  if (s.mode === 'hidden') return { mode: s.lastShown, lastShown: s.lastShown };
  return { mode: 'hidden', lastShown: s.mode };
}

/** A mode picked by name (Settings, the button's menu). */
export function chooseSidebar(state, mode) {
  const s = sanitizeSidebar(state);
  if (!SIDEBAR_MODES.includes(mode)) return s;
  return mode === 'hidden' ? { mode, lastShown: s.mode === 'hidden' ? s.lastShown : s.mode } : { mode, lastShown: mode };
}

/** What is drawn at a window width: a full sidebar folds to icons when narrow. */
export function sidebarShown(state, width) {
  const { mode } = sanitizeSidebar(state);
  if (mode === 'full' && Number.isFinite(width) && width < FOLD_BELOW) return 'icons';
  return mode;
}

const KEY = 'opensave.sidebar';

export function loadSidebar(storage = globalThis.localStorage) {
  try {
    const saved = storage?.getItem(KEY);
    return saved ? sanitizeSidebar(JSON.parse(saved)) : { ...DEFAULT_SIDEBAR };
  } catch {
    return { ...DEFAULT_SIDEBAR };
  }
}

export function saveSidebar(v, storage = globalThis.localStorage) {
  try {
    storage?.setItem(KEY, JSON.stringify(sanitizeSidebar(v)));
  } catch {
    // Storage refused: the choice lasts until restart.
  }
}

export const sidebar = writable(loadSidebar());
sidebar.subscribe((v) => saveSidebar(v));

/** Whether the sync panel (title bar, top right) is open. A store so the
 *  status bar and the palette can open it too. */
export const syncPanelOpen = writable(false);
