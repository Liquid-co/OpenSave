// The right-click menu: one, drawn by ContextMenu.svelte in App, opened from
// anywhere with the items for what was clicked.
import { writable } from 'svelte/store';
import FolderOpen from 'lucide-svelte/icons/folder-open';
import RefreshCw from 'lucide-svelte/icons/refresh-cw';
import Camera from 'lucide-svelte/icons/camera';
import Play from 'lucide-svelte/icons/play';
import ArrowUpRight from 'lucide-svelte/icons/arrow-up-right';
import RotateCcw from 'lucide-svelte/icons/rotate-ccw';
import Trash2 from 'lucide-svelte/icons/trash-2';
import Star from 'lucide-svelte/icons/star';
import Tags from 'lucide-svelte/icons/tags';
import { get } from 'svelte/store';
import { gameMenuItems, runGameAction } from './gameactions.js';
import { collections, isFavourite } from './collections.js';

/** {x, y, items, anchor} while open; items are {label, icon, run, danger,
 *  disabled, hint} or null for a divider; anchor is the element it was opened on. */
export const contextMenu = writable(null);

/** Opens the menu where the event happened — or, from the keyboard's menu
 *  key, which reports no pointer position, at the element itself. */
export function openMenu(event, items) {
  event.preventDefault();
  event.stopPropagation();
  const anchor = event.currentTarget?.getBoundingClientRect ? event.currentTarget : null;
  let { clientX: x, clientY: y } = event;
  if (!x && !y && anchor) {
    const r = anchor.getBoundingClientRect();
    x = r.left + 12;
    y = r.top + 12;
  }
  contextMenu.set({ x, y, items, anchor });
}

// The element a menu belongs to is marked data-menu-open while the menu is
// up, so it stays highlighted and it is plain which game the menu is for —
// the pointer has usually moved off it by then. Any element opening a menu
// gets this; each styles the mark its own way.
//
// Closed with the keyboard or a controller, focus goes back to that element:
// the menu had it, and would otherwise take it away with it, leaving nothing
// in focus and the next key or button press with nowhere to start from.
let marked = null;
contextMenu.subscribe((menu) => {
  const next = menu?.anchor ?? null;
  if (marked && marked !== next) {
    if (marked.dataset) delete marked.dataset.menuOpen;
    if (!menu) giveFocusBack(marked);
  }
  if (next?.dataset) next.dataset.menuOpen = '';
  marked = next;
});

function giveFocusBack(el) {
  const active = globalThis.document?.activeElement;
  if (!el.isConnected || !el.focus) return;
  if (!active || active === globalThis.document.body || active.closest?.('.menu')) el.focus({ preventScroll: true });
}

export const closeMenu = () => contextMenu.set(null);

const gameIcons = {
  open: ArrowUpRight,
  favourite: Star,
  collections: Tags,
  sync: RefreshCw,
  snapshot: Camera,
  launch: Play,
  folder: FolderOpen,
  'restore-latest': RotateCcw,
  untrack: Trash2
};

export function openGameMenu(event, game) {
  openMenu(
    event,
    gameMenuItems(game, { favourite: isFavourite(get(collections), game.id) }).map((item) => item && { ...item, icon: gameIcons[item.id], run: () => runGameAction(item.id, game) })
  );
}
