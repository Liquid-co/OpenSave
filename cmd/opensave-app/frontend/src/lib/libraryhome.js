// What the Library page and the frame around it say about the library as a
// whole: the games played lately, whose cover the page is lit by, where
// syncing stands (the button at the top right), and what is waiting on you.
// Plain functions of the stores' values, so each can be tested on its own.

const ms = (v) => {
  if (v == null || v === '') return 0;
  if (typeof v === 'number') return Number.isFinite(v) ? v : 0;
  const t = Date.parse(v);
  return Number.isFinite(t) ? t : 0;
};

/**
 * The games played most lately, newest first, those being played now ahead
 * of the rest. A game's last play is the later of this device's own record
 * (game.lastPlayedAt) and what the activity report knows (`played`: gameId
 * to {at, on}, which includes play inferred from another device's save
 * arriving). Games never played are left out, and so are those removed
 * from the row (`hidden`: gameId to the play, in ms, that was removed) —
 * until they are played again, when the newer play brings them back.
 * @returns {{game, playing: boolean, at: number, on: string}[]}
 */
export function recentlyPlayed(games, played = {}, limit = 4, hidden = {}) {
  const out = [];
  const keep = (item) => {
    if (!(item.at > (hidden?.[item.game.id] ?? 0))) return;
    out.push(item);
  };
  for (const game of games ?? []) {
    const since = ms(game.playingSince);
    if (since) {
      keep({ game, playing: true, at: since, on: '' });
      continue;
    }
    const here = ms(game.lastPlayedAt);
    const report = played?.[game.id];
    const there = ms(report?.at);
    if (!here && !there) continue;
    // The report says where only when it was elsewhere; "" is this device.
    if (there > here) keep({ game, playing: false, at: there, on: report?.on ?? '' });
    else keep({ game, playing: false, at: here, on: '' });
  }
  out.sort((a, b) => Number(b.playing) - Number(a.playing) || b.at - a.at);
  return out.slice(0, limit);
}

/** The game whose cover lights the top of the Library: the one being played,
 *  or else the one played last. Null when nothing has been played. */
export function backdropGame(games, played = {}, hidden = {}) {
  return recentlyPlayed(games, played, 1, hidden)[0]?.game ?? null;
}

/** The games the Library's cover moves through, in turn: the one it starts
 *  on (backdropGame), then the rest of the library in its order, coming
 *  round again. Art marked explicit is never shown, so those are left out. */
export function backdropCycle(games, start = null) {
  const list = (games ?? []).filter((game) => game && !game.coverExplicit);
  const i = start ? list.findIndex((game) => game.id === start.id) : -1;
  return i > 0 ? [...list.slice(i), ...list.slice(0, i)] : list;
}

/** gameId -> {at, on} from /api/activity's games, for recentlyPlayed. */
export function playedFromReport(report) {
  const out = {};
  for (const g of report?.games ?? []) {
    if (g?.gameId && g.lastPlayedAt) out[g.gameId] = { at: g.lastPlayedAt, on: g.lastPlayedOn ?? '' };
  }
  return out;
}

/**
 * Where syncing stands, for the button at the top right and the panel it
 * opens. In order: paused, syncing now, no other device paired, none of them
 * online, and synced.
 * @returns {{kind: 'paused'|'syncing'|'alone'|'offline'|'synced', running: number, paired: number, online: number, lastAt: number}}
 */
export function syncState({ peers = {}, activity = {}, paused = false } = {}) {
  const list = Object.values(peers ?? {});
  const online = list.filter((p) => p.status === 'online').length;
  const running = Object.values(activity ?? {}).filter((a) => a?.state === 'running').length;
  const lastAt = list.reduce((best, p) => Math.max(best, ms(p.lastSynced)), 0);
  const base = { running, paired: list.length, online, lastAt };
  if (paused) return { kind: 'paused', ...base };
  if (running > 0) return { kind: 'syncing', ...base };
  if (list.length === 0) return { kind: 'alone', ...base };
  if (online === 0) return { kind: 'offline', ...base };
  return { kind: 'synced', ...base };
}

/**
 * What is waiting on you, by kind, for the strip at the top of the Library:
 * the bell's own list (notifications.js waitingOnYou) counted up, so the two
 * never disagree. An update on offer is not counted: it waits on nobody.
 * Kinds come from each item's id ("conflict:…", "missing:…").
 * @returns {{kind: string, n: number}[]} in the order the bell lists them
 */
export function waitingSummary(items) {
  const counts = new Map();
  for (const item of items ?? []) {
    const kind = String(item?.id ?? '').split(':')[0];
    if (!kind || kind === 'update') continue;
    // "new:A,B,C" is one entry for several games.
    const n = kind === 'new' ? String(item.id).slice(4).split(',').filter(Boolean).length || 1 : 1;
    counts.set(kind, (counts.get(kind) ?? 0) + n);
  }
  return [...counts].map(([kind, n]) => ({ kind, n }));
}
