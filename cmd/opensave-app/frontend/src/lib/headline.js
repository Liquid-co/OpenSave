// The one sentence under the Library's title: the most important thing about
// the whole library, as a catalog key and its params. In order, the first
// that applies: a save folder missing, a save emptied here, a decision
// waiting, syncing now, a sync that failed, a snapshot found damaged, games
// with no snapshot yet, games with auto-sync off — and otherwise, everything
// backed up. Moved here from the summary card it was built for, unchanged.
import { latestSnapshotAt } from './gamestatus.js';

/** @param {{game, status: {state}}[]} rows */
export function libraryHeadline(rows) {
  const list = rows ?? [];
  const by = (state) => list.filter((r) => r.status?.state === state);
  const pick = (found, one, many, oneParams) =>
    found.length === 1 ? { key: one, params: oneParams(found[0]) } : { key: many, params: { count: found.length } };

  const missing = by('missing');
  if (missing.length) return pick(missing, 'home.summaryLine.missingOne', 'home.summaryLine.missingMany', (r) => ({ name: r.game.name }));
  const emptied = by('emptied');
  if (emptied.length) return pick(emptied, 'home.summaryLine.emptiedOne', 'home.summaryLine.emptiedMany', (r) => ({ name: r.game.name }));
  const conflict = by('conflict');
  if (conflict.length) return pick(conflict, 'home.summaryLine.conflictOne', 'home.summaryLine.conflictMany', (r) => ({ name: r.game.name }));
  const syncing = by('syncing');
  if (syncing.length) return pick(syncing, 'home.summaryLine.syncingOne', 'home.summaryLine.syncingMany', (r) => ({ name: r.game.name }));
  const failed = by('error');
  if (failed.length) return pick(failed, 'home.summaryLine.failedOne', 'home.summaryLine.failedMany', (r) => ({ name: r.game.name }));
  // From the snapshots, not the state: a damaged snapshot is found on reading
  // it back, whatever the game's sync state says.
  const damaged = list.flatMap((r) =>
    Object.values(r.game?.branches ?? {}).flatMap((b) => (b.snapshots ?? []).filter((s) => s.problem).map(() => r))
  );
  if (damaged.length) return pick(damaged, 'home.summaryLine.damagedOne', 'home.summaryLine.damagedMany', (r) => ({ name: r.game.name }));
  const empty = list.filter((r) => latestSnapshotAt(r.game) === null);
  if (empty.length) return pick(empty, 'home.summaryLine.emptyOne', 'home.summaryLine.emptyMany', () => ({ count: 1 }));
  const paused = by('paused');
  if (paused.length) return pick(paused, 'home.summaryLine.pausedOne', 'home.summaryLine.pausedMany', () => ({ count: 1 }));
  return list.length === 1
    ? { key: 'home.summaryLine.backedUpOne', params: {} }
    : { key: 'home.summaryLine.backedUpMany', params: { count: list.length } };
}
