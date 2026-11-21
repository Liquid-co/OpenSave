<script>
  // What is moving between this device and others, and what moved lately.
  // Opens from Transfers in the status bar, above it; refreshes whenever a
  // sync reports progress, and every two seconds while something runs.
  //
  // Drawn as the sync panel is: a header that says where things stand, then
  // each game by its cover. A running one shows its progress as the toasts
  // do (a thin bar, the amount and the speed under it); a finished one says
  // which way it went, how much, and when.
  import { createEventDispatcher, onDestroy, onMount } from 'svelte';
  import ArrowDown from 'lucide-svelte/icons/arrow-down';
  import ArrowUp from 'lucide-svelte/icons/arrow-up';
  import ArrowDownUp from 'lucide-svelte/icons/arrow-down-up';
  import CircleCheck from 'lucide-svelte/icons/circle-check';
  import CircleX from 'lucide-svelte/icons/circle-x';
  import X from 'lucide-svelte/icons/x';
  import Activity from 'lucide-svelte/icons/activity';
  import { api, gameCover } from '../lib/api.js';
  import { games, syncActivity, navigate } from '../lib/stores.js';
  import { timeAgoParts } from '../lib/timeago.js';
  import { amountLabel, speedLabel, withNames } from '../lib/transfers.js';
  import { unfold } from '../lib/motion.js';
  import CoverImage from './CoverImage.svelte';
  // Aliased: each transfer below is named t, which would shadow the store.
  import { t as i18n } from '../lib/i18n.js';

  const dispatch = createEventDispatcher();
  let data = null;
  let error = '';
  let now = Date.now();

  async function load() {
    try {
      data = await api.get('/api/transfers');
      error = '';
    } catch (e) {
      error = e.message;
    }
    now = Date.now();
  }
  onMount(load);
  // Any sync event means the list has changed.
  const unsub = syncActivity.subscribe(() => load());
  // And while something runs, its progress moves on its own.
  const poll = setInterval(() => data?.active?.length && load(), 2000);
  onDestroy(() => {
    unsub();
    clearInterval(poll);
  });

  $: shown = withNames(data, $games);
  $: recent = shown.recent.slice(0, 20);

  const ago = (at) => {
    const p = timeAgoParts(at, now);
    if (p.kind === 'invalid') return p.text;
    const { kind, ...params } = p;
    return $i18n('timeAgo.' + kind, params);
  };
  const way = (tr) => $i18n(tr.direction === 'upload' ? 'transfers.to' : 'transfers.from', { peer: tr.peer || $i18n('transfers.anotherDevice') });
  const amount = (tr) => amountLabel(tr, (done, total) => $i18n('transfers.ofTotal', { done, total }));

  function open(tr) {
    if ($games[tr.gameId]) {
      dispatch('close');
      navigate('game', { gameId: tr.gameId });
    }
  }
  function toActivity() {
    dispatch('close');
    navigate('activity');
  }
</script>

<div class="panel" role="dialog" aria-label={$i18n('transfers.title')} transition:unfold={{ origin: 'bottom right', y: 6 }}>
  <div class="top">
    <span class="tile" class:moving={shown.active.length}><ArrowDownUp size={18} /></span>
    <div class="top-text">
      <p class="headline">{$i18n('transfers.title')}</p>
      <p class="detail" role="status">{shown.active.length ? $i18n('transfers.movingNow', { n: shown.active.length }) : $i18n('transfers.nothingNow')}</p>
    </div>
    <button class="shut" aria-label={$i18n('common.close')} title={$i18n('common.close')} on:click={() => dispatch('close')}><X size={16} /></button>
  </div>

  <div class="scroll">
    {#if error}
      <p class="empty">{$i18n('transfers.readError', { error })}</p>
    {:else if !data}
      <div class="skeleton" style="height: 52px; margin: 10px 14px"></div>
    {:else if shown.active.length === 0 && recent.length === 0}
      <div class="empty-state">
        <span class="empty-icon"><ArrowDownUp size={22} /></span>
        <p>{$i18n('transfers.empty')}</p>
      </div>
    {:else}
      {#if shown.active.length}
        <h4>{$i18n('transfers.now')}</h4>
        {#each shown.active as tr, i (tr.gameId + tr.peer + tr.direction)}
          <button class="row live" style="--i: {i}" on:click={() => open(tr)}>
            <span class="thumb"><CoverImage src={gameCover($games[tr.gameId] ?? { name: tr.name }, true)} alt="" /></span>
            <span class="main">
              <span class="line"><span class="name">{tr.name}</span><span class="pct">{tr.percentage ?? 0}%</span></span>
              <span class="sub"><svelte:component this={tr.direction === 'upload' ? ArrowUp : ArrowDown} size={12} />{way(tr)}</span>
              <span class="bar"><span style="transform: scaleX({Math.max(0.03, (tr.percentage ?? 0) / 100)})"></span></span>
              <span class="under">
                {#if amount(tr)}<span>{amount(tr)}</span>{/if}
                {#if speedLabel(tr.speedBytesPerSec)}<span>{speedLabel(tr.speedBytesPerSec)}</span>{/if}
              </span>
            </span>
          </button>
        {/each}
      {/if}
      {#if recent.length}
        <h4>{$i18n('transfers.recently')}</h4>
        {#each recent as tr, i (tr.gameId + tr.peer + tr.direction + tr.endedAt)}
          <button class="row" style="--i: {Math.min(i + shown.active.length, 10)}" on:click={() => open(tr)} title={tr.error || ''}>
            <span class="thumb"><CoverImage src={gameCover($games[tr.gameId] ?? { name: tr.name }, true)} alt="" /></span>
            <span class="main">
              <span class="name">{tr.name}</span>
              <span class="sub"><svelte:component this={tr.direction === 'upload' ? ArrowUp : ArrowDown} size={12} />{way(tr)}</span>
              {#if tr.error}<span class="err">{tr.error}</span>{/if}
            </span>
            <span class="end">
              <span class="state" class:bad={tr.state === 'error'}><svelte:component this={tr.state === 'error' ? CircleX : CircleCheck} size={14} /></span>
              {#if amount(tr)}<span class="size">{amount(tr)}</span>{/if}
              {#if tr.endedAt}<span class="when">{ago(tr.endedAt)}</span>{/if}
            </span>
          </button>
        {/each}
      {/if}
    {/if}
  </div>

  <div class="foot">
    <button class="btn small ghost" on:click={toActivity}><Activity size={14} />{$i18n('transfers.seeActivity')}</button>
  </div>
</div>

<style>
  .panel {
    position: fixed;
    right: 12px;
    bottom: calc(var(--statusbar-h) + 8px);
    z-index: 120;
    width: min(390px, calc(100vw - 24px));
    max-height: min(64vh, 560px);
    display: flex;
    flex-direction: column;
    background: var(--bg-raised);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow);
    overflow: hidden;
  }
  .top {
    flex-shrink: 0;
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 14px 12px 14px 16px;
    border-bottom: 1px solid var(--border);
  }
  .tile {
    width: 38px;
    height: 38px;
    flex: none;
    border-radius: calc(11px * var(--round, 1));
    display: grid;
    place-items: center;
    background: var(--bg-active);
    color: var(--text-dim);
  }
  .tile.moving {
    background: rgba(var(--info-rgb), 0.16);
    color: var(--info);
  }
  .tile.moving :global(svg) {
    animation: trade 1.6s ease-in-out infinite;
  }
  @keyframes trade {
    50% {
      transform: rotate(180deg);
    }
  }
  .top-text {
    flex: 1;
    min-width: 0;
  }
  .headline {
    font-size: var(--text-lg);
    font-weight: 600;
  }
  .detail {
    font-size: var(--text-sm);
    color: var(--text-faint);
  }
  .shut {
    width: 30px;
    height: 30px;
    display: grid;
    place-items: center;
    border: none;
    border-radius: calc(8px * var(--round, 1));
    background: transparent;
    color: var(--text-dim);
    cursor: pointer;
  }
  .shut:hover {
    background: var(--bg-hover);
    color: var(--text);
  }
  .scroll {
    flex: 1 1 auto;
    min-height: 0;
    overflow-y: auto;
    padding: 4px 6px 8px;
  }
  h4 {
    margin: 10px 10px 4px;
    font-size: var(--text-sm);
    font-weight: 600;
    color: var(--text-faint);
  }
  .row {
    display: flex;
    align-items: center;
    gap: 12px;
    width: 100%;
    padding: 8px 10px;
    border: none;
    border-radius: var(--radius);
    background: transparent;
    color: var(--text);
    font: inherit;
    text-align: left;
    cursor: pointer;
    animation: arrive 0.3s cubic-bezier(0.2, 0.7, 0.2, 1) backwards;
    animation-delay: calc(var(--i) * 30ms + 60ms);
  }
  .row:hover {
    background: var(--bg-hover);
  }
  .thumb {
    position: relative;
    width: 30px;
    height: 40px;
    flex: none;
    border-radius: calc(5px * var(--round, 1));
    overflow: hidden;
    background: var(--bg-active);
    box-shadow: 0 3px 8px rgba(0, 0, 0, 0.3);
  }
  .thumb :global(img) {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
  .main {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .line {
    display: flex;
    align-items: baseline;
    gap: 8px;
  }
  .name {
    flex: 1;
    min-width: 0;
    font-size: var(--text-md);
    font-weight: 600;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .sub {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    font-size: var(--text-sm);
    color: var(--text-faint);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .pct {
    font-size: var(--text-sm);
    font-weight: 600;
    color: var(--info);
    font-variant-numeric: tabular-nums;
  }
  .bar {
    height: 3px;
    margin-top: 4px;
    border-radius: calc(2px * var(--round, 1));
    background: var(--bg-active);
    overflow: hidden;
  }
  .bar span {
    display: block;
    height: 100%;
    background: var(--info);
    transform-origin: left;
    transition: transform 0.4s cubic-bezier(0.2, 0.7, 0.2, 1);
  }
  .under {
    display: flex;
    justify-content: space-between;
    gap: 8px;
    font-size: var(--text-xs);
    color: var(--text-faint);
    font-variant-numeric: tabular-nums;
  }
  .err {
    font-size: var(--text-xs);
    color: var(--danger-text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .end {
    flex: none;
    display: flex;
    flex-direction: column;
    align-items: flex-end;
    gap: 1px;
  }
  .state {
    display: flex;
    color: var(--success);
  }
  .state.bad {
    color: var(--danger);
  }
  .size,
  .when {
    font-size: var(--text-xs);
    color: var(--text-faint);
    white-space: nowrap;
  }
  .empty {
    padding: 12px 14px;
    font-size: var(--text-md);
    color: var(--text-dim);
  }
  .empty-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 10px;
    padding: 22px 20px 18px;
    text-align: center;
    color: var(--text-dim);
    font-size: var(--text-md);
  }
  .empty-icon {
    width: 44px;
    height: 44px;
    border-radius: calc(13px * var(--round, 1));
    display: grid;
    place-items: center;
    background: var(--bg-active);
    color: var(--text-faint);
  }
  .foot {
    flex-shrink: 0;
    display: flex;
    justify-content: flex-end;
    padding: 8px 10px;
    border-top: 1px solid var(--border);
    background: var(--bg-sidebar);
  }
</style>
