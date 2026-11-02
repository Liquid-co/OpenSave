import { tr } from './i18n.js';
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
    { id: 'open', label: tr('common.open') },
    { id: 'favourite', label: favourite ? tr('gameHeader.removeFromFavourites') : tr('gameHeader.addToFavourites') },
    { id: 'collections', label: tr('gameactions.collections') },
    { id: 'rename', label: tr('gameactions.rename') },
    null,
    { id: 'sync', label: tr('gameHeader.syncNow') },
    { id: 'snapshot', label: tr('snapshotsTab.snapshotNow') },
    ...(canLaunch(game) ? [{ id: 'launch', label: tr('gameHeader.launch') }] : []),
    { id: 'folder', label: tr('gameactions.openSaveFolder') },
    {
      id: 'restore-latest',
      label: tr('gameactions.restoreLatestSnapshot'),
      disabled: !latest,
      hint: latest ? whenLabel(latest.timestamp, now) : tr('gameactions.noneYet')
    },
    null,
    { id: 'untrack', label: tr('manageTab.stopTracking'), danger: true }
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
      return attempt(() => api.post(`/api/games/${game.id}/sync`), tr('gameactions.syncing', { p0: game.name }));
    case 'snapshot':
      return attempt(() => api.post(`/api/games/${game.id}/snapshot`, { comment: '' }), tr('gameactions.snapshotOfTaken', { p0: game.name }));
    case 'launch':
      return attempt(() => api.post(`/api/games/${game.id}/launch`), tr('gameactions.launching', { p0: game.name }));
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
  if (!name) return { error: tr('gameactions.aGameSNameCannotBeEmpty') };
  const length = [...name].length;
  if (length > MAX_GAME_NAME) return { error: tr('gameactions.aGameSNameCanBeAtMost', { p0: MAX_GAME_NAME, p1: length }) };
  // eslint-disable-next-line no-control-regex
  if (/[\u0000-\u001f\u007f-\u009f]/.test(name)) return { error: tr('gameactions.aGameSNameCannotContainLineBreaks') };
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
    toast(tr('gameactions.renamedTo', { p0: name }), 'success');
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
  return attempt(() => api.post(`/api/games/${game.id}/rollback`, { snapshotId: snap.id }), tr('importDialog.restored', { p0: game.name }));
}

/** Stops tracking a game, with a few seconds to undo it. */
export async function untrackGame(game) {
  const current = get(view);
  if (current.name === 'game' && current.params?.gameId === game.id) navigate('home');
  const outcome = await withUndo({
    message: tr('gameactions.stoppedTrackingItsSnapshotsStayOn', { p0: game.name }),
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
    message: tr('gameactions.stoppedTrackingGameTheirSnapshotsStayOn', { p0: n, p1: n === 1 ? '' : 's' }),
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
  toast(tr('gameactions.sCloudSnapshotsWereKept', { p0: game.name }), 'info', {
    ttl: 12000,
    action: {
      label: tr('gameactions.deleteThem'),
      run: async () => {
        const ok = await askConfirm(tr('gameactions.deleteSSnapshotsFromTheCloudSnapshots', { p0: game.name }), {
          title: tr('gameactions.deleteCloudCopies'),
          confirmText: tr('gameactions.deleteFromCloud'),
          danger: true
        });
        if (!ok) return;
        try {
          const res = await api.post(`/api/cloud/delete-game/${game.id}`);
          toast(res.deleted > 0 ? tr('gameactions.removed', { p0: plural(res.deleted, 'cloud snapshot') }) : tr('gameactions.noCloudSnapshotsToRemove'), 'success');
        } catch (e) {
          toast(tr('gameactions.cloudCleanupFailed', { p0: e.message }), 'error');
        }
      }
    }
  });
}
