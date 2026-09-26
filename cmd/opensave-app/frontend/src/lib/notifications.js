// The notification hub (components/NotificationHub.svelte): everything worth
// telling you, in one place behind the bell.
//
// Two sorts. Things waiting on you — a conflict, an emptied save, a device
// asking to pair, a newer save offered from the cloud, games found, a
// folder gone, a snapshot that cannot be restored, an update — come from the
// app's state and stay until they are dealt with; there is nothing to mark
// read about a question still open. Things that happened — a save arriving
// from another device, one brought from the cloud, a deletion made
// elsewhere, a restore — come from the activity history (lib/timeline.js)
// and are unread until the bell has been opened since.
import { writable } from 'svelte/store';
import { describe } from './timeline.js';
import { plural } from './format.js';

/** Activity kinds worth a notification; the rest stay on the timeline. */
export const NOTIFY_KINDS = ['received', 'cloud-pulled', 'deleted', 'restored', 'emptied', 'conflict'];

/** How far back "Earlier" reaches. */
export const NOTIFY_DAYS = 14;

const n = (count, one, many) => `${count} ${count === 1 ? one : many}`;

/**
 * The things waiting on you, most pressing first.
 * @returns {{id: string, tone: 'warn'|'info', title: string, detail: string, go: {view: string, params?: object}|null, gameId?: string}[]}
 */
export function waitingOnYou({ games = {}, conflicts = {}, locationConflicts = [], pairingRequests = [], cloudOffers = [], newGames = [], update = null } = {}) {
  const out = [];
  const name = (id) => games[id]?.name ?? 'A game';
  for (const id of Object.keys(conflicts)) {
    out.push({ id: `conflict:${id}`, tone: 'warn', gameId: id, title: `${name(id)} needs a decision`, detail: 'It changed here and on another device at once', go: { view: 'game', params: { gameId: id } } });
  }
  for (const c of locationConflicts) {
    out.push({ id: `location:${c.gameId}:${c.root}`, tone: 'warn', gameId: c.gameId, title: `${name(c.gameId)} needs a decision`, detail: `Its “${c.root}” location changed on two devices`, go: { view: 'game', params: { gameId: c.gameId } } });
  }
  for (const g of Object.values(games)) {
    if (g.emptied?.state === 'held') {
      out.push({ id: `emptied:${g.id}`, tone: 'warn', gameId: g.id, title: `Every save file of ${g.name} was deleted here`, detail: 'Your other devices keep theirs until you choose', go: { view: 'game', params: { gameId: g.id } } });
    }
  }
  for (const r of pairingRequests) {
    out.push({ id: `pair:${r.peerId}`, tone: 'info', title: `${r.deviceName ?? 'A device'} wants to pair`, detail: 'Approve it on Home to sync with it', go: { view: 'home' } });
  }
  for (const o of cloudOffers) {
    out.push({ id: `offer:${o.gameId}:${o.snapshotId}`, tone: 'info', gameId: o.gameId, title: `A newer save for ${o.gameName ?? name(o.gameId)}`, detail: `From ${o.deviceName ?? 'another device'}, through the cloud`, go: { view: 'home' } });
  }
  for (const g of Object.values(games)) {
    if (g.savePathMissing) {
      out.push({ id: `missing:${g.id}`, tone: 'warn', gameId: g.id, title: `${g.name}'s save folder is missing`, detail: 'Nothing is synced for it until it is back', go: { view: 'game', params: { gameId: g.id } } });
    }
  }
  for (const g of Object.values(games)) {
    const damaged = Object.values(g.branches ?? {}).flatMap((b) => (b.snapshots ?? []).filter((s) => s.problem)).length;
    if (damaged) {
      out.push({ id: `damaged:${g.id}`, tone: 'warn', gameId: g.id, title: `${n(damaged, 'snapshot', 'snapshots')} of ${g.name} can't be restored`, detail: 'Found damaged when read back', go: { view: 'game', params: { gameId: g.id } } });
    }
  }
  if (newGames.length) {
    out.push({ id: `new:${newGames.map((g) => g.name).join(',')}`, tone: 'info', title: `${n(newGames.length, 'new game', 'new games')} found`, detail: newGames.slice(0, 3).map((g) => g.name).join(', '), go: { view: 'home' } });
  }
  if (update?.latest) {
    out.push({ id: `update:${update.latest}`, tone: 'info', title: `OpenSave ${update.latest} is available`, detail: `You're on ${update.current ?? 'an older version'} — install it from the banner at the top`, go: null });
  }
  return out;
}

/**
 * The things that happened, newest first, from the activity history.
 * @param {object[]} items GET /api/activity items
 * @param {Record<string, object>} games
 * @param {number} seenAt when the bell was last opened (ms)
 */
export function happened(items, games = {}, seenAt = 0, now = Date.now()) {
  const since = now - NOTIFY_DAYS * 86_400_000;
  return items
    .filter((it) => NOTIFY_KINDS.includes(it.kind) && it.atMs >= since && games[it.gameId])
    .map((it) => {
      const said = describe(it, new Date(now));
      return {
        id: `${it.kind}:${it.gameId}:${it.atMs}`,
        gameId: it.gameId,
        atMs: it.atMs,
        tone: said.tone === 'warn' ? 'warn' : 'info',
        // "Hades: got 3 files from Steam Deck" — but a sentence that opens
        // with a device's name keeps it as it is.
        title: `${games[it.gameId].name}: ${it.device && said.title.startsWith(it.device) ? said.title : said.title[0].toLowerCase() + said.title.slice(1)}`,
        detail: said.detail,
        unread: it.atMs > seenAt,
        go: { view: 'game', params: { gameId: it.gameId } }
      };
    });
}

/** What the bell's badge counts: every question open, and every event unread. */
export const badgeCount = (waiting, events) => waiting.length + events.filter((e) => e.unread).length;

const SEEN_KEY = 'opensave.notificationsSeenAt';

export function loadSeenAt(storage = globalThis.localStorage) {
  try {
    return Number(storage?.getItem(SEEN_KEY)) || 0;
  } catch {
    return 0;
  }
}

export function saveSeenAt(ms, storage = globalThis.localStorage) {
  try {
    storage?.setItem(SEEN_KEY, String(ms));
  } catch {
    // Storage refused: everything reads as unread again after a restart.
  }
}

// A save arriving from another device is said in the corner too — at most
// once in a while for each game, since a game played on the other device
// saves every few minutes, and each of those arrives here.
export const ARRIVAL_QUIET_MS = 5 * 60_000;

/** The message for an activity event of a save arriving, or null when it is
 *  not one or one was shown for that game lately. lastShown is updated. */
export function arrivalMessage(ev, games, lastShown, now = Date.now()) {
  if (ev?.kind !== 'received' || !games?.[ev.gameId]) return null;
  if (now - (lastShown[ev.gameId] ?? 0) < ARRIVAL_QUIET_MS) return null;
  lastShown[ev.gameId] = now;
  return `${games[ev.gameId].name}: got ${plural(ev.files || 1, 'file')} from ${ev.device}`;
}

// Examples, from "Show me" in Settings → Notifications: entries in the bell
// that look like the real thing, gone once the bell has been looked at.
export const exampleEvents = writable([]);

export function exampleEvent(now = Date.now()) {
  return {
    id: `example:${now}`,
    example: true,
    atMs: now,
    tone: 'info',
    title: 'Hades: got 3 files from Steam Deck',
    detail: 'An example — this is how a save arriving from another device shows',
    unread: true,
    go: null
  };
}
