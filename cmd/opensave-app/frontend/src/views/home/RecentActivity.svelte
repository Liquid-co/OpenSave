<script>
  // The latest things that happened to your saves, across every game, each
  // one click from its game. See lib/overview.js.
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

  let now = Date.now();
  const tick = setInterval(() => (now = Date.now()), 30_000);
  onDestroy(() => clearInterval(tick));

  const ICONS = { manual: Camera, auto: History, start: FolderPlus, safety: ShieldCheck, other: History, sync: ArrowLeftRight };
  $: events = recentEvents($games, $peers, { limit: 5, now: new Date(now) });
  const iso = (ms) => new Date(ms).toISOString();
</script>

<section class="panel">
  <header>
    <h3>Recent activity</h3>
    <button class="more" on:click={() => navigate('activity')}>All activity<ArrowRight size={13} /></button>
  </header>
  {#if events.length === 0}
    <p class="empty">Nothing yet. Snapshots and syncs show up here as they happen.</p>
  {:else}
    <ul>
      {#each events as e (`${e.game.id}:${e.kind}:${e.at}`)}
        <li>
          <button class="event" on:click={() => navigate('game', { gameId: e.game.id })} title={new Date(e.at).toLocaleString()}>
            <span class="thumb"><CoverImage src={gameCover(e.game)} alt="" /></span>
            <span class="text">
              <span class="game">{e.game.name}</span>
              <span class="what">
                <svelte:component this={ICONS[e.kind] ?? History} size={13} />
                <span class="title">{e.title}</span>
                {#if e.count > 1}<span class="count">×{e.count}</span>{/if}
              </span>
            </span>
            <span class="when">{timeAgo(iso(e.at), now)}</span>
          </button>
        </li>
      {/each}
    </ul>
  {/if}
</section>

<style>
  .panel {
    background: var(--bg-raised);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    padding: 14px 8px 8px;
    min-width: 0;
  }
  header {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 12px;
    padding: 0 8px 8px;
  }
  h3 {
    font-size: 0.88rem;
    font-weight: 600;
  }
  .more {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    background: none;
    border: none;
    padding: 0;
    font: inherit;
    font-size: 0.78rem;
    color: var(--text-faint);
    cursor: pointer;
  }
  .more:hover {
    color: var(--text);
  }
  ul {
    list-style: none;
    margin: 0;
    padding: 0;
  }
  .event {
    display: flex;
    align-items: center;
    gap: 11px;
    width: 100%;
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
  .event:hover,
  .event:focus-visible {
    background: var(--bg-hover);
  }
  .thumb {
    position: relative;
    width: 30px;
    height: 30px;
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
    flex: 1;
  }
  .game {
    font-size: 0.85rem;
    font-weight: 600;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .what {
    display: flex;
    align-items: center;
    gap: 5px;
    font-size: 0.78rem;
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
  }
  .count {
    color: var(--text-faint);
    flex-shrink: 0;
  }
  .when {
    font-size: 0.76rem;
    color: var(--text-faint);
    white-space: nowrap;
    flex-shrink: 0;
  }
  .empty {
    padding: 8px 8px 12px;
    font-size: 0.82rem;
    color: var(--text-faint);
  }
</style>
