// Central app state, fed by the daemon's init dump + live WS updates.
import { writable, derived } from 'svelte/store';

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
  setTimeout(() => toasts.update((t) => t.filter((x) => x.id !== id)), 4200);
}

/** Apply one WS message to the stores. */
export function applyMessage(msg) {
  const { type, data } = msg;
  switch (type) {
    case 'init':
      settings.set(data.settings ?? null);
      games.set(data.games ?? {});
      applyPeersPayload(data);
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
    case 'sync-error':
      syncActivity.update((s) => ({ ...s, [data.gameId]: { state: 'error', ...data.data } }));
      break;
  }
}

function applyPeersPayload(data) {
  if (data.peers !== undefined) peers.set(data.peers ?? {});
  if (data.discoveredPeers !== undefined) discoveredPeers.set(data.discoveredPeers ?? []);
  if (data.pairingRequests !== undefined) pairingRequests.set(data.pairingRequests ?? []);
  if (data.wanRoom !== undefined) wanRoom.set(data.wanRoom ?? null);
  if (data.conflicts !== undefined) conflicts.set(data.conflicts ?? {});
}
