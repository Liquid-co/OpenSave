// What can be done to a game from anywhere: the right-click menu on the
// library and the sidebar, the game's own page, the Ctrl+K palette.
//
// One place, so that "Stop tracking" from a menu is the same act as the
// button on the game's page — same undo, same cloud question afterwards —
// rather than a second, slightly different copy of it.
import { derived, get, writable } from 'svelte/store';
import { plural } from './format.js';
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

/** Whether a game can be launched from here: it has a program or a Steam App
 *  ID, and is not known to be missing from this device — Steam asked to run a
 *  game it does not have offers to install it instead (daemon.InstallState). */
export function canLaunch(game) {
  return !!(game?.appId || game?.exePath) && game.installed !== 'not-found';
}

/** The menu for one game, in order; `null` is a divider. Pure, so tested.
 *  `favourite` is whether the game is in Favourites. */
export function gameMenuItems(game, { now = new Date(), favourite = false } = {}) {
  const latest = latestSnapshot(game);
  return [
    { id: 'open', label: 'Open' },
    { id: 'favourite', label: favourite ? 'Remove from Favourites' : 'Add to Favourites' },
    { id: 'collections', label: 'Collections…' },
    { id: 'rename', label: 'Rename…' },
    null,
    { id: 'sync', label: 'Sync now' },
    { id: 'snapshot', label: 'Snapshot now' },
    ...(canLaunch(game) ? [{ id: 'launch', label: 'Launch' }] : []),
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
    case 'rename':
      // On the game's page, where its name is: `rename` opens the name for
      // editing there (GameHeader). A new value each time, so asking twice
      // for the same game opens it again.
      return navigate('game', { gameId: game.id, rename: Date.now() });
  }
}

/** The longest name a game can have, in characters (store.MaxGameNameLength). */
export const MAX_GAME_NAME = 120;

/** A typed game name as it would be kept — trimmed — or why it cannot be:
 *  {name} or {error}. The same rules as the daemon's store.CleanGameName, so
 *  the field can say so before anything is sent. Pure, so tested. */
export function cleanGameName(raw) {
  const name = String(raw ?? '').trim();
  if (!name) return { error: "A game's name cannot be empty." };
  const length = [...name].length;
  if (length > MAX_GAME_NAME) return { error: `A game's name can be at most ${MAX_GAME_NAME} characters (this one is ${length}).` };
  // eslint-disable-next-line no-control-regex
  if (/[\u0000-\u001f\u007f-\u009f]/.test(name)) return { error: "A game's name cannot contain line breaks." };
  return { name };
}

/** Renames a game on this device. Its ID stays, so sync, snapshots and cloud
 *  copies carry on under it; other devices keep their own name for it.
 *  Resolves to whether it was renamed. */
export async function renameGame(game, raw) {
  const { name, error } = cleanGameName(raw);
  if (error) {
    toast(error, 'error');
    return false;
  }
  if (name === game.name) return true;
  try {
    await api.patch(`/api/games/${game.id}`, { name });
    toast(`Renamed to ${name}`, 'success');
    return true;
  } catch (e) {
    toast(e.message, 'error');
    return false;
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
  if (!settings?.cloudSync?.ready) return;
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
          toast(res.deleted > 0 ? `Removed ${plural(res.deleted, 'cloud snapshot')}` : 'No cloud snapshots to remove', 'success');
        } catch (e) {
          toast(`Cloud cleanup failed: ${e.message}`, 'error');
        }
      }
    }
  });
}
