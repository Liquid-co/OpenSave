// Keyboard shortcuts: which keys do what, in one list, so the shortcuts
// sheet shows exactly what the handler does and cannot drift from it.
import { writable } from 'svelte/store';

/** Whether the Ctrl+K palette and the shortcuts list are open — stores, so
 *  a button elsewhere (the status bar's) can open them too. */
export const paletteOpen = writable(false);
export const shortcutsOpen = writable(false);

// Pages in the order of the sidebar, for Ctrl+1 … Ctrl+6.
export const PAGES = [
  { id: 'home', label: 'Home' },
  { id: 'devices', label: 'Devices' },
  { id: 'cloud', label: 'Cloud Backup' },
  { id: 'activity', label: 'Activity' },
  { id: 'settings', label: 'Settings' },
  { id: 'changelog', label: 'Changelog' }
];

/** For the sheet: [keys, what they do], in the order shown. */
export const SHORTCUTS = [
  ['Ctrl K', 'Jump to a game, a page or an action'],
  ['Ctrl F', 'Search the page you are on'],
  ...PAGES.map((p, i) => [`Ctrl ${i + 1}`, `Go to ${p.label}`]),
  ['Ctrl ,', 'Settings'],
  ['Shift F10', 'Actions for the focused game (or the menu key)'],
  ['?', 'This list'],
  ['Esc', 'Close a dialog or menu']
];

/** Whether keys pressed now are someone typing, which a bare key must not interrupt. */
export function isTyping(target) {
  if (!target) return false;
  const tag = target.tagName;
  return tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || !!target.isContentEditable;
}

/**
 * What a keydown means, or null. Ctrl works as Cmd on a Mac. Combinations
 * work while typing — Ctrl+K from a search box is the point of it — but a
 * bare "?" typed into a field is a question mark.
 */
export function matchShortcut(e) {
  const mod = e.ctrlKey || e.metaKey;
  if (mod && !e.altKey) {
    const k = e.key.toLowerCase();
    if (k === 'k') return 'palette';
    if (k === 'f') return 'find';
    if (k === ',') return 'go:settings';
    const n = Number(e.key);
    if (Number.isInteger(n) && n >= 1 && n <= PAGES.length && !e.shiftKey) return `go:${PAGES[n - 1].id}`;
    return null;
  }
  if (e.key === '?' && !e.ctrlKey && !e.metaKey && !e.altKey && !isTyping(e.target)) return 'help';
  return null;
}
