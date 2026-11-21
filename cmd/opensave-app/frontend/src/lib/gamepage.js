// The game page's facts: the timeline of where the save has been, where each
// branch came from and who is on it, and the line under the game's name.
//
// Only what the app knows. Play sessions are what each device recorded
// (Activity's own report); a branch's origin is read from its first snapshot,
// which says how it began; a paired device's branch is the one it answered
// the last sync with, and is never said to be more current than that.
import { MIRROR_PREFIX } from './snapshots.js';

const DAY = 24 * 60 * 60 * 1000;

/**
 * The timeline's window: the last `days` days, today the last, starting at
 * local midnight.
 */
export function timelineWindow(now = Date.now(), days = 14) {
  const end = new Date(now);
  end.setHours(24, 0, 0, 0);
  const start = new Date(end);
  start.setDate(start.getDate() - days);
  const dayStarts = [];
  for (let i = 0; i < days; i++) {
    const d = new Date(start);
    d.setDate(d.getDate() + i);
    dayStarts.push(d.getTime());
  }
  return { start: start.getTime(), end: end.getTime(), now, dayStarts };
}

/** Where a moment falls in the window, 0 to 1. */
export const placeIn = (win, ms) => Math.min(1, Math.max(0, (ms - win.start) / (win.end - win.start)));

/**
 * One lane per device the game was played on in the window, this device
 * first, each with its sessions as spans (`at` and `width`, 0 to 1). A
 * session's `atMs` is when it ended; it began `durationMs` earlier.
 */
export function playLanes(items = [], gameId, here, win) {
  const lanes = new Map([[here, []]]);
  for (const i of items) {
    if (i.kind !== 'played' || i.gameId !== gameId) continue;
    const endMs = i.atMs;
    const startMs = endMs - (i.durationMs ?? 0);
    if (endMs < win.start || startMs > win.end) continue;
    const device = i.device || here;
    if (!lanes.has(device)) lanes.set(device, []);
    const at = placeIn(win, startMs);
    lanes.get(device).push({ startMs, endMs, at, width: Math.max(0, placeIn(win, endMs) - at) });
  }
  return [...lanes].map(([device, sessions]) => ({ device, here: device === here, sessions: sessions.sort((a, b) => a.startMs - b.startMs) }));
}

/** Every snapshot of the game, all branches, with its branch. */
export function allSnapshots(game) {
  return Object.values(game?.branches ?? {}).flatMap((b) => (b.snapshots ?? []).map((s) => ({ ...s, branch: b.name })));
}

/** The snapshots taken inside the window, oldest first, placed. */
export function snapshotMarks(game, win) {
  return allSnapshots(game)
    .map((s) => ({ snap: s, ms: Date.parse(s.timestamp) }))
    .filter(({ ms }) => ms >= win.start && ms < win.end)
    .sort((a, b) => a.ms - b.ms)
    .map(({ snap, ms }) => ({ snap, ms, at: placeIn(win, ms) }));
}

/**
 * Where a branch came from, read from its first snapshot:
 * - 'main': the save since tracking began;
 * - 'copied': made here from another branch (`from`);
 * - 'kept': another device's save kept apart when both changed (`peer`);
 * - 'made': began some other way, on `ms`;
 * - 'empty': no snapshot on it yet, so nothing says.
 */
export function branchOrigin(branch) {
  const snaps = [...(branch?.snapshots ?? [])].sort((a, b) => (a.timestamp < b.timestamp ? -1 : 1));
  const first = snaps[0];
  const ms = first ? Date.parse(first.timestamp) : 0;
  if (branch?.name === 'main') return { kind: 'main', ms };
  if (!first) return { kind: 'empty', ms: 0 };
  const copied = /^branch "(.+)" created from "(.+)"$/i.exec(first.comment ?? '');
  if (copied) return { kind: 'copied', from: copied[2], ms };
  if ((first.comment ?? '').startsWith(MIRROR_PREFIX)) {
    const peer = (first.comment.slice(MIRROR_PREFIX.length).match(/^(.+?)(?: \(.*\))?$/) ?? [])[1] ?? '';
    return { kind: 'kept', peer, ms };
  }
  return { kind: 'made', ms };
}

/** The branch's newest snapshot, or null. */
export function latestOf(branch) {
  let best = null;
  for (const s of branch?.snapshots ?? []) if (!best || s.timestamp > best.timestamp) best = s;
  return best;
}

/**
 * The devices on a branch: this one when it is the branch being played, and
 * each paired device whose last sync of the game answered with it.
 * @returns {{name: string, here: boolean, seenMs: number}[]}
 */
export function devicesOn(branchName, game, peers = {}, here = '') {
  const out = [];
  if (game?.activeBranch === branchName) out.push({ name: here, here: true, seenMs: 0 });
  for (const [id, pb] of Object.entries(game?.peerBranches ?? {})) {
    const peer = peers[id];
    if (peer && pb.branch === branchName) out.push({ name: peer.name, here: false, seenMs: pb.seenMs });
  }
  return out;
}

/**
 * The branches when a new one began inside the window (not main), for the
 * timeline's markers.
 */
export function branchMarks(game, win) {
  return Object.values(game?.branches ?? {})
    .filter((b) => b.name !== 'main')
    .map((b) => ({ branch: b, origin: branchOrigin(b) }))
    .filter(({ origin }) => origin.ms >= win.start && origin.ms < win.end)
    .map(({ branch, origin }) => ({ name: branch.name, origin, at: placeIn(win, origin.ms) }));
}

/**
 * The line under the game's name, as a kind and its facts:
 * - 'playing': being played here now;
 * - 'syncing': a sync running, with its percentage;
 * - 'conflict': both sides changed it, waiting on you;
 * - 'synced': last agreed with a device (the most recent), `others` more;
 * - 'never': paired devices, none synced with yet;
 * - 'alone': no paired device.
 */
export function heroStatus({ game, peers = {}, activity = null, conflict = false }) {
  if (game?.playingSince) return { kind: 'playing', since: Date.parse(game.playingSince) };
  if (activity?.state === 'running') return { kind: 'syncing', percentage: activity.percentage ?? 0 };
  if (conflict) return { kind: 'conflict' };
  const paired = Object.values(peers);
  if (!paired.length) return { kind: 'alone' };
  const synced = paired
    .map((p) => ({ name: p.name, ms: Date.parse(game?.lastSyncedWith?.[p.id] ?? '') }))
    .filter((d) => Number.isFinite(d.ms))
    .sort((a, b) => b.ms - a.ms);
  if (!synced.length) return { kind: 'never' };
  return { kind: 'synced', device: synced[0].name, ms: synced[0].ms, others: synced.length - 1 };
}

/** Played time across every device, from Activity's entry for the game,
 *  else this device's own total. */
export function playedTotal(reportGame, game) {
  const sum = (reportGame?.devices ?? []).reduce((n, d) => n + (d.playtimeMs ?? 0), 0);
  return sum || game?.playtimeMs || 0;
}

export { DAY };
