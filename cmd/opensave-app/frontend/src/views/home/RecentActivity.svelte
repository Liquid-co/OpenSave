<script>
  // The latest things that happened to your saves, across every game, along
  // the foot of the summary card: one row of them, as many as the width holds,
  // each one click from its game. See lib/overview.js.
  //
  // It was a panel of its own beside a chart of the last 14 days. The panel's
  // one-line entries left most of its width empty, and the chart said less
  // than the list; the row says the same in the space the card already has.
  import { onDestroy } from 'svelte';
  import Camera from 'lucide-svelte/icons/camera';
  import History from 'lucide-svelte/icons/history';
  import ShieldCheck from 'lucide-svelte/icons/shield-check';
  import ArrowLeftRight from 'lucide-svelte/icons/arrow-left-right';
  import FolderPlus from 'lucide-svelte/icons/folder-plus';
  import ArrowRight from 'lucide-svelte/icons/arrow-right';
  import CoverImage from '../../components/CoverImage.svelte';
  import { games, peers, navigate } from '../../lib/stores.js';
  import { gameCover } from '../../lib/api.js';
  import { recentEvents } from '../../lib/overview.js';
  import { timeAgo } from '../../lib/timeago.js';

  const MOST = 6;
  const MIN_WIDTH = 210;
  const GAP = 8;

  let now = Date.now();
  const tick = setInterval(() => (now = Date.now()), 30_000);
  onDestroy(() => clearInterval(tick));

  const ICONS = { manual: Camera, auto: History, start: FolderPlus, safety: ShieldCheck, other: History, sync: ArrowLeftRight };
  const iso = (ms) => new Date(ms).toISOString();

  $: events = recentEvents($games, $peers, { limit: MOST, now: new Date(now) });
  // Only as many as fit on one row are drawn at all, so none sits hidden
  // where the keyboard could still reach it.
  let width = 0;
  $: fit = Math.max(1, Math.min(MOST, Math.floor((width + GAP) / (MIN_WIDTH + GAP))));
  $: shown = events.slice(0, fit);
</script>

<div class="recent">
  <div class="head">
    <span class="label">Recent</span>
    <button class="more" on:click={() => navigate('activity')}>All activity<ArrowRight size={12} /></button>
  </div>
  <div class="row" bind:clientWidth={width} style="grid-template-columns: repeat({fit}, minmax(0, 1fr))">
    {#if events.length === 0}
      <p class="empty">Nothing yet. Snapshots and syncs show up here as they happen.</p>
    {:else}
      {#each shown as e (`${e.game.id}:${e.kind}:${e.at}`)}
        <button class="item" on:click={() => navigate('game', { gameId: e.game.id })} title={new Date(e.at).toLocaleString()}>
          <span class="thumb"><CoverImage src={gameCover(e.game)} alt="" /></span>
          <span class="text">
            <span class="game">{e.game.name}</span>
            <span class="what">
              <svelte:component this={ICONS[e.kind] ?? History} size={12} />
              <span class="title">{e.title}{e.count > 1 ? ` ×${e.count}` : ''}</span>
              <span class="when">· {timeAgo(iso(e.at), now)}</span>
            </span>
          </span>
        </button>
      {/each}
    {/if}
  </div>
</div>

<style>
  .recent {
    flex-basis: 100%;
    border-top: 1px solid var(--border);
    padding-top: 12px;
    min-width: 0;
  }
  .head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    margin-bottom: 6px;
  }
  .label {
    font-size: 0.74rem;
    color: var(--text-faint);
  }
  .more {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    background: none;
    border: none;
    padding: 0;
    font: inherit;
    font-size: 0.74rem;
    color: var(--text-faint);
    cursor: pointer;
  }
  .more:hover {
    color: var(--text);
  }
  .row {
    display: grid;
    gap: 8px;
    margin: 0 -8px;
  }
  .item {
    display: flex;
    align-items: center;
    gap: 10px;
    min-width: 0;
    padding: 6px 8px;
    border: none;
    border-radius: 8px;
    background: none;
    color: inherit;
    font: inherit;
    text-align: left;
    cursor: pointer;
    transition: background 0.12s;
  }
  .item:hover,
  .item:focus-visible {
    background: var(--bg-hover);
  }
  .thumb {
    position: relative;
    width: 32px;
    height: 32px;
    border-radius: 7px;
    overflow: hidden;
    flex-shrink: 0;
    background: var(--bg-active);
  }
  .thumb :global(img) {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
  .text {
    display: flex;
    flex-direction: column;
    gap: 1px;
    min-width: 0;
  }
  .game {
    font-size: 0.84rem;
    font-weight: 600;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .what {
    display: flex;
    align-items: center;
    gap: 4px;
    font-size: 0.75rem;
    color: var(--text-dim);
    min-width: 0;
  }
  .what :global(svg) {
    flex-shrink: 0;
    color: var(--text-faint);
  }
  .title {
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    min-width: 0;
  }
  .when {
    color: var(--text-faint);
    white-space: nowrap;
    flex-shrink: 0;
  }
  .empty {
    grid-column: 1 / -1;
    padding: 4px 8px 2px;
    font-size: 0.8rem;
    color: var(--text-faint);
  }
</style>
