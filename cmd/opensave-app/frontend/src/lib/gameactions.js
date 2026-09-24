// What can be done to a game from anywhere: the right-click menu on the
// library and the sidebar, the game's own page, the Ctrl+K palette.
//
// One place, so that "Stop tracking" from a menu is the same act as the
// button on the game's page — same undo, same cloud question afterwards —
// rather than a second, slightly different copy of it.
import { derived, get, writable } from 'svelte/store';
import { api, native } from './api.js';
import { askConfirm, gameList, games, navigate, toast, view } from './stores.js';
import { hiddenKeys, withUndo } from './undo.js';
import { whenLabel } from './snapshots.js';
import { askRestore } from './restore.js';
import { collections, toggleFavourite } from './collections.js';

/** The game whose collections are being edited, or null. */
export const collectionsDialog = writable(null);

/** The library as shown: without games on their way out (see undo.js). */
export const visibleGames = derived([gameList, hiddenKeys], ([$games, $hidden]) => $games.filter((g) => !$hidden.has(g.id)));

/** The newest snapshot on the branch being played, or null. */
export function latestSnapshot(game) {
  const snaps = game?.branches?.[game.activeBranch]?.snapshots ?? [];
  return snaps.reduce((best, s) => (!best || s.timestamp > best.timestamp ? s : best), null);
}

/** The menu for one game, in order; `null` is a divider. Pure, so tested.
 *  `favourite` is whether the game is in Favourites. */
export function gameMenuItems(game, { now = new Date(), favourite = false } = {}) {
  const latest = latestSnapshot(game);
  return [
    { id: 'open', label: 'Open' },
    { id: 'favourite', label: favourite ? 'Remove from Favourites' : 'Add to Favourites' },
    { id: 'collections', label: 'Collections…' },
    null,
    { id: 'sync', label: 'Sync now' },
    { id: 'snapshot', label: 'Snapshot now' },
    ...(game.appId || game.exePath ? [{ id: 'launch', label: 'Launch' }] : []),
    { id: 'folder', label: 'Open save folder' },
    {
      id: 'restore-latest',
      label: 'Restore latest snapshot…',
      disabled: !latest,
      hint: latest ? whenLabel(latest.timestamp, now) : 'none yet'
    },
    null,
    { id: 'untrack', label: 'Stop tracking', danger: true }
  ];
}

async function attempt(fn, success) {
  try {
    await fn();
    if (success) toast(success, 'success');
  } catch (e) {
    toast(e.message, 'error');
  }
}

export async function runGameAction(id, game) {
  switch (id) {
    case 'open':
      return navigate('game', { gameId: game.id });
    case 'sync':
      return attempt(() => api.post(`/api/games/${game.id}/sync`), `Syncing ${game.name}`);
    case 'snapshot':
      return attempt(() => api.post(`/api/games/${game.id}/snapshot`, { comment: '' }), `Snapshot of ${game.name} taken`);
    case 'launch':
      return attempt(() => api.post(`/api/games/${game.id}/launch`), `Launching ${game.name}…`);
    case 'folder': {
      const problem = await native.openFolder(game.savePath);
      if (problem) toast(problem, 'error');
      return;
    }
    case 'restore-latest':
      return restoreLatest(game);
    case 'untrack':
      return untrackGame(game);
    case 'favourite':
      return toggleFavourite(get(collections), game);
    case 'collections':
      return collectionsDialog.set(game);
  }
}

async function restoreLatest(game) {
  const snap = latestSnapshot(game);
  if (!snap) return;
  if (!(await askRestore(game, snap))) return;
  return attempt(() => api.post(`/api/games/${game.id}/rollback`, { snapshotId: snap.id }), `${game.name} restored`);
}

/** Stops tracking a game, with a few seconds to undo it. */
export async function untrackGame(game) {
  const current = get(view);
  if (current.name === 'game' && current.params?.gameId === game.id) navigate('home');
  const outcome = await withUndo({
    message: `Stopped tracking ${game.name}. Its snapshots stay on disk.`,
    keys: [game.id],
    stillThere: (id) => !!get(games)[id],
    run: () => api.del(`/api/games/${game.id}`)
  });
  if (outcome === 'done') offerCloudCleanup(game);
  return outcome;
}

/** Stops tracking several games at once, with the same few seconds. */
export function untrackGames(list) {
  const ids = list.map((g) => g.id);
  const n = ids.length;
  return withUndo({
    message: `Stopped tracking ${n} game${n === 1 ? '' : 's'}. Their snapshots stay on disk.`,
    keys: ids,
    stillThere: (id) => !!get(games)[id],
    run: () => api.post('/api/games/untrack-bulk', { ids })
  });
}

// Cloud copies outlive an untrack unless removed. Offered, not asked: a
// dialog arriving seconds after the click, over whatever is on screen by
// then, would be answered without being read.
async function offerCloudCleanup(game) {
  let settings;
  try {
    settings = await api.get('/api/settings');
  } catch {
    return;
  }
  if (!settings?.cloudSync?.enabled) return;
  toast(`${game.name}'s cloud snapshots were kept.`, 'info', {
    ttl: 12000,
    action: {
      label: 'Delete them',
      run: async () => {
        const ok = await askConfirm(`Delete "${game.name}"'s snapshots from the cloud? Snapshots on your devices are not touched.`, {
          title: 'Delete cloud copies?',
          confirmText: 'Delete from cloud',
          danger: true
        });
        if (!ok) return;
        try {
          const res = await api.post(`/api/cloud/delete-game/${game.id}`);
          toast(res.deleted > 0 ? `Removed ${res.deleted} cloud snapshot(s)` : 'No cloud snapshots to remove', 'success');
        } catch (e) {
          toast(`Cloud cleanup failed: ${e.message}`, 'error');
        }
      }
    }
  });
}
