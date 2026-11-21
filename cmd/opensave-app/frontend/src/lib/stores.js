// Central app state, fed by the daemon's init dump + live WS updates.
import { writable, derived, get } from 'svelte/store';
import { notifyPrefs } from './notifyprefs.js';
import { arrivalMessage, arrivalNote, incompleteSnapshotsMessage, newGamesNote } from './notifications.js';
import { toDesktop } from './notify.js';
import { t } from './i18n.js';
import { createToasts, restoreKey } from './toasts.js';
import { restoreProgressText } from './restoreprogress.js';
import { api } from './api.js';

// When each game's arrival was last said (lib/notifications.js).
const arrivalShown = {};
// New games already told about, by folder, so a list that shrinks or comes
// round again is not announced twice.
const newGamesSeen = new Set();

export const view = writable({ name: 'home', params: {} });
export const settings = writable(null);
export const games = writable({});
// Counts events kept in the activity history as they arrive (the daemon's
// "activity" message), for the timeline and the notifications to catch up on.
export const activityTick = writable(0);
// The newest of those events, for the notifications.
export const lastActivity = writable(null);
/** True once the daemon has sent its first full state. Until then an empty
 *  game list means "not here yet", not "no games" — without this, Home showed
 *  its first-run welcome for a moment on every launch. */
export const stateLoaded = writable(false);
/** Whether this device has paused syncing: {paused, untilRestart, endsAt}
 *  where endsAt is a local timestamp for a timed pause. See lib/syncpause.js. */
export const syncPause = writable({ paused: false });
const pauseFromWire = (st) => ({
  paused: !!st?.paused,
  untilRestart: !!st?.untilRestart,
  endsAt: st?.paused && !st.untilRestart ? Date.now() + (st.remainingSeconds ?? 0) * 1000 : null
});
export const peers = writable({});
export const discoveredPeers = writable([]);
export const pairingRequests = writable([]);
// Newer saves from other devices' cloud backups, waiting for a yes or a no.
export const cloudOffers = writable([]);
// Newly installed games the background scan found and nobody has looked at.
export const newGames = writable([]);
export const wanRoom = writable(null);
export const conflicts = writable({});
// Divergences in a game's EXTRA save locations, as a list: one game can
// have several folders disagreeing at once, so this cannot be keyed by
// game id the way whole-game conflicts are.
export const locationConflicts = writable([]);
export const logEntries = writable([]);
export const wsConnected = writable(false);
export const syncActivity = writable({}); // gameId -> {state, peerName, percentage, ...}

// Self-healing for the sync indicator: terminal events (sync-complete /
// sync-error) travel fire-and-forget, so a dropped one would leave a game
// showing "syncing… 0%" forever. Sweep entries that stopped receiving
// events — the backend's retry loop handles the actual re-sync.
const SYNC_STALE_MS = 3 * 60 * 1000; // no progress for 3 min = stalled
const SYNC_ERROR_LINGER_MS = 30 * 1000;
setInterval(() => {
  syncActivity.update((s) => {
    const now = Date.now();
    let changed = false;
    const copy = { ...s };
    for (const [gameId, entry] of Object.entries(copy)) {
      const age = now - (entry.at ?? 0);
      if (entry.state === 'running' && age > SYNC_STALE_MS) {
        delete copy[gameId];
        changed = true;
        toast(`Sync of ${get(games)[gameId]?.name ?? 'a game'} stalled — it will retry automatically`, 'error', {
          key: syncErrorKey(gameId),
          game: gameId,
          retry: () => syncGame(gameId)
        });
      } else if ((entry.state === 'error' || entry.state === 'done') && age > SYNC_ERROR_LINGER_MS) {
        delete copy[gameId];
        changed = true;
      }
    }
    return changed ? copy : s;
  });
}, 30 * 1000);
// Feedback toasts (lib/toasts.js), drawn by components/Toasts.svelte.
export const toasts = createToasts();
export const cloudAuthEvent = writable(null); // {success, userEmail?, error?} from the OAuth callback
export const cloudUploadEvent = writable(null); // {gameId, done, total, current, complete} while sync-local runs
export const backupProgressEvent = writable(null); // {op, done, total, current, complete} while an .sscb export/import runs
export const conflictResolution = writable(null); // {gameId, resolution, branchName?, error?} when a background resolution finishes
export const appUpdate = writable(null);
// A newer version found by the update check ({latest, current, ...}), for
// the notifications; the banner has its own copy.
export const availableUpdate = writable(null); // {state: downloading|installing|restarting|error, percentage, error} during self-update
export const showAbout = writable(false); // About dialog visibility (shared so any surface can open it)
export const aboutChangelogOpen = writable(false); // open About with the changelog pre-expanded

export const gameList = derived(games, ($games) =>
  Object.values($games).sort((a, b) => a.name.localeCompare(b.name))
);

export const conflictCount = derived(conflicts, ($c) => Object.keys($c).length);

export function navigate(name, params = {}) {
  view.set({ name, params });
}

// In-app confirmation dialog (replaces window.confirm, which renders as
// the bare WebView2 browser popup — jarringly non-native to the app).
// Usage: if (await askConfirm('Unpair "Laptop"?', { danger: true })) { … }
export const confirmRequest = writable(null); // {title, message, confirmText, cancelText, danger, resolve}
export function askConfirm(message, opts = {}) {
  return new Promise((resolve) => {
    confirmRequest.set({
      title: opts.title ?? 'Are you sure?',
      message,
      confirmText: opts.confirmText ?? 'Confirm',
      cancelText: opts.cancelText ?? 'Cancel',
      danger: opts.danger ?? false,
      resolve
    });
  });
}
export function answerConfirm(result) {
  const req = get(confirmRequest);
  confirmRequest.set(null);
  req?.resolve(result);
}

/**
 * Says what just happened. `kind` is success, info, warning, error or
 * progress. Options (lib/toasts.js): `game` shows its cover; `title` and
 * `detail` give the two lines (otherwise the message is split at its dash);
 * `action` ({label, run}) is the one thing to do next; `retry` offers Try
 * again on an error; `key` updates the toast already shown with that key;
 * `progress` ({done, total}) draws a bar; `icon` replaces the kind's own;
 * `ttl` says how long it stays. Returns its id.
 */
export function toast(message, kind = 'info', opts = {}) {
  return toasts.show(message, kind, opts);
}
/** Takes a toast away without anything waiting on it going ahead. */
export function dismissToast(id) {
  toasts.remove(id);
}

// Turn a raw sync error into a plain-language reason.
function friendlySyncError(raw) {
  if (!raw) return '';
  const m = String(raw).toLowerCase();
  if (m.includes('no space') || m.includes('not enough space') || m.includes('enospc') || m.includes('disk full'))
    return 'not enough free storage to save the incoming files';
  if (m.includes('permission') || m.includes('access is denied') || m.includes('eacces'))
    return 'the save folder is read-only or locked by another program';
  if (m.includes('timeout') || m.includes('connection') || m.includes('reset') || m.includes('eof') || m.includes('refused'))
    return 'the connection to the other device dropped';
  // Unknown error: show it, trimmed to something readable.
  return String(raw).slice(0, 160);
}

// Collections live in lib/collections.js, which needs this module for its
// toasts; it registers where incoming lists go rather than being imported
// here, which would make the two import each other.
let collectionsIn = () => {};
export function onCollections(fn) {
  collectionsIn = fn;
}

/** Apply one WS message to the stores. */
export function applyMessage(msg) {
  const { type, data } = msg;
  switch (type) {
    case 'init':
      settings.set(data.settings ?? null);
      games.set(data.games ?? {});
      applyPeersPayload(data, true);
      logEntries.set(data.logHistory ?? []);
      cloudOffers.set(data.cloudOffers ?? []);
      newGames.set(data.newGames ?? []);
      for (const g of data.newGames ?? []) newGamesSeen.add(g.savePath);
      syncPause.set(pauseFromWire(data.syncPause));
      collectionsIn(data.collections ?? []);
      stateLoaded.set(true);
      break;
    case 'collections-update':
      collectionsIn(data ?? []);
      break;
    case 'sync-pause':
      syncPause.set(pauseFromWire(data));
      break;
    case 'new-games': {
      const found = data ?? [];
      const fresh = found.filter((g) => !newGamesSeen.has(g.savePath));
      for (const g of found) newGamesSeen.add(g.savePath);
      newGames.set(found);
      if (fresh.length) toDesktop('newGames', newGamesNote(fresh));
      break;
    }
    case 'cloud-offers':
      cloudOffers.set(data ?? []);
      break;
    case 'snapshots-incomplete':
      // Sent once per run of discarded automatic snapshots, so no rate limit.
      toast(incompleteSnapshotsMessage(data, get(t)), 'error', {
        game: data.gameId,
        action: { label: get(t)('common.open'), run: () => navigate('game', { gameId: data.gameId }) }
      });
      break;
    case 'cloud-pulled':
      // Taken without asking, because it carried on from the save this
      // device had and this device had not changed since. Said out loud all
      // the same: a save that changes by itself should say who changed it.
      if (get(notifyPrefs).cloudPulled) {
        toast('', 'success', {
          game: data.gameId,
          title: `Newer save from the cloud`,
          detail: `${data.gameName}, from ${data.deviceName}`
        });
        toDesktop('cloudPulled', {
          title: data.gameName,
          body: `Brought ${data.deviceName}'s newer save from the cloud`,
          game: get(games)[data.gameId]
        });
      }
      break;
    case 'games-update':
      games.set(data ?? {});
      followRestores(data ?? {});
      break;
    case 'activity': {
      lastActivity.set(data);
      activityTick.update((n) => n + 1);
      const said = get(notifyPrefs).arrivals ? arrivalMessage(data, get(games), arrivalShown) : null;
      if (said) {
        const note = arrivalNote(data, get(games));
        toast(said, 'received', {
          game: data.gameId,
          title: note?.body,
          detail: note?.title,
          action: { label: get(t)('common.open'), run: () => navigate('game', { gameId: data.gameId }) }
        });
        toDesktop('arrivals', note);
      }
      break;
    }
    case 'peers-update':
      applyPeersPayload(data ?? {});
      break;
    case 'log':
      logEntries.update((l) => [...l.slice(-199), data]);
      break;
    case 'sync-start':
      syncActivity.update((s) => ({ ...s, [data.gameId]: { state: 'running', at: Date.now(), ...data.data } }));
      break;
    case 'sync-progress':
      syncActivity.update((s) => ({ ...s, [data.gameId]: { state: 'running', at: Date.now(), ...data.data } }));
      break;
    case 'sync-complete': {
      syncActivity.update((s) => ({ ...s, [data.gameId]: { state: 'done', at: Date.now(), ...data.data } }));
      const failed = get(toasts).find((x) => x.key === syncErrorKey(data.gameId));
      if (failed) toasts.change(failed.id, { kind: 'success', title: `${get(games)[data.gameId]?.name ?? 'Game'} synced`, detail: 'It worked on a later try', retry: null, action: null });
      setTimeout(
        () =>
          syncActivity.update((s) => {
            const copy = { ...s };
            if (copy[data.gameId]?.state === 'done') delete copy[data.gameId];
            return copy;
          }),
        4000
      );
      break;
    }
    case 'sync-error': {
      syncActivity.update((s) => ({ ...s, [data.gameId]: { state: 'error', at: Date.now(), ...data.data } }));
      const gameName = get(games)[data.gameId]?.name ?? 'a game';
      const reason = friendlySyncError(data.data?.error);
      // Every failed sync is queued for automatic retry — reassure the user
      // they don't need to manually re-sync. The retry loop re-fails every
      // ~20s while the cause persists, so rate-limit the toast per game.
      const now = Date.now();
      // A toast still shown for it is brought up to date instead.
      const shown = get(toasts).some((x) => x.key === syncErrorKey(data.gameId));
      if (shown || now - (lastSyncErrorToast[data.gameId] ?? 0) > 60_000) {
        lastSyncErrorToast[data.gameId] = now;
        toast('', 'error', {
          key: syncErrorKey(data.gameId),
          game: data.gameId,
          title: `Couldn't sync ${gameName}`,
          detail: reason ? `${reason.charAt(0).toUpperCase()}${reason.slice(1)}. It tries again on its own.` : 'It tries again on its own.',
          retry: () => syncGame(data.gameId),
          action: { label: get(t)('common.open'), run: () => navigate('game', { gameId: data.gameId }) }
        });
      }
      break;
    }
    case 'conflict-resolved': {
      const gameName = get(games)[data.gameId]?.name ?? 'a game';
      if (data.error) {
        toast(
          `Couldn't apply your choice for “${gameName}” — ${friendlySyncError(data.error)}. The conflict is still open.`,
          'error',
          { game: data.gameId }
        );
      } else {
        toast(
          data.resolution === 'merge-branch'
            ? `Both versions kept for “${gameName}” — you carry on with yours, and the other device's is on branch "${data.branchName}"`
            : data.resolution === 'keep-remote'
              ? `Now using the other device's version of “${gameName}” — yours is snapshotted if you change your mind`
              : `Kept this device's version of “${gameName}”`,
          'success',
          { game: data.gameId }
        );
      }
      conflictResolution.set(data ?? null);
      break;
    }
    case 'location-conflict-resolved': {
      // Applying can take a while (it may pull the other copy), so the
      // outcome arrives here rather than at the click. A failure has to be
      // said out loud: the folder stays out of sync until it is answered, and
      // a silent failure looks exactly like a silent success.
      const name = get(games)[data.gameId]?.name ?? 'a game';
      if (data.error) {
        toast(
          `Couldn't apply your choice for the “${data.root}” folder of “${name}” — ${friendlySyncError(data.error)}. It's still waiting.`,
          'error',
          { game: data.gameId }
        );
      } else {
        toast(
          data.resolution === 'keep-remote'
            ? `Now using the other device's “${data.root}” folder for “${name}” — yours is snapshotted if you change your mind`
            : `Kept this device's “${data.root}” folder for “${name}”`,
          'success',
          { game: data.gameId }
        );
      }
      break;
    }
    case 'cloud-auth':
      cloudAuthEvent.set(data ?? null);
      break;
    case 'cloud-upload':
      cloudUploadEvent.set(data ?? null);
      break;
    case 'backup-progress':
      backupProgressEvent.set(data ?? null);
      break;
    case 'app-update':
      appUpdate.set(data ?? null);
      if (data?.state === 'error') {
        toast(`Update failed — ${data.error}. Nothing was changed; you're still on the current version.`, 'error');
        setTimeout(() => appUpdate.set(null), 500);
      }
      break;
  }
}

const lastSyncErrorToast = {}; // gameId -> ms timestamp of last error toast
const syncErrorKey = (gameId) => `sync-error:${gameId}`;
const syncGame = (gameId) => api.post(`/api/games/${gameId}/sync`).catch((e) => toast(e.message, 'error'));

// A restore as one toast: the daemon's progress fills it in (the games
// update carries `restoring` while one runs), and the answer to the request
// that started it finishes it in place, as done or as what went wrong — the
// request passes the same key (restoreKey). One whose answer never comes
// here, started from another screen, is finished a moment after the daemon
// stops reporting it.
const restoringNow = new Set();
function followRestores(all) {
  const tr = get(t);
  for (const [id, g] of Object.entries(all)) {
    const p = g?.restoring;
    if (p?.phase) {
      restoringNow.add(id);
      toast('', 'progress', {
        key: restoreKey(id),
        game: id,
        title: tr('toast.restoring', { name: g.name }),
        detail: restoreProgressText(p, tr),
        progress: { done: p.done, total: p.total }
      });
    } else if (restoringNow.has(id)) {
      restoringNow.delete(id);
      setTimeout(() => {
        const left = get(toasts).find((x) => x.key === restoreKey(id) && x.kind === 'progress');
        if (left) toasts.change(left.id, { kind: 'success', title: tr('toast.restored', { name: g.name }), detail: '', progress: null });
      }, 3000);
    }
  }
}

// The stale-sync sweep lives at the top of this file, beside SYNC_STALE_MS.
//
// There was a second copy here doing the same thing on the same 30s interval,
// and the duplication was not merely redundant: the version above also raises
// a "sync stalled" toast, and whichever timer fired first removed the entry.
// So the toast appeared or did not appear depending on which interval won a
// race — the same stall telling one user and not another, for no reason
// either could see.

let wasWanConnected = false;

function applyPeersPayload(data, isInit = false) {
  if (data.peers !== undefined) peers.set(data.peers ?? {});
  if (data.discoveredPeers !== undefined) discoveredPeers.set(data.discoveredPeers ?? []);
  if (data.pairingRequests !== undefined) pairingRequests.set(data.pairingRequests ?? []);
  if (data.wanRoom !== undefined) {
    const room = data.wanRoom ?? null;
    wanRoom.set(room);
    // Toast the moment a relay connection is established — but not on the
    // initial state dump at app launch (already-connected is not news).
    const connected = !!room?.connected;
    if (connected && !wasWanConnected && !isInit) {
      toast(`Connected to relay room “${room.roomCode}”`, 'success');
    }
    wasWanConnected = connected;
  }
  if (data.conflicts !== undefined) conflicts.set(data.conflicts ?? {});
  if (data.locationConflicts !== undefined) locationConflicts.set(data.locationConflicts ?? []);
}
