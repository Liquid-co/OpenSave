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
import { gameMenuItems, runGameAction } from './gameactions.js';

/** {x, y, items} while open; items are {label, icon, run, danger, disabled, hint} or null for a divider. */
export const contextMenu = writable(null);

/** Opens the menu where the event happened — or, from the keyboard's menu
 *  key, which reports no pointer position, at the element itself. */
export function openMenu(event, items) {
  event.preventDefault();
  event.stopPropagation();
  let { clientX: x, clientY: y } = event;
  if (!x && !y && event.currentTarget?.getBoundingClientRect) {
    const r = event.currentTarget.getBoundingClientRect();
    x = r.left + 12;
    y = r.top + 12;
  }
  contextMenu.set({ x, y, items });
}

export const closeMenu = () => contextMenu.set(null);

const gameIcons = {
  open: ArrowUpRight,
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
    gameMenuItems(game).map((item) => item && { ...item, icon: gameIcons[item.id], run: () => runGameAction(item.id, game) })
  );
}
