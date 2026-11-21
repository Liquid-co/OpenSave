// The Activity page's figures: how long games were played, on which device,
// in a stretch of time; how many snapshots and syncs. Plain functions of the
// activity report (GET /api/activity: items and each game's standing) so
// each can be tested on its own.
//
// Play on other devices is what they told this one of their sessions
// (p2p/playsessions.go). A device on an older build tells nothing: its time
// is "not recorded", never guessed from when its saves arrived.

export const PERIODS = ['week', 'month', 'all'];

/** When a period starts, in ms; 0 for all time. A week starts seven days
 *  back from now, a month thirty: a rolling window, so Monday morning does
 *  not read as a week with nothing in it. */
export function periodStart(period, now = Date.now()) {
  if (period === 'week') return now - 7 * 864e5;
  if (period === 'month') return now - 30 * 864e5;
  return 0;
}

// The device colours: identity, so a fixed order, never cycled, and each
// device keeps its colour whatever is filtered (validated with the dataviz
// skill's checker against the dark card and white). Past the fifth device,
// the rest share a neutral "other" rather than a made-up hue.
export const DEVICE_COLOURS = 5;

/**
 * Which colour slot each device has: this device first, then the paired
 * devices in the order they were paired, then any name only the report knows
 * (a device unpaired since). Stable: a filter does not repaint anyone.
 * @returns {Map<string, number>} device name to slot 0..4, or 5 for other
 */
export function deviceSlots(here, peers = [], extraNames = []) {
  const slots = new Map();
  const add = (name) => {
    if (name && !slots.has(name)) slots.set(name, Math.min(slots.size, DEVICE_COLOURS));
  };
  add(here);
  [...peers].sort((a, b) => String(a.pairedAt ?? '').localeCompare(String(b.pairedAt ?? '')) || String(a.name).localeCompare(String(b.name))).forEach((p) => add(p.name));
  [...extraNames].sort().forEach(add);
  return slots;
}

/** The device an item happened on: its own, or this one when it has none. */
const itemDevice = (item, here) => item.device || here;

/**
 * The tiles at the top, for a period and a device ('' for every device).
 * Played time and sessions come from the play sessions in the items; for all
 * time from each game's totals, which are exact where the items stop.
 */
export function activityStats({ items = [], games = [], here = '', period = 'month', device = '', now = Date.now(), complete = true } = {}) {
  const from = periodStart(period, now);
  const inPeriod = items.filter((i) => i.atMs >= from);
  const mine = (name) => !device || name === device;

  const byDevice = {};
  let playedMs = 0;
  let sessions = 0;
  let longest = null;
  if (period === 'all') {
    for (const g of games) {
      for (const d of g.devices ?? []) {
        if (!mine(d.device)) continue;
        byDevice[d.device] = (byDevice[d.device] ?? 0) + d.playtimeMs;
        playedMs += d.playtimeMs;
        sessions += d.sessions;
      }
    }
  }
  for (const i of inPeriod) {
    if (i.kind !== 'played' || !mine(itemDevice(i, here))) continue;
    const ms = i.durationMs ?? 0;
    if (period !== 'all') {
      const name = itemDevice(i, here);
      byDevice[name] = (byDevice[name] ?? 0) + ms;
      playedMs += ms;
      sessions += 1;
    }
    if (!longest || ms > longest.ms) longest = { ms, gameId: i.gameId, device: itemDevice(i, here) };
  }

  const snaps = inPeriod.filter((i) => i.kind === 'snapshot' && !i.device);
  const syncs = inPeriod.filter((i) => (i.kind === 'sent' || i.kind === 'received') && mine(i.device));
  return {
    playedMs,
    byDevice,
    sessions,
    longest,
    snapshots: snaps.length,
    takenByYou: snaps.filter((i) => i.auto === false).length,
    syncs: syncs.length,
    syncDevices: new Set(syncs.map((i) => i.device).filter(Boolean)).size,
    // The items reach back to the start of the period, or the counts are
    // "at least" (the page asks for a bounded number).
    complete: complete || (items.length > 0 && items[items.length - 1].atMs < from)
  };
}

/**
 * One row per game for the table: where and when it was last played, time
 * played in the period on each device, sessions, and the last snapshot and
 * sync. `report.games` is the daemon's standing for each game.
 */
export function gameRows({ items = [], games = [], here = '', period = 'month', device = '', now = Date.now() } = {}) {
  const from = periodStart(period, now);
  const mine = (name) => !device || name === device;
  const played = new Map(); // gameId -> {byDevice, sessions}
  if (period !== 'all') {
    for (const i of items) {
      if (i.kind !== 'played' || i.atMs < from) continue;
      const name = itemDevice(i, here);
      if (!mine(name)) continue;
      const p = played.get(i.gameId) ?? { byDevice: {}, sessions: 0 };
      p.byDevice[name] = (p.byDevice[name] ?? 0) + (i.durationMs ?? 0);
      p.sessions += 1;
      played.set(i.gameId, p);
    }
  }
  return games.map((g) => {
    let byDevice = {};
    let sessions = 0;
    if (period === 'all') {
      for (const d of g.devices ?? []) {
        if (!mine(d.device)) continue;
        byDevice[d.device] = d.playtimeMs;
        sessions += d.sessions;
      }
    } else {
      const p = played.get(g.gameId);
      if (p) ({ byDevice, sessions } = p);
    }
    const playedMs = Object.values(byDevice).reduce((a, b) => a + b, 0);
    // Played somewhere that has not told its sessions: the last-played
    // device is known (a save arrived from it) but its time is not.
    const lastOn = g.lastPlayedAt ? g.lastPlayedOn || here : '';
    const toldBy = new Set((g.devices ?? []).map((d) => d.device));
    const unrecorded = !!lastOn && !toldBy.has(lastOn) && mine(lastOn);
    return {
      gameId: g.gameId,
      lastPlayedAt: g.lastPlayedAt ?? 0,
      lastPlayedOn: lastOn,
      playedMs,
      byDevice,
      sessions,
      unrecorded,
      lastSnapshotAt: g.lastSnapshotAt ?? 0,
      lastSyncedAt: g.lastSyncedAt ?? 0,
      lastSyncedWith: g.lastSyncedWith ?? ''
    };
  });
}

/** The columns the table sorts by, and each one's value. */
export const SORT_COLUMNS = ['played', 'time', 'sessions'];
const SORT_VALUE = {
  played: (r) => r.lastPlayedAt || 0,
  time: (r) => r.playedMs || 0,
  sessions: (r) => r.sessions || 0
};

/** No column chosen: the table's normal order, by name. */
export const NORMAL_SORT = Object.freeze({ by: '', dir: '' });

/**
 * A column header pressed: normal, then descending, then ascending, then
 * normal again. Pressing another column starts that one at descending, so
 * only one column sorts at a time.
 */
export function nextSort(state, column) {
  if (state?.by !== column) return { by: column, dir: 'desc' };
  if (state.dir === 'desc') return { by: column, dir: 'asc' };
  return NORMAL_SORT;
}

/**
 * Rows in the order chosen: by a column either way, or normally by name.
 * Rows with nothing in that column (never played, no sessions) stay at the
 * bottom both ways; ties go by name.
 */
export function sortRows(rows, sort = NORMAL_SORT, names = {}) {
  const name = (r) => String(names[r.gameId] ?? r.gameId);
  const byName = (a, b) => name(a).localeCompare(name(b));
  const value = SORT_VALUE[sort?.by];
  if (!value) return [...rows].sort(byName);
  const sign = sort.dir === 'asc' ? 1 : -1;
  return [...rows].sort((a, b) => {
    const va = value(a);
    const vb = value(b);
    if (!va !== !vb) return va ? -1 : 1;
    return sign * (va - vb) || byName(a, b);
  });
}

/** The parts of a split bar: each device's share, in slot order, the ones
 *  with no time left out. */
export function barParts(byDevice, slots) {
  const total = Object.values(byDevice).reduce((a, b) => a + b, 0);
  if (!total) return [];
  return Object.entries(byDevice)
    .filter(([, ms]) => ms > 0)
    .sort(([a], [b]) => (slots.get(a) ?? 99) - (slots.get(b) ?? 99) || a.localeCompare(b))
    .map(([device, ms]) => ({ device, ms, share: ms / total, slot: slots.get(device) ?? DEVICE_COLOURS }));
}
