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
  import ArchiveRestore from 'lucide-svelte/icons/archive-restore';
  import ArrowLeftRight from 'lucide-svelte/icons/arrow-left-right';
  import FolderPlus from 'lucide-svelte/icons/folder-plus';
  import Gamepad2 from 'lucide-svelte/icons/gamepad-2';
  import ArrowRight from 'lucide-svelte/icons/arrow-right';
  import CoverImage from '../../components/CoverImage.svelte';
  import { games, peers, navigate } from '../../lib/stores.js';
  import { gameCover } from '../../lib/api.js';
  import { recentEvents, recentText } from '../../lib/overview.js';
  import { timeAgoParts } from '../../lib/timeago.js';
  import { t } from '../../lib/i18n.js';

  // timeAgo's facts as a catalog key and its params, ready for $t.
  const agoKey = (at, now = Date.now()) => {
    const p = timeAgoParts(at, now);
    if (p.kind === 'invalid') return { text: p.text };
    const { kind, ...params } = p;
    return { key: 'timeAgo.' + kind, params };
  };

  const MOST = 6;
  const MIN_WIDTH = 210;
  const GAP = 8;

  let now = Date.now();
  const tick = setInterval(() => (now = Date.now()), 30_000);
  onDestroy(() => clearInterval(tick));

  const ICONS = { manual: Camera, auto: History, start: FolderPlus, safety: ArchiveRestore, session: Gamepad2, other: History, sync: ArrowLeftRight };
  // Null-safe for the same reason as Timeline's.
  const iso = (ms) => (ms == null || ms === '' ? null : new Date(ms).toISOString());
  // What shows while a cover loads, or when a game has none — as in the sidebar.
  const initials = (name) => name.split(/\s+/).map((w) => w[0]).join('').slice(0, 2).toUpperCase();

  $: events = recentEvents($games, $peers, { limit: MOST, now: new Date(now) });
  // Only as many as fit on one row are drawn at all, so none sits hidden
  // where the keyboard could still reach it.
  let width = 0;
  $: fit = Math.max(1, Math.min(MOST, Math.floor((width + GAP) / (MIN_WIDTH + GAP))));
  // What each event says: the lib's kinds (see "overview." in en.json) in
  // the reader's language; a session's or safety copy's own words stay.
  $: shown = events.slice(0, fit).map((e) => {
    const s = recentText(e, $peers, $t);
    return { ...e, what: s.key ? $t(s.key, s.params) : s.text };
  });
</script>

<div class="recent">
  <div class="head">
    <span class="label">{$t('home.recent.label')}</span>
    <button class="more" on:click={() => navigate('activity')}>{$t('home.recent.allActivity')}<ArrowRight size={12} /></button>
  </div>
  <div class="row" bind:clientWidth={width} style="grid-template-columns: repeat({fit}, minmax(0, 1fr))">
    {#if events.length === 0}
      <p class="empty">{$t('home.recent.empty')}</p>
    {:else}
      {#each shown as e (`${e.game.id}:${e.kind}:${e.at}`)}
        {@const w = agoKey(iso(e.at), now)}
        <button class="item" on:click={() => navigate('game', { gameId: e.game.id })} title={new Date(e.at).toLocaleString()}>
          <span class="thumb"><span class="initials">{initials(e.game.name)}</span><CoverImage src={gameCover(e.game)} alt="" /></span>
          <span class="text">
            <span class="game">{e.game.name}</span>
            <span class="what">
              <svelte:component this={ICONS[e.kind] ?? History} size={12} />
              <span class="title">{e.what}{e.count > 1 ? ` ×${e.count}` : ''}</span>
              <span class="when">· {w.text ?? $t(w.key, w.params)}</span>
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
  .initials {
    position: absolute;
    inset: 0;
    display: grid;
    place-items: center;
    font-size: 0.66rem;
    font-weight: 700;
    color: var(--text-faint);
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
