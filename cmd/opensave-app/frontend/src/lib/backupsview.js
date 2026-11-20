// The Backups page's facts, from GET /api/cloud/overview: each game's state,
// the headline, the shelf's order and filters, and where each copy sits on
// the timeline.
//
// Only what the app knows. A cloud copy carries no record of why it was
// taken; this PC's own record says, for the copies it holds too. A copy it
// does not hold is 'unknown', never given a kind it may not have.
import { snapshotKind } from './snapshots.js';

/** A copy's kind, in the snapshot rows' colours (views/game/SnapshotsTab). */
export function copyKind(copy) {
  if (!copy?.known) return 'unknown';
  if (copy.safety) return 'safety';
  const k = snapshotKind({ comment: copy.comment ?? '', isSystemAuto: !!copy.isSystemAuto }).kind;
  if (k === 'manual' || k === 'session' || k === 'safety') return k;
  return 'auto';
}

/** The kinds in the order the key lists them. */
export const COPY_KINDS = ['session', 'manual', 'safety', 'auto', 'unknown'];

/** When a copy's snapshot was taken, else when it reached the cloud. */
export function copyTime(copy) {
  if (copy?.takenMs > 0) return copy.takenMs;
  const t = Date.parse(copy?.createdTime ?? '');
  return Number.isFinite(t) ? t : 0;
}

/**
 * Where a game stands:
 * - 'safe': the save this PC is at is in the cloud;
 * - 'behind': it has copies, but not of the save it is at now;
 * - 'waiting': snapshots here, none up yet;
 * - 'empty': no snapshots anywhere yet;
 * - 'cloudOnly': in the cloud, not tracked on this PC.
 */
export function gameState(g) {
  if (!g.tracked) return 'cloudOnly';
  if (!g.copies?.length) return g.localSnapshots > 0 ? 'waiting' : 'empty';
  if (g.current && !g.currentUploaded) return 'behind';
  return 'safe';
}

/** Needs an upload: something here the cloud does not have. */
export const needsUpload = (g) => ['waiting', 'behind'].includes(gameState(g));

/** The newest copy of a game, or null. */
export function newestCopy(g) {
  let best = null;
  for (const c of g.copies ?? []) if (!best || copyTime(c) > copyTime(best)) best = c;
  return best;
}

/** When a copy reached the cloud, else when its snapshot was taken. */
export function uploadTime(copy) {
  const t = Date.parse(copy?.createdTime ?? '');
  return Number.isFinite(t) ? t : copyTime(copy);
}

/**
 * The headline: of the games tracked here, how many have their current save
 * up, which are waiting, and when anything last went up. That is the upload,
 * not the snapshot: an old snapshot sent today was backed up today.
 */
export function backupSummary(games = []) {
  const tracked = games.filter((g) => g.tracked);
  const safe = tracked.filter((g) => gameState(g) === 'safe').length;
  const waiting = tracked.filter(needsUpload);
  let lastMs = 0;
  for (const g of games) for (const c of g.copies ?? []) lastMs = Math.max(lastMs, uploadTime(c));
  return { tracked: tracked.length, safe, waiting: waiting.map((g) => g.gameName), lastMs };
}

/** The shelf's filters and what each holds. */
export const SHELF_FILTERS = ['all', 'safe', 'notYet'];
export function inFilter(g, filter) {
  if (filter === 'safe') return gameState(g) === 'safe';
  if (filter === 'notYet') return gameState(g) !== 'safe';
  return true;
}

/** Games needing an upload first, then the newest copy first, then by name. */
export function shelfOrder(games = []) {
  const rank = (g) => (needsUpload(g) ? 0 : 1);
  return [...games].sort(
    (a, b) =>
      rank(a) - rank(b) ||
      copyTime(newestCopy(b) ?? {}) - copyTime(newestCopy(a) ?? {}) ||
      a.gameName.localeCompare(b.gameName)
  );
}

/** How many edges the stack behind a cover shows: more copies, more edges. */
export const stackEdges = (copies) => (copies >= 5 ? 2 : copies >= 2 ? 1 : 0);

const DAY = 24 * 60 * 60 * 1000;

/**
 * The timeline's window: the last `days` days, today the last. Days start at
 * local midnight, so a copy lands on the day it was taken where you are.
 */
export function timelineWindow(now = Date.now(), days = 14) {
  const midnight = new Date(now);
  midnight.setHours(0, 0, 0, 0);
  const start = new Date(midnight);
  start.setDate(start.getDate() - (days - 1));
  const end = new Date(midnight);
  end.setDate(end.getDate() + 1);
  const dayStarts = [];
  for (let i = 0; i < days; i++) {
    const d = new Date(start);
    d.setDate(d.getDate() + i);
    dayStarts.push(d.getTime());
  }
  return { start: start.getTime(), end: end.getTime(), now, dayStarts };
}

/** Where a moment falls in the window, 0 to 1. */
export const placeIn = (win, ms) => (ms - win.start) / (win.end - win.start);

/**
 * A game's dots: its copies inside the window, oldest first, each with its
 * place, its kind, and whether it is the game's newest copy.
 */
export function laneDots(g, win) {
  const newest = newestCopy(g);
  return (g.copies ?? [])
    .map((c) => ({ copy: c, ms: copyTime(c) }))
    .filter(({ ms }) => ms >= win.start && ms < win.end)
    .sort((a, b) => a.ms - b.ms)
    .map(({ copy, ms }) => ({ copy, ms, at: placeIn(win, ms), kind: copyKind(copy), latest: copy === newest }));
}

/** The kinds present on the timeline, for its key. */
export function kindsShown(games, win) {
  const seen = new Set();
  for (const g of games) for (const d of laneDots(g, win)) seen.add(d.kind);
  return COPY_KINDS.filter((k) => seen.has(k));
}

export { DAY };
