// One line per game saying where its save stands, for the library and the
// summary above it. The order is the order a person needs to hear things in:
// something waiting on them first, then something happening now, then how
// recently the save was last made safe.
//
// "Synced" means synced with a paired device; with nothing paired, the only
// safety net is a snapshot, so that is what the line reports instead. Never
// "up to date": this device cannot know what another device has done since.
import { timeAgo, latestOf } from './timeago.js';
import { playLength } from './format.js';

/**
 * @param {object} game A game from the games payload.
 * @param {object} ctx
 * @param {object} ctx.peers Paired devices, keyed by id.
 * @param {object} [ctx.activity] This game's syncActivity entry.
 * @param {boolean} [ctx.conflicted] A conflict for this game is waiting.
 * @param {number} [ctx.now]
 * @returns {{state: string, label: string, tone: 'warn'|'busy'|'playing'|'ok'|'muted'}}
 */
export function gameStatus(game, { peers = {}, activity, conflicted = false, now = Date.now() } = {}) {
  // Before anything else: with its folder gone nothing else about it can
  // move — no snapshot, no sync, no decision taken.
  if (game.savePathMissing) return { state: 'missing', label: 'Save folder missing', tone: 'warn' };
  // Every save file deleted here at once, held back from the other devices
  // until someone says whether that was meant (lib/emptied.js).
  if (game.emptied?.state === 'held') return { state: 'emptied', label: 'Files deleted — needs a decision', tone: 'warn' };
  if (game.emptied?.state === 'fetching') return { state: 'restoring', label: 'Putting files back', tone: 'busy' };
  if (conflicted) return { state: 'conflict', label: 'Needs a decision', tone: 'warn' };
  // Being played here now (see daemon/sessions.go): what matters most about
  // it while it lasts; its save is kept as the session leaves it. Its own
  // tone, green, and never the spinning "busy" one — nothing is syncing.
  if (game.playingSince) return { state: 'playing', label: sessionLabel(game.playingSince, now), tone: 'playing' };
  if (activity?.state === 'running') {
    return { state: 'syncing', label: `Syncing ${activity.percentage ?? 0}%`, tone: 'busy' };
  }
  if (activity?.state === 'error') return { state: 'error', label: 'Last sync failed', tone: 'warn' };
  if (game.autoSync === false) return { state: 'paused', label: 'Auto-sync off', tone: 'muted' };

  if (Object.keys(peers).length > 0) {
    const paired = Object.fromEntries(Object.entries(game.lastSyncedWith ?? {}).filter(([id]) => id in peers));
    const at = latestOf(paired);
    return at
      ? { state: 'synced', label: `Synced ${timeAgo(at, now)}`, tone: 'ok' }
      : { state: 'unsynced', label: 'Not synced yet', tone: 'muted' };
  }

  const snap = latestSnapshotAt(game);
  return snap
    ? { state: 'local', label: `Snapshot ${timeAgo(snap, now)}`, tone: 'ok' }
    : { state: 'empty', label: 'No snapshots yet', tone: 'muted' };
}

const n = (count, one, many) => `${count} ${count === 1 ? one : many}`;

/**
 * The one sentence above the library: the most pressing thing about all of
 * it. A decision waiting beats a sync running, which beats a failure that
 * retries on its own, which beats the quiet cases.
 * @param {{game: object, status: {state: string}}[]} rows
 */
export function librarySummary(rows) {
  const by = (state) => rows.filter((r) => r.status.state === state);
  const missing = by('missing');
  const emptied = by('emptied');
  const conflict = by('conflict');
  const syncing = by('syncing');
  const failed = by('error');
  // From the snapshots, not the state: a game with devices paired reports
  // its sync, and may have no snapshot at all behind it. "Backed up" below
  // has to be true of every game it counts.
  const empty = rows.filter((r) => latestSnapshotAt(r.game) === null);
  // Snapshots found damaged when read back (daemon/verify.go): a backup that
  // cannot be restored is worth knowing about before it is needed.
  const damaged = rows.flatMap((r) =>
    Object.values(r.game.branches ?? {}).flatMap((b) => (b.snapshots ?? []).filter((s) => s.problem).map(() => r.game))
  );
  const paused = by('paused');

  if (missing.length > 0) {
    return {
      tone: 'warn',
      headline:
        missing.length === 1
          ? `${missing[0].game.name}'s save folder is missing`
          : `${missing.length} games' save folders are missing`
    };
  }
  if (emptied.length > 0) {
    return {
      tone: 'warn',
      headline:
        emptied.length === 1
          ? `${emptied[0].game.name}'s save files were all deleted here`
          : `${emptied.length} games' save files were all deleted here`
    };
  }
  if (conflict.length > 0) {
    return {
      tone: 'warn',
      headline:
        conflict.length === 1
          ? `${conflict[0].game.name} needs a decision`
          : `${n(conflict.length, 'game needs', 'games need')} a decision`
    };
  }
  if (syncing.length > 0) {
    return {
      tone: 'busy',
      headline: syncing.length === 1 ? `Syncing ${syncing[0].game.name}…` : `Syncing ${syncing.length} games…`
    };
  }
  if (failed.length > 0) {
    return {
      tone: 'warn',
      headline:
        failed.length === 1
          ? `The last sync of ${failed[0].game.name} failed — it will try again`
          : `${failed.length} syncs failed — they will try again`
    };
  }
  if (damaged.length > 0) {
    return {
      tone: 'warn',
      headline:
        damaged.length === 1
          ? `A snapshot of ${damaged[0].name} can't be restored`
          : `${damaged.length} snapshots can't be restored`
    };
  }
  if (empty.length > 0) {
    return {
      tone: 'muted',
      headline: `${n(empty.length, 'game has', 'games have')} no snapshot yet`
    };
  }
  if (paused.length > 0) {
    return {
      tone: 'muted',
      headline: `Auto-sync is off for ${n(paused.length, 'game', 'games')}`
    };
  }
  return {
    tone: 'ok',
    headline: rows.length === 1 ? 'Your game is backed up' : `All ${rows.length} games are backed up`
  };
}

/** The newest snapshot's time on any branch, or null. */
export function latestSnapshotAt(game) {
  let best = null;
  for (const b of Object.values(game?.branches ?? {})) {
    for (const s of b.snapshots ?? []) {
      if (s.timestamp && (best === null || s.timestamp > best)) best = s.timestamp;
    }
  }
  return best;
}

/** Game ids with a conflict waiting, whole-game or in one of its folders. */
export function conflictedIds(conflicts, locationConflicts) {
  const ids = new Set(Object.keys(conflicts ?? {}));
  for (const c of locationConflicts ?? []) if (c?.gameId) ids.add(c.gameId);
  return ids;
}

const byName = (a, b) => a.game.name.localeCompare(b.game.name);

// Larger first, ties and the missing by name.
const descending = (value) => (a, b) => {
  const x = value(a) ?? -Infinity;
  const y = value(b) ?? -Infinity;
  return x === y ? byName(a, b) : x < y ? 1 : -1;
};

const stamp = (s) => (s ? Date.parse(s) : null);
const snapshotsOf = (game) => Object.values(game.branches ?? {}).flatMap((b) => b.snapshots ?? []);

// What needs looking at, in the order the summary above the library reads it.
const URGENCY = { missing: 0, emptied: 1, conflict: 2, error: 3, syncing: 4, restoring: 4, unsynced: 5, empty: 6, paused: 7 };

/**
 * The orders the library can be put in. Each compares two rows, {game,
 * status}, and puts first what that order is about; the View menu can turn
 * any of them around.
 */
export const SORTS = {
  name: { label: 'Name', compare: byName },
  // The newest snapshot is the last time OpenSave saw the save change.
  recent: { label: 'Recently changed', compare: descending((r) => stamp(latestSnapshotAt(r.game))) },
  synced: {
    label: 'Recently synced',
    compare: descending((r) => {
      const times = Object.values(r.game.lastSyncedWith ?? {}).map(stamp).filter((t) => t !== null);
      return times.length ? Math.max(...times) : null;
    })
  },
  attention: {
    label: 'Needs attention first',
    compare: (a, b) => {
      const x = URGENCY[a.status?.state] ?? 9;
      const y = URGENCY[b.status?.state] ?? 9;
      return x === y ? byName(a, b) : x - y;
    }
  },
  snapshots: { label: 'Most snapshots', compare: descending((r) => snapshotsOf(r.game).length) },
  size: {
    label: 'Most space',
    compare: descending((r) => snapshotsOf(r.game).reduce((n, s) => n + (Number(s.sizeBytes) || 0), 0))
  },
  added: { label: 'Recently added', compare: descending((r) => stamp(r.game.createdAt)) },
  // A game being played now first, then by when it was last played.
  played: {
    label: 'Recently played',
    compare: descending((r) => (r.game.playingSince ? Infinity : stamp(r.game.lastPlayedAt || null)))
  }
};

/** Rows in a view's order. */
export function sortRows(rows, sort, reverse = false) {
  const compare = (SORTS[sort] ?? SORTS.name).compare;
  const out = [...rows].sort(compare);
  return reverse ? out.reverse() : out;
}

/** "In session", and for how long once it is a minute or more. */
export function sessionLabel(since, now = Date.now()) {
  const ms = now - Date.parse(since);
  return Number.isFinite(ms) && ms >= 60_000 ? `In session for ${playLength(ms)}` : 'In session';
}
