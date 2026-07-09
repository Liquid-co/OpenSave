// Central app state, fed by the daemon's init dump + live WS updates.
import { writable, derived, get } from 'svelte/store';

export const view = writable({ name: 'home', params: {} });
export const settings = writable(null);
export const games = writable({});
export const peers = writable({});
export const discoveredPeers = writable([]);
export const pairingRequests = writable([]);
export const wanRoom = writable(null);
export const conflicts = writable({});
export const logEntries = writable([]);
export const wsConnected = writable(false);
export const syncActivity = writable({}); // gameId -> {state, peerName, percentage, ...}
export const toasts = writable([]);
export const cloudAuthEvent = writable(null); // {success, userEmail?, error?} from the OAuth callback

export const gameList = derived(games, ($games) =>
  Object.values($games).sort((a, b) => a.name.localeCompare(b.name))
);

export const conflictCount = derived(conflicts, ($c) => Object.keys($c).length);

export function navigate(name, params = {}) {
  view.set({ name, params });
}

let toastId = 0;
export function toast(message, kind = 'info') {
  const id = ++toastId;
  toasts.update((t) => [...t, { id, message, kind }]);
  // Errors stay long enough to actually read the reason.
  const ttl = kind === 'error' ? 9000 : 4200;
  setTimeout(() => toasts.update((t) => t.filter((x) => x.id !== id)), ttl);
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

/** Apply one WS message to the stores. */
export function applyMessage(msg) {
  const { type, data } = msg;
  switch (type) {
    case 'init':
      settings.set(data.settings ?? null);
      games.set(data.games ?? {});
      applyPeersPayload(data, true);
      logEntries.set(data.logHistory ?? []);
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
      syncActivity.update((s) => ({ ...s, [data.gameId]: { state: 'running', ...data.data } }));
      break;
    case 'sync-progress':
      syncActivity.update((s) => ({ ...s, [data.gameId]: { state: 'running', ...data.data } }));
      break;
    case 'sync-complete':
      syncActivity.update((s) => ({ ...s, [data.gameId]: { state: 'done', ...data.data } }));
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
      syncActivity.update((s) => ({ ...s, [data.gameId]: { state: 'error', ...data.data } }));
      const gameName = get(games)[data.gameId]?.name ?? 'a game';
      const reason = friendlySyncError(data.data?.error);
      toast(`Sync failed for “${gameName}”${reason ? ' — ' + reason : ''}`, 'error');
      break;
    }
    case 'cloud-auth':
      cloudAuthEvent.set(data ?? null);
      break;
  }
}

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
}
