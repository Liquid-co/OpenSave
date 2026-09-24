// Central app state, fed by the daemon's init dump + live WS updates.
import { writable, derived, get } from 'svelte/store';
import { notifyPrefs } from './notifyprefs.js';

export const view = writable({ name: 'home', params: {} });
export const settings = writable(null);
export const games = writable({});
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
        toast(`Sync of ${get(games)[gameId]?.name ?? 'a game'} stalled — it will retry automatically`, 'error');
      } else if ((entry.state === 'error' || entry.state === 'done') && age > SYNC_ERROR_LINGER_MS) {
        delete copy[gameId];
        changed = true;
      }
    }
    return changed ? copy : s;
  });
}, 30 * 1000);
export const toasts = writable([]);
export const cloudAuthEvent = writable(null); // {success, userEmail?, error?} from the OAuth callback
export const cloudUploadEvent = writable(null); // {gameId, done, total, current, complete} while sync-local runs
export const backupProgressEvent = writable(null); // {op, done, total, current, complete} while an .sscb export/import runs
export const conflictResolution = writable(null); // {gameId, resolution, branchName?, error?} when a background resolution finishes
export const appUpdate = writable(null); // {state: downloading|installing|restarting|error, percentage, error} during self-update
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

let toastId = 0;
/** Shows a message for a few seconds. `action` ({label, run}) adds a button
 *  to it — Undo, most often — and `ttl` says how long it stays. */
export function toast(message, kind = 'info', { action = null, ttl = 0 } = {}) {
  const id = ++toastId;
  toasts.update((t) => [...t, { id, message, kind, action }]);
  // Errors stay long enough to actually read the reason.
  const life = ttl || (kind === 'error' ? 9000 : 4200);
  setTimeout(() => dismissToast(id), life);
  return id;
}
export function dismissToast(id) {
  toasts.update((t) => t.filter((x) => x.id !== id));
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
    case 'new-games':
      newGames.set(data ?? []);
      break;
    case 'cloud-offers':
      cloudOffers.set(data ?? []);
      break;
    case 'cloud-pulled':
      // Taken without asking, because it carried on from the save this
      // device had and this device had not changed since. Said out loud all
      // the same: a save that changes by itself should say who changed it.
      if (get(notifyPrefs).cloudPulled) toast(`Brought ${data.deviceName}'s newer save for “${data.gameName}” from the cloud`, 'success');
      break;
    case 'games-update':
      games.set(data ?? {});
      break;
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
    case 'sync-complete':
      syncActivity.update((s) => ({ ...s, [data.gameId]: { state: 'done', at: Date.now(), ...data.data } }));
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
    case 'sync-error': {
      syncActivity.update((s) => ({ ...s, [data.gameId]: { state: 'error', at: Date.now(), ...data.data } }));
      const gameName = get(games)[data.gameId]?.name ?? 'a game';
      const reason = friendlySyncError(data.data?.error);
      // Every failed sync is queued for automatic retry — reassure the user
      // they don't need to manually re-sync. The retry loop re-fails every
      // ~20s while the cause persists, so rate-limit the toast per game.
      const now = Date.now();
      if (now - (lastSyncErrorToast[data.gameId] ?? 0) > 60_000) {
        lastSyncErrorToast[data.gameId] = now;
        toast(`Sync failed for “${gameName}”${reason ? ' — ' + reason : ''}. OpenSave will retry automatically.`, 'error');
      }
      break;
    }
    case 'conflict-resolved': {
      const gameName = get(games)[data.gameId]?.name ?? 'a game';
      if (data.error) {
        toast(
          `Couldn't apply your choice for “${gameName}” — ${friendlySyncError(data.error)}. The conflict is still open.`,
          'error'
        );
      } else {
        toast(
          data.resolution === 'merge-branch'
            ? `Both versions kept for “${gameName}” — the other device's copy is on branch "${data.branchName}"`
            : data.resolution === 'keep-remote'
              ? `Now using the other device's version of “${gameName}” — yours is snapshotted if you change your mind`
              : `Kept this device's version of “${gameName}”`,
          'success'
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
          'error'
        );
      } else {
        toast(
          data.resolution === 'keep-remote'
            ? `Now using the other device's “${data.root}” folder for “${name}” — yours is snapshotted if you change your mind`
            : `Kept this device's “${data.root}” folder for “${name}”`,
          'success'
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
