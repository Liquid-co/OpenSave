// How the library looks, and which of it is shown.
//
// The look — cover style, tiles per row, tile size, order — is a preference
// of this device's screen, kept on this device. It is changed from Settings
// and from the library's own View menu, and both take effect at once. It is
// deliberately not part of the settings the Settings page saves as a form:
// that form is a copy taken when the page opens, and saving it would put back
// whatever was chosen before, from the other place, since.
//
// Filters are not kept at all: they are for finding something now.
import { writable } from 'svelte/store';

export const COVER_STYLES = {
  // Steam's header image, 460×215: the title is legible in the art.
  wide: { label: 'Wide banners', aspect: '460 / 215' },
  // Steam's library capsule, 600×900: about twice as many games on screen.
  tall: { label: 'Tall box art', aspect: '600 / 900' }
};

export const TILE_SIZES = { small: 'Small', medium: 'Medium', large: 'Large' };

// Narrowest a tile may get when the number per row is left to the window.
const MIN_WIDTH = {
  wide: { small: 200, medium: 260, large: 340 },
  tall: { small: 120, medium: 155, large: 200 }
};

export const COLUMN_CHOICES = ['auto', 2, 3, 4, 5, 6, 7, 8];

export const SORT_IDS = ['name', 'recent', 'synced', 'attention', 'snapshots', 'size', 'added'];

export const DEFAULT_VIEW = Object.freeze({ cover: 'wide', columns: 'auto', size: 'medium', sort: 'name', reverse: false, overview: true });

/** A view with every field valid, whatever was stored. */
export function sanitizeView(raw) {
  const v = raw && typeof raw === 'object' ? raw : {};
  const columns = v.columns === 'auto' ? 'auto' : Number(v.columns);
  return {
    cover: v.cover in COVER_STYLES ? v.cover : DEFAULT_VIEW.cover,
    columns: COLUMN_CHOICES.includes(columns) ? columns : DEFAULT_VIEW.columns,
    size: v.size in TILE_SIZES ? v.size : DEFAULT_VIEW.size,
    sort: SORT_IDS.includes(v.sort) ? v.sort : DEFAULT_VIEW.sort,
    reverse: typeof v.reverse === 'boolean' ? v.reverse : DEFAULT_VIEW.reverse,
    // Recent activity, along the foot of the summary card on Home.
    overview: typeof v.overview === 'boolean' ? v.overview : DEFAULT_VIEW.overview
  };
}

/** The grid's column template for a view. */
export function gridColumns(view) {
  const v = sanitizeView(view);
  if (v.columns !== 'auto') return `repeat(${v.columns}, minmax(0, 1fr))`;
  return `repeat(auto-fill, minmax(${MIN_WIDTH[v.cover][v.size]}px, 1fr))`;
}

const VIEW_KEY = 'opensave.libraryView';
// The order was stored on its own before the rest of the view existed.
const OLD_SORT_KEY = 'opensave.librarySort';

export function loadView(storage = globalThis.localStorage) {
  try {
    const saved = storage?.getItem(VIEW_KEY);
    if (saved) return sanitizeView(JSON.parse(saved));
    const oldSort = storage?.getItem(OLD_SORT_KEY);
    return sanitizeView({ ...DEFAULT_VIEW, sort: oldSort });
  } catch {
    return { ...DEFAULT_VIEW };
  }
}

export function saveView(view, storage = globalThis.localStorage) {
  try {
    storage?.setItem(VIEW_KEY, JSON.stringify(sanitizeView(view)));
  } catch {}
}

/** The library's look, as a store both Settings and the library write to. */
export const libraryView = writable(loadView());
libraryView.subscribe((v) => saveView(v));

// ── Filters ──────────────────────────────────────────────────────────

// What a game's status line says, gathered into what someone would look for.
export const STATUS_FILTERS = [
  { id: 'all', label: 'All', states: null },
  { id: 'attention', label: 'Needs attention', states: ['conflict', 'error'] },
  { id: 'syncing', label: 'Syncing', states: ['syncing'] },
  { id: 'synced', label: 'Synced', states: ['synced'] },
  { id: 'unsynced', label: 'Not synced yet', states: ['unsynced'] },
  { id: 'paused', label: 'Auto-sync off', states: ['paused'] },
  { id: 'empty', label: 'No snapshot yet', states: ['empty'] }
];

const byId = Object.fromEntries(STATUS_FILTERS.map((f) => [f.id, f]));

/** Rows ({game, status}) matching a name search and a status filter. */
export function filterRows(rows, { query = '', status = 'all' } = {}) {
  const q = query.trim().toLowerCase();
  const states = byId[status]?.states ?? null;
  return (rows ?? []).filter(
    (r) => (!q || (r.game.name ?? '').toLowerCase().includes(q)) && (!states || states.includes(r.status.state))
  );
}

/**
 * The status filters worth offering, with how many games each would show.
 * One that would show nothing is left out, and so is one that would show
 * everything, since All already does.
 */
export function offeredFilters(rows) {
  const total = (rows ?? []).length;
  return STATUS_FILTERS.map((f) => ({
    ...f,
    count: f.states ? rows.filter((r) => f.states.includes(r.status.state)).length : total
  })).filter((f) => f.id === 'all' || (f.count > 0 && f.count < total));
}
