import { tr } from './i18n.js';
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

const n = (count, one, many) => plural(count, one, many);

/**
 * The things waiting on you, most pressing first.
 * @returns {{id: string, tone: 'warn'|'info', title: string, detail: string, go: {view: string, params?: object}|null, gameId?: string}[]}
 */
export function waitingOnYou({ games = {}, conflicts = {}, locationConflicts = [], pairingRequests = [], cloudOffers = [], newGames = [], update = null } = {}) {
  const out = [];
  const name = (id) => games[id]?.name ?? tr('notifications.aGame');
  for (const id of Object.keys(conflicts)) {
    out.push({ id: `conflict:${id}`, tone: 'warn', gameId: id, title: tr('gamestatus.needsADecisionLabel', { p0: name(id) }), detail: tr('notifications.itChangedHereAndOnAnotherDevice'), go: { view: 'game', params: { gameId: id } } });
  }
  for (const c of locationConflicts) {
    out.push({ id: `location:${c.gameId}:${c.root}`, tone: 'warn', gameId: c.gameId, title: tr('gamestatus.needsADecisionLabel', { p0: name(c.gameId) }), detail: tr('notifications.itsLocationChangedOnTwoDevices', { p0: c.root }), go: { view: 'game', params: { gameId: c.gameId } } });
  }
  for (const g of Object.values(games)) {
    if (g.emptied?.state === 'held') {
      out.push({ id: `emptied:${g.id}`, tone: 'warn', gameId: g.id, title: tr('notifications.everySaveFileOfWasDeletedHere', { p0: g.name }), detail: tr('notifications.yourOtherDevicesKeepTheirsUntilYou'), go: { view: 'game', params: { gameId: g.id } } });
    }
  }
  for (const r of pairingRequests) {
    out.push({ id: `pair:${r.peerId}`, tone: 'info', title: tr('notifications.wantsToPair', { p0: r.deviceName ?? 'A device' }), detail: tr('notifications.approveItOnHomeToSyncWith'), go: { view: 'home' } });
  }
  for (const o of cloudOffers) {
    out.push({ id: `offer:${o.gameId}:${o.snapshotId}`, tone: 'info', gameId: o.gameId, title: tr('notifications.aNewerSaveFor', { p0: o.gameName ?? name(o.gameId) }), detail: tr('notifications.fromThroughTheCloud', { p0: o.deviceName ?? 'another device' }), go: { view: 'home' } });
  }
  for (const g of Object.values(games)) {
    // The same folder found on another drive here: offered, waiting for a yes.
    if (g.savePathMissing && g.relocateSuggestion) {
      out.push({ id: `relocate:${g.id}:${g.relocateSuggestion}`, tone: 'warn', gameId: g.id, title: tr('notifications.sSaveFolderMayBeAt', { p0: g.name, p1: g.relocateSuggestion }), detail: tr('notifications.confirmItBeforeItIsUsed'), go: { view: 'game', params: { gameId: g.id } } });
    }
  }
  for (const g of Object.values(games)) {
    // A drive this device doesn't have is not waiting on anyone.
    if (g.savePathMissing && !g.saveDriveMissing) {
      out.push({ id: `missing:${g.id}`, tone: 'warn', gameId: g.id, title: tr('gamestatus.sSaveFolderIsMissing', { p0: g.name }), detail: tr('notifications.nothingIsSyncedForItUntilIt'), go: { view: 'game', params: { gameId: g.id } } });
    }
  }
  for (const g of Object.values(games)) {
    const damaged = Object.values(g.branches ?? {}).flatMap((b) => (b.snapshots ?? []).filter((s) => s.problem)).length;
    if (damaged) {
      out.push({ id: `damaged:${g.id}`, tone: 'warn', gameId: g.id, title: tr('notifications.ofCanTBeRestored', { p0: n(damaged, 'snapshot', 'snapshots'), p1: g.name }), detail: tr('notifications.foundDamagedWhenReadBack'), go: { view: 'game', params: { gameId: g.id } } });
    }
  }
  if (newGames.length) {
    out.push({ id: `new:${newGames.map((g) => g.name).join(',')}`, tone: 'info', title: tr('notifications.found', { p0: n(newGames.length, 'new game', 'new games') }), detail: newGames.slice(0, 3).map((g) => g.name).join(', '), go: { view: 'home' } });
  }
  if (update?.latest) {
    out.push({ id: `update:${update.latest}`, tone: 'info', title: tr('notifications.openSaveIsAvailable', { p0: update.latest }), detail: tr('notifications.youReOnInstallItFromThe', { p0: update.current ?? 'an older version' }), go: null });
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

// Clearing the bell. What happened can be dismissed one by one or all at
// once, and of what waits on you, only what is news rather than a problem:
// games found, an update, a newer save offered from the cloud. A conflict, an
// emptied or missing save folder, a damaged snapshot or a device asking to pair
// stays until it is dealt with — hiding one would hide a game that is not
// syncing. Clearing only takes things out of the bell: Activity still has
// them. Kept per device, like what has been read.

const CLEARED_KEY = 'opensave.notificationsCleared';

/** What was cleared from the bell here: everything that happened up to
 *  `before` (ms), and single items by id. */
export function loadCleared(storage = globalThis.localStorage) {
  try {
    const v = JSON.parse(storage?.getItem(CLEARED_KEY) ?? 'null');
    return {
      before: Number(v?.before) || 0,
      ids: Array.isArray(v?.ids) ? v.ids.filter((id) => typeof id === 'string') : []
    };
  } catch {
    return { before: 0, ids: [] };
  }
}

export function saveCleared(cleared, storage = globalThis.localStorage) {
  try {
    storage?.setItem(CLEARED_KEY, JSON.stringify(cleared));
  } catch {
    // Storage refused: cleared items come back after a restart.
  }
}

/** Whether an item that waits on you can be dismissed: news, not a problem. */
export const canDismiss = (item) => /^(new|update|offer):/.test(item?.id ?? '');

/** The items still to show: those not cleared. An item waiting on you has no
 *  time, so only "Clear all" of what happened passes it by. */
export function notCleared(list, cleared) {
  const ids = new Set(cleared.ids);
  return list.filter((it) => !ids.has(it.id) && !(typeof it.atMs === 'number' && it.atMs <= cleared.before));
}

/** cleared with one more item dismissed. Ids of items no longer listed
 *  anywhere (`listed`) are dropped, so the list does not grow for ever: the
 *  ids of what waits on you name what it is about, so the next "new games",
 *  update or offer is a new id and shows. */
export function dismissOne(cleared, id, listed) {
  return { before: cleared.before, ids: [...new Set([...cleared.ids.filter((i) => listed.has(i)), id])] };
}

/** cleared with everything that happened up to now cleared. */
export function clearAllHappened(cleared, now, listed) {
  return { before: now, ids: cleared.ids.filter((i) => listed.has(i)) };
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
  return tr('notifications.gotFrom', { p0: games[ev.gameId].name, p1: plural(ev.files || 1, 'file'), p2: ev.device });
}

/** An arrival as the desktop shows it: the game's name over what came, with
 *  its cover. Asked only once arrivalMessage has said to. */
export function arrivalNote(ev, games) {
  const game = games?.[ev.gameId];
  if (!game) return null;
  return { title: game.name, body: tr('notifications.gotFromLabel', { p0: plural(ev.files || 1, 'file'), p1: ev.device }), game };
}

/** A game whose automatic snapshots keep missing files it holds open
 *  (daemon/autosnapshot.go), in the words of the catalog: `t` is the
 *  translate function, `$t` in a component. */
export function incompleteSnapshotsMessage(data, t) {
  const names = data?.files ?? [];
  const more = Math.max((data?.count ?? names.length) - names.length, 0);
  const files = names.join(', ') + (more ? t('snapshots.incomplete.more', { n: more }) : '');
  return t('snapshots.incomplete', { name: data?.gameName ?? '', files });
}

/** New games found, as the desktop shows them. */
export function newGamesNote(found) {
  if (!found?.length) return null;
  const names = found.map((g) => g.name);
  const body = names.length <= 3 ? names.join(', ') : tr('notifications.andMore', { p0: names.slice(0, 2).join(', '), p1: names.length - 2 });
  return {
    title: names.length === 1 ? tr('notifications.newGameFound') : tr('notifications.newGamesFound', { p0: names.length }),
    body: tr('notifications.trackToKeepSavesIn', { p0: body, p1: names.length === 1 ? 'it' : 'them', p2: names.length === 1 ? 'its' : 'their' }),
    open: { view: 'home', params: {} }
  };
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
    title: tr('notifications.hadesGot3FilesFromSteamDeck'),
    detail: tr('notifications.anExampleThisIsHowA'),
    unread: true,
    go: null
  };
}
