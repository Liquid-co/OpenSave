<script>
  // Every game, as a table: where and when it was last played, how long it
  // was played in the period on each device, how many sessions, and its last
  // snapshot and sync (lib/activityview.js gameRows). A row opens the game.
  import { onDestroy } from 'svelte';
  import ArrowDown from 'lucide-svelte/icons/arrow-down';
  import CoverImage from '../../components/CoverImage.svelte';
  import { gameCover } from '../../lib/api.js';
  import { games, conflicts, navigate } from '../../lib/stores.js';
  import { t } from '../../lib/i18n.js';
  import { timeAgoParts } from '../../lib/timeago.js';
  import { playLength } from '../../lib/format.js';
  import { snapshotKind } from '../../lib/snapshots.js';
  import { gameRows, sortRows, barParts } from '../../lib/activityview.js';
  import { gatedFlip } from '../../lib/motion.js';

  /** The activity report (GET /api/activity), gathered for the period. */
  export let report = null;
  export let period = 'month';
  /** One device's name, or '' for every device. */
  export let device = '';
  /** Device name to colour slot (deviceSlots). */
  export let slots = new Map();

  let sort = 'played';
  let now = Date.now();
  const tick = setInterval(() => (now = Date.now()), 60_000);
  onDestroy(() => clearInterval(tick));

  const ago = (ms) => {
    if (!ms) return '';
    const p = timeAgoParts(new Date(ms).toISOString(), now);
    if (p.kind === 'invalid') return p.text;
    const { kind, ...params } = p;
    return $t('timeAgo.' + kind, params);
  };
  const initials = (name = '') => name.split(/\s+/).map((w) => w[0]).join('').slice(0, 2).toUpperCase();

  $: here = report?.device ?? '';
  $: names = Object.fromEntries(Object.values($games).map((g) => [g.id, g.name]));
  $: rows = sortRows(
    gameRows({ items: report?.items ?? [], games: (report?.games ?? []).filter((g) => $games[g.gameId]), here, period, device, now }),
    sort,
    names
  );

  // The newest snapshot on any branch, said the way the snapshot list says it.
  function lastSnapshot(gameId) {
    let best = null;
    for (const b of Object.values($games[gameId]?.branches ?? {})) {
      for (const s of b.snapshots ?? []) if (!best || s.timestamp > best.timestamp) best = s;
    }
    return best ? snapshotKind(best, new Date(now)) : null;
  }
  const kindLabel = (k) => (k.kind === 'manual' ? k.title : k.kind === 'session' ? $t('activity.games.afterPlaying') : k.kind === 'safety' ? $t('activity.games.safetyCopy') : k.kind === 'start' ? $t('activity.games.trackingStarted') : $t('activity.games.saveChanged'));

  const SORTABLE = [
    ['played', 'activity.games.colLastPlayed'],
    ['time', 'activity.games.colTimePlayed'],
    ['sessions', 'activity.games.colSessions']
  ];
  $: big = rows.length > 120;
</script>

<div class="table-wrap">
  <table class="tbl">
    <thead>
      <tr>
        <th scope="col"><button class="sort" class:on={sort === 'name'} on:click={() => (sort = 'name')}>{$t('activity.games.colGame')}{#if sort === 'name'}<ArrowDown size={13} />{/if}</button></th>
        {#each SORTABLE as [id, key]}
          <th scope="col"><button class="sort" class:on={sort === id} on:click={() => (sort = id)}>{$t(key)}{#if sort === id}<ArrowDown size={13} />{/if}</button></th>
        {/each}
        <th scope="col">{$t('activity.games.colLastSnapshot')}</th>
        <th scope="col">{$t('activity.games.colLastSynced')}</th>
      </tr>
    </thead>
    <tbody>
      {#each rows as r (r.gameId)}
        {@const game = $games[r.gameId]}
        {@const snap = lastSnapshot(r.gameId)}
        {@const parts = barParts(r.byDevice, slots)}
        <tr animate:gatedFlip={{ count: rows.length, limit: 120 }} on:click={() => navigate('game', { gameId: r.gameId })} class:big>
          <td>
            <button class="game" on:click|stopPropagation={() => navigate('game', { gameId: r.gameId })}>
              <span class="thumb"><span class="initials">{initials(game.name)}</span><CoverImage src={gameCover(game, true)} alt="" /></span>
              <span class="name">{game.name}</span>
            </button>
          </td>
          <td class="two">
            {#if r.lastPlayedAt}
              <span class="main"><span class="dev-dot slot-{slots.get(r.lastPlayedOn) ?? 5}" aria-hidden="true"></span>{r.lastPlayedOn}</span>
              <span class="sub">{ago(r.lastPlayedAt)}</span>
            {:else}<span class="none">{$t('activity.games.notPlayed')}</span>{/if}
          </td>
          <td class="time">
            {#if r.playedMs > 0}
              <span class="main">{playLength(r.playedMs)}</span>
              <span class="bar" role="img" aria-label={parts.map((p) => `${p.device} ${playLength(p.ms)}`).join(', ')}>
                {#each parts as p (p.device)}<span class="seg slot-{p.slot}" style="flex: {p.share}" title="{p.device}: {playLength(p.ms)}"></span>{/each}
              </span>
              {#if r.unrecorded}<span class="sub">{$t('activity.games.notRecordedOn', { device: r.lastPlayedOn })}</span>{/if}
            {:else if r.unrecorded}
              <span class="none" title={$t('activity.games.notRecordedTitle', { device: r.lastPlayedOn })}>{$t('activity.games.notRecorded')}</span>
            {:else}<span class="none">—</span>{/if}
          </td>
          <td>{#if r.sessions}<span class="main">{r.sessions}</span>{:else}<span class="none">—</span>{/if}</td>
          <td class="two">
            {#if snap}
              <span class="main">{kindLabel(snap)}</span>
              <span class="sub">{ago(r.lastSnapshotAt)}</span>
            {:else}<span class="none">{$t('home.summary.noneYet')}</span>{/if}
          </td>
          <td class="two">
            {#if $conflicts[r.gameId]}
              <span class="main warn">{$t('activity.games.needsDecision')}</span>
              <span class="sub">{ago(r.lastSyncedAt) || '—'}</span>
            {:else if r.lastSyncedAt}
              <span class="main">{$t('activity.games.withDevice', { device: r.lastSyncedWith || $t('activity.timeline.anotherDevice') })}</span>
              <span class="sub">{ago(r.lastSyncedAt)}</span>
            {:else}
              <span class="none">{$t('activity.games.notSynced')}</span>
              <span class="sub">{$t('activity.games.onlyHere')}</span>
            {/if}
          </td>
        </tr>
      {/each}
    </tbody>
  </table>
</div>

<style>
  .table-wrap {
    background: var(--bg-raised);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    overflow: hidden;
  }
  .tbl {
    width: 100%;
    border-collapse: collapse;
    table-layout: fixed;
  }
  th {
    text-align: left;
    padding: 10px 14px;
    font-size: var(--text-sm);
    font-weight: 600;
    color: var(--text-faint);
    border-bottom: 1px solid var(--border);
    white-space: nowrap;
  }
  th:first-child {
    width: 26%;
  }
  .sort {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    margin: -4px -6px;
    padding: 4px 6px;
    border: none;
    border-radius: 6px;
    background: transparent;
    color: inherit;
    font: inherit;
    cursor: pointer;
  }
  .sort:hover {
    background: var(--bg-hover);
    color: var(--text);
  }
  .sort.on {
    color: var(--text);
  }
  td {
    padding: 10px 14px;
    border-top: 1px solid var(--border);
    vertical-align: middle;
    font-size: var(--text-md);
    min-width: 0;
  }
  tbody tr:first-child td {
    border-top: none;
  }
  tbody tr {
    cursor: pointer;
    transition: background 0.12s;
  }
  tbody tr:hover {
    background: var(--bg-hover);
  }
  .game {
    display: flex;
    align-items: center;
    gap: 12px;
    min-width: 0;
    max-width: 100%;
    border: none;
    background: none;
    padding: 0;
    color: inherit;
    font: inherit;
    text-align: left;
    cursor: pointer;
  }
  .thumb {
    position: relative;
    width: 28px;
    height: 38px;
    flex-shrink: 0;
    border-radius: 4px;
    overflow: hidden;
    background: var(--bg-active);
  }
  .thumb :global(img) {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
  .initials {
    position: absolute;
    inset: 0;
    display: grid;
    place-items: center;
    font-size: 0.6rem;
    font-weight: 700;
    color: var(--text-faint);
  }
  .name {
    font-weight: 600;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .two,
  .time {
    line-height: 1.3;
  }
  .main {
    display: flex;
    align-items: center;
    gap: 7px;
    font-weight: 600;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .main.warn {
    color: var(--warn);
  }
  .sub {
    display: block;
    font-size: var(--text-sm);
    color: var(--text-faint);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .none {
    color: var(--text-faint);
  }
  /* Time on each device: one bar, a segment per device in its colour, a
     2px gap between them (dataviz: thin marks, surface gaps). */
  .bar {
    display: flex;
    gap: 2px;
    width: 100%;
    max-width: 150px;
    height: 6px;
    margin: 6px 0 2px;
  }
  .seg {
    border-radius: 3px;
    min-width: 4px;
    transition: flex 0.3s cubic-bezier(0.2, 0.7, 0.2, 1);
  }
  .dev-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    flex-shrink: 0;
  }
  /* The device colours (lib/activityview.js deviceSlots). */
  .slot-0 {
    background: var(--device-0);
  }
  .slot-1 {
    background: var(--device-1);
  }
  .slot-2 {
    background: var(--device-2);
  }
  .slot-3 {
    background: var(--device-3);
  }
  .slot-4 {
    background: var(--device-4);
  }
  .slot-5 {
    background: var(--device-other);
  }
</style>
