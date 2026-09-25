// What Home shows above the library: the latest things that happened to your
// saves, and the space the snapshots take.
//
// Read from the saved state — every snapshot and every sync is recorded there
// — and not from the log, which starts afresh each launch and is mostly
// routine.
import { snapshotKind } from './snapshots.js';

const ms = (t) => (typeof t === 'number' ? t : new Date(t).getTime());

/** Every snapshot of every game, on every branch: [{game, snap, at}]. */
function everySnapshot(games) {
  const out = [];
  for (const game of Object.values(games ?? {})) {
    for (const branch of Object.values(game.branches ?? {})) {
      for (const snap of branch.snapshots ?? []) {
        const at = ms(snap.timestamp);
        if (Number.isFinite(at)) out.push({ game, snap, at });
      }
    }
  }
  return out;
}

/**
 * The latest events, newest first: snapshots and syncs with a paired device.
 * A run of the same thing happening to the same game — a save changing every
 * few minutes while it is played — is one line with a count.
 * @returns {{kind: string, game: object, title: string, at: number, count: number}[]}
 */
export function recentEvents(games, peers = {}, { limit = 5, now = new Date() } = {}) {
  const events = everySnapshot(games).map(({ game, snap, at }) => {
    const why = snapshotKind(snap, now);
    return { kind: why.kind, game, title: why.title, at };
  });
  for (const game of Object.values(games ?? {})) {
    for (const [peerId, stamp] of Object.entries(game.lastSyncedWith ?? {})) {
      // A device since unpaired still has its time on record; it is not news.
      if (!(peerId in peers)) continue;
      const at = ms(stamp);
      if (Number.isFinite(at)) events.push({ kind: 'sync', game, title: `Synced with ${peers[peerId].name ?? 'a device'}`, at });
    }
  }
  events.sort((a, b) => b.at - a.at);

  const out = [];
  for (const e of events) {
    const last = out[out.length - 1];
    if (last && last.game.id === e.game.id && last.kind === e.kind && last.title === e.title) {
      last.count++;
      continue;
    }
    if (out.length === limit) break;
    out.push({ ...e, count: 1 });
  }
  return out;
}

/** Bytes taken by every snapshot of every game. */
export function spaceUsed(games) {
  return everySnapshot(games).reduce((sum, { snap }) => sum + (Number(snap.sizeBytes) || 0), 0);
}
