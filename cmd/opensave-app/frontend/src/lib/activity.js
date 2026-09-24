// The Activity page's reading of the daemon's log.
//
// The log is written for whoever debugs a sync, one line per event, and the
// page used to show it exactly that way: monospace, oldest first, levels as
// words. It is kept — behind "Technical log" — because a bug report wants it
// verbatim. The default is the same entries read like a feed: newest first,
// by day, the routine left out until asked for, and the games in them one
// click away.
import { dayLabel } from './snapshots.js';

// What each filter shows. "Highlights" leaves out info, which is most of the
// log and mostly routine: every snapshot writes four lines, three of them
// about uploading it and finding no one online to sync with.
export const SHOW = {
  highlights: new Set(['success', 'warn', 'error']),
  all: null,
  problems: new Set(['warn', 'error'])
};

/** Entries matching the filters, in the order given. */
export function filterEntries(entries, { show = 'all', query = '' } = {}) {
  const levels = SHOW[show] ?? null;
  const q = query.trim().toLowerCase();
  return (entries ?? []).filter(
    (e) => (!levels || levels.has(e.level)) && (!q || (e.message ?? '').toLowerCase().includes(q))
  );
}

/** Newest first, in runs by day: [{day, entries}]. */
export function feedByDay(entries, now = new Date()) {
  const groups = [];
  for (let i = (entries ?? []).length - 1; i >= 0; i--) {
    const e = entries[i];
    const day = dayLabel(new Date(e.timestamp), now);
    const last = groups[groups.length - 1];
    if (last && last.day === day) last.entries.push(e);
    else groups.push({ day, entries: [e] });
  }
  return groups;
}

const isWordChar = (c) => !!c && /[\p{L}\p{N}_]/u.test(c);
// A folder in a path is often named after the game, and is not a mention of
// it: "D:\Games\Hades\save.dat" should stay a path.
const isPathChar = (c) => c === '\\' || c === '/';

/**
 * A message split so the games named in it can be links:
 * [{text}, {text, gameId}, …]. The daemon names a game by its name in some
 * messages and by its ID in others ("auto-snapshot created for balatro"); an
 * ID is shown as the game's name. A match counts only as a whole word outside
 * a path — "Hades" is not found in "Hades II" or in "D:\Games\Hades\" — and
 * the longest wins where two overlap.
 */
export function linkGames(message, games) {
  const text = message ?? '';
  const needles = [];
  for (const g of games ?? []) {
    if (g?.name && g.name.length >= 2) needles.push({ find: g.name, id: g.id, show: g.name });
    if (g?.id && g.id.length >= 2 && g.id !== g.name) needles.push({ find: g.id, id: g.id, show: g.name ?? g.id });
  }
  needles.sort((a, b) => b.find.length - a.find.length);

  const taken = []; // [start, end, gameId, show]
  for (const n of needles) {
    let from = 0;
    for (;;) {
      const at = text.indexOf(n.find, from);
      if (at < 0) break;
      const end = at + n.find.length;
      from = end;
      const before = text[at - 1];
      const after = text[end];
      if (isWordChar(before) || isWordChar(after) || isPathChar(before) || isPathChar(after)) continue;
      if (taken.some(([s, e]) => at < e && end > s)) continue;
      taken.push([at, end, n.id, n.show]);
    }
  }
  taken.sort((a, b) => a[0] - b[0]);
  const out = [];
  let pos = 0;
  for (const [s, e, id, show] of taken) {
    if (s > pos) out.push({ text: text.slice(pos, s) });
    out.push({ text: show, gameId: id });
    pos = e;
  }
  if (pos < text.length || out.length === 0) out.push({ text: text.slice(pos) });
  return out;
}

/** The entries as plain text, one per line, for pasting into a report. */
export function asText(entries) {
  return (entries ?? [])
    .map((e) => `${e.timestamp} ${String(e.level ?? '').toUpperCase().padEnd(7)} ${e.message}`)
    .join('\n');
}
